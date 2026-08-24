/**
 * Browser client for Platform Core account-center form endpoints,
 * proxied at /account-auth on the henukit edge.
 *
 * Flow: bootstrap CSRF → POST /login/code → POST /login/verify →
 * then complete Portal session via /api/v1/auth/login.
 */

const ACCOUNT_AUTH_BASE = "/account-auth";
const EXPLICIT_FORM_RESPONSE_HEADER = "X-Henukit-Form-Response";

export class AccountCenterError extends Error {
  constructor(
    message: string,
    readonly code:
      | "NETWORK"
      | "CSRF"
      | "PARSE"
      | "SEND_FAILED"
      | "VERIFY_FAILED"
      | "CONTINUATION_UNAVAILABLE"
      | "UNKNOWN" = "UNKNOWN",
    readonly requestID?: string
  ) {
    super(message);
    this.name = "AccountCenterError";
  }
}

export type BootstrapResult = {
  csrfToken: string;
  continuation?: { productName: string };
};

type AccountBootstrapFlow = "login" | "register" | "recover" | "security";

type AccountFormPath =
  | "/login/code"
  | "/login/verify"
  | "/login/password"
  | "/register/code"
  | "/register"
  | "/recover/code"
  | "/recover"
  | "/account/security/code"
  | "/account/security/password";

async function accountResponseRequestID(res: Response): Promise<string | undefined> {
  try {
    const envelope: unknown = await res.json();
    if (
      envelope &&
      typeof envelope === "object" &&
      "request_id" in envelope &&
      typeof envelope.request_id === "string" &&
      envelope.request_id.startsWith("req_")
    ) {
      return envelope.request_id;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

async function bootstrapAccountForm(
  flow: AccountBootstrapFlow,
  continuation = ""
): Promise<BootstrapResult> {
  const query = new URLSearchParams({ flow });
  if (continuation) query.set("continuation", continuation);
  const url = `${ACCOUNT_AUTH_BASE}/account/bootstrap?${query.toString()}`;
  let res: Response;
  try {
    res = await fetch(url, {
      method: "GET",
      credentials: "include",
      headers: { Accept: "application/json" },
      cache: "no-store",
    });
  } catch (e) {
    throw new AccountCenterError(
      e instanceof Error ? "无法连接登录服务，请检查网络后重试" : "网络错误",
      "NETWORK"
    );
  }
  if (res.status === 401 && flow === "security") {
    throw new AccountCenterError(
      "登录状态已过期，请重新登录后再操作",
      "VERIFY_FAILED"
    );
  }
  if (res.status === 410 && continuation) {
    const requestID = await accountResponseRequestID(res);
    throw new AccountCenterError(
      "登录链接已过期或不可继续，请重新开始登录",
      "CONTINUATION_UNAVAILABLE",
      requestID
    );
  }
  if (!res.ok) {
    const requestID = continuation
      ? await accountResponseRequestID(res)
      : undefined;
    throw new AccountCenterError(
      `登录服务暂时不可用，请稍后再试`,
      "NETWORK",
      requestID
    );
  }
  let envelope: unknown;
  try {
    envelope = await res.json();
  } catch {
    throw new AccountCenterError("登录服务返回了无法识别的内容，请刷新重试", "PARSE");
  }
  if (!envelope || typeof envelope !== "object") {
    throw new AccountCenterError("登录服务返回了无法识别的内容，请刷新重试", "PARSE");
  }
  const data = "data" in envelope ? envelope.data : undefined;
  const requestID = "request_id" in envelope ? envelope.request_id : undefined;
  if (
    !data ||
    typeof data !== "object" ||
    !("flow" in data) ||
    data.flow !== flow ||
    !("csrf_token" in data) ||
    typeof data.csrf_token !== "string" ||
    data.csrf_token.length < 32 ||
    typeof requestID !== "string" ||
    !requestID.startsWith("req_")
  ) {
    throw new AccountCenterError("登录服务返回了无法识别的内容，请刷新重试", "PARSE");
  }
  if (continuation) {
    const continuationData =
      "continuation" in data ? data.continuation : undefined;
    if (
      !continuationData ||
      typeof continuationData !== "object" ||
      !("available" in continuationData) ||
      continuationData.available !== true ||
      !("product_name" in continuationData) ||
      typeof continuationData.product_name !== "string" ||
      continuationData.product_name.length < 1 ||
      continuationData.product_name.length > 80
    ) {
      throw new AccountCenterError(
        "登录服务返回了无法识别的内容，请刷新重试",
        "PARSE"
      );
    }
    return {
      csrfToken: data.csrf_token,
      continuation: { productName: continuationData.product_name },
    };
  }
  return { csrfToken: data.csrf_token };
}

async function postAccountForm(
  path: AccountFormPath,
  fields: Record<string, string>
): Promise<{
  status: number;
  errorCode: string | null;
}> {
  const body = new URLSearchParams(fields);
  let res: Response;
  try {
    res = await fetch(`${ACCOUNT_AUTH_BASE}${path}`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "application/json",
        [EXPLICIT_FORM_RESPONSE_HEADER]: "status",
      },
      body,
      redirect: "manual",
    });
  } catch (e) {
    throw new AccountCenterError(
      e instanceof Error ? "无法连接登录服务，请检查网络后重试" : "网络错误",
      "NETWORK"
    );
  }

  // This caller opts into Platform Core's explicit status contract. A redirect
  // cannot prove that the requested credential operation established a Core
  // Session, so it must fail closed like every other unexpected response.
  if (res.type === "opaqueredirect" && res.status === 0) {
    return { status: 0, errorCode: null };
  }

  if (res.status === 403) {
    throw new AccountCenterError("登录表单已过期，请刷新页面", "CSRF");
  }

  if (res.status >= 300 && res.status < 400) {
    return { status: res.status, errorCode: null };
  }

  let errorCode: string | null = null;
  if (res.status !== 204) {
    try {
      const envelope: unknown = await res.json();
      if (
        envelope &&
        typeof envelope === "object" &&
        "error" in envelope &&
        envelope.error &&
        typeof envelope.error === "object" &&
        "code" in envelope.error &&
        typeof envelope.error.code === "string"
      ) {
        errorCode = envelope.error.code;
      }
    } catch {
      errorCode = null;
    }
  }
  return {
    status: res.status,
    errorCode,
  };
}

/** Load login form + CSRF cookie for subsequent POSTs. */
export function bootstrapAccountLogin(
  continuation = ""
): Promise<BootstrapResult> {
  return bootstrapAccountForm("login", continuation);
}

export function bootstrapAccountRegister(
  continuation = ""
): Promise<BootstrapResult> {
  return bootstrapAccountForm("register", continuation);
}

export function bootstrapPasswordRecovery(): Promise<BootstrapResult> {
  return bootstrapAccountForm("recover");
}

export function bootstrapAccountSecurity(): Promise<BootstrapResult> {
  return bootstrapAccountForm("security");
}

/** Request a 6-digit login code for a henu.edu.cn mailbox. */
export async function requestLoginCode(input: {
  csrfToken: string;
  email: string;
  returnTo: string;
}): Promise<{ csrfToken: string }> {
  const email = input.email.trim().toLowerCase();
  // Domain is fixed by UI; still reject accidental non-HENU addresses.
  if (!email.endsWith("@henu.edu.cn") || email.indexOf("@") !== email.lastIndexOf("@")) {
    throw new AccountCenterError("仅支持 @henu.edu.cn 邮箱", "SEND_FAILED");
  }

  const { status } = await postAccountForm("/login/code", {
    csrf_token: input.csrfToken,
    email,
    return_to: input.returnTo,
  });

  if (status === 204) {
    return { csrfToken: input.csrfToken };
  }
  throw new AccountCenterError(`验证码暂时发不出去，请稍后再试`, "SEND_FAILED");
}

/**
 * Verify code. On success Platform Core sets core session cookie and may
 * redirect to returnTo (OAuth authorize or portal path).
 */
export async function verifyLoginCode(input: {
  csrfToken: string;
  email: string;
  code: string;
  returnTo: string;
}): Promise<{ redirectedTo: string | null }> {
  const email = input.email.trim().toLowerCase();
  const code = input.code.trim();
  if (!/^\d{6}$/.test(code)) {
    throw new AccountCenterError("请输入 6 位数字验证码", "VERIFY_FAILED");
  }

  const { status, errorCode } = await postAccountForm(
    "/login/verify",
    {
      csrf_token: input.csrfToken,
      email,
      code,
      return_to: input.returnTo,
    }
  );

  if (status === 204) {
    return { redirectedTo: null };
  }

  throw new AccountCenterError(
    errorCode === "DEPENDENCY_UNAVAILABLE"
      ? "登录服务暂时不可用，请稍后再试"
      : "验证没有成功，请检查验证码后重试",
    "VERIFY_FAILED"
  );
}

export async function passwordLogin(input: {
  csrfToken: string;
  email: string;
  password: string;
  returnTo: string;
}): Promise<void> {
  const result = await postAccountForm("/login/password", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
    password: input.password,
    return_to: input.returnTo,
  });
  if (result.status === 204) {
    return;
  }
  throw new AccountCenterError(
    result.errorCode === "DEPENDENCY_UNAVAILABLE"
      ? "登录服务暂时不可用，请稍后再试"
      : result.errorCode === "EMAIL_CODE_LOGIN_REQUIRED"
        ? "密码尝试过多，请改用邮箱验证码登录。"
        : "邮箱或密码错误，或登录暂不可用",
    "VERIFY_FAILED"
  );
}

export async function requestRegistrationCode(input: {
  csrfToken: string;
  email: string;
  returnTo: string;
}): Promise<{ csrfToken: string }> {
  const result = await postAccountForm("/register/code", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
    return_to: input.returnTo,
  });
  if (result.status === 204) {
    return { csrfToken: input.csrfToken };
  }
  throw new AccountCenterError(
    "无法发送验证码，请检查邮箱或稍后重试",
    "SEND_FAILED"
  );
}

export async function registerAccount(input: {
  csrfToken: string;
  displayName: string;
  email: string;
  code: string;
  password: string;
  returnTo: string;
}): Promise<void> {
  const result = await postAccountForm("/register", {
    csrf_token: input.csrfToken,
    display_name: input.displayName,
    email: input.email.trim().toLowerCase(),
    code: input.code.trim(),
    password: input.password,
    return_to: input.returnTo,
  });
  if (result.status === 204) {
    return;
  }
  throw new AccountCenterError(
    result.errorCode === "DEPENDENCY_UNAVAILABLE"
      ? "登录服务暂时不可用，请稍后再试"
      : result.errorCode === "ACCOUNT_ALREADY_REGISTERED"
        ? "该邮箱已注册，请登录或找回密码。"
        : "注册失败，请检查验证码和注册信息后重试",
    "VERIFY_FAILED"
  );
}

export async function requestRecoveryCode(input: {
  csrfToken: string;
  email: string;
}): Promise<{ csrfToken: string }> {
  const result = await postAccountForm("/recover/code", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
  });
  if (result.status === 204) {
    return { csrfToken: input.csrfToken };
  }
  throw new AccountCenterError(
    "无法发送验证码，请检查邮箱或稍后重试",
    "SEND_FAILED"
  );
}

export async function recoverPassword(input: {
  csrfToken: string;
  email: string;
  code: string;
  password: string;
}): Promise<void> {
  const result = await postAccountForm("/recover", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
    code: input.code.trim(),
    password: input.password,
  });
  if (result.status === 204) {
    return;
  }
  throw new AccountCenterError(
    result.errorCode === "DEPENDENCY_UNAVAILABLE"
      ? "登录服务暂时不可用，请稍后再试"
      : "无法重置密码，请检查验证码和新密码后重试",
    "VERIFY_FAILED"
  );
}

export async function requestSecurityCode(input: {
  csrfToken: string;
  email: string;
}): Promise<{ csrfToken: string }> {
  const result = await postAccountForm("/account/security/code", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
  });
  if (result.status === 204) {
    return { csrfToken: input.csrfToken };
  }
  throw new AccountCenterError(
    "无法发送验证码，请检查邮箱或稍后重试",
    "SEND_FAILED"
  );
}

export async function changePassword(input: {
  csrfToken: string;
  email: string;
  code: string;
  currentPassword: string;
  newPassword: string;
}): Promise<void> {
  const result = await postAccountForm("/account/security/password", {
    csrf_token: input.csrfToken,
    email: input.email.trim().toLowerCase(),
    code: input.code.trim(),
    current_password: input.currentPassword,
    new_password: input.newPassword,
  });
  if (result.status === 401) {
    throw new AccountCenterError(
      "登录状态已过期，请重新登录后再修改密码",
      "VERIFY_FAILED"
    );
  }
  if (result.status !== 204) {
    throw new AccountCenterError(
      result.errorCode === "DEPENDENCY_UNAVAILABLE"
        ? "登录服务暂时不可用，请稍后再试"
        : "无法更改密码，请检查当前密码、验证码和新密码",
      "VERIFY_FAILED"
    );
  }
}

/** After core session exists, finish Portal Gateway OAuth session. */
export function portalOAuthStartUrl(nextPath: string): string {
  const next = nextPath.startsWith("/") ? nextPath : "/account";
  return `/api/v1/auth/login?return_to=${encodeURIComponent(next)}`;
}

export function buildOAuthContinuationResume(
  continuation: string,
  csrfToken: string
): {
  action: string;
  fields: { continuation: string; csrf_token: string };
} {
  return {
    action: `${ACCOUNT_AUTH_BASE}/account/continuation/resume`,
    fields: { continuation, csrf_token: csrfToken },
  };
}
