/**
 * Browser client for Platform Core account-center form endpoints,
 * proxied at /account-auth on the henukit edge.
 *
 * Flow: bootstrap CSRF → POST /login/code → POST /login/verify →
 * then complete Portal session via /api/v1/auth/login.
 */

const ACCOUNT_AUTH_BASE = "/account-auth";

export class AccountCenterError extends Error {
  constructor(
    message: string,
    readonly code:
      | "NETWORK"
      | "CSRF"
      | "PARSE"
      | "SEND_FAILED"
      | "VERIFY_FAILED"
      | "UNKNOWN" = "UNKNOWN"
  ) {
    super(message);
    this.name = "AccountCenterError";
  }
}

function extractCsrf(html: string): string | null {
  const match =
    html.match(/name=["']csrf_token["'][^>]*value=["']([^"']+)["']/) ||
    html.match(/value=["']([^"']+)["'][^>]*name=["']csrf_token["']/);
  return match?.[1] ?? null;
}

function extractError(html: string): string | null {
  const match = html.match(/class="error"[^>]*>([^<]+)</);
  return match?.[1]?.trim() || null;
}

function isCodeStep(html: string): boolean {
  return (
    html.includes('name="code"') ||
    html.includes("验证码已进入发送队列") ||
    html.includes('id="code"')
  );
}

export type BootstrapResult = {
  csrfToken: string;
  /** Absolute path to resume OAuth after core session exists */
  returnTo: string;
};

/** Load login form + CSRF cookie for subsequent POSTs. */
export async function bootstrapAccountLogin(
  returnTo: string
): Promise<BootstrapResult> {
  const url = `${ACCOUNT_AUTH_BASE}/login?return_to=${encodeURIComponent(returnTo)}`;
  let res: Response;
  try {
    res = await fetch(url, {
      method: "GET",
      credentials: "include",
      headers: { Accept: "text/html" },
      cache: "no-store",
    });
  } catch (e) {
    throw new AccountCenterError(
      e instanceof Error ? e.message : "网络错误",
      "NETWORK"
    );
  }
  if (!res.ok) {
    throw new AccountCenterError(`账号中心不可用（${res.status}）`, "NETWORK");
  }
  const html = await res.text();
  const csrfToken = extractCsrf(html);
  if (!csrfToken || csrfToken.length < 16) {
    throw new AccountCenterError("无法获取登录凭证，请刷新重试", "CSRF");
  }
  return { csrfToken, returnTo };
}

async function postLoginForm(
  path: "/login/code" | "/login/verify",
  fields: Record<string, string>
): Promise<{ html: string; redirectedTo: string | null; status: number }> {
  const body = new URLSearchParams(fields);
  let res: Response;
  try {
    res = await fetch(`${ACCOUNT_AUTH_BASE}${path}`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "text/html",
      },
      body,
      redirect: "manual",
    });
  } catch (e) {
    throw new AccountCenterError(
      e instanceof Error ? e.message : "网络错误",
      "NETWORK"
    );
  }

  if (res.status === 403) {
    throw new AccountCenterError("登录表单已过期，请刷新页面", "CSRF");
  }

  // Successful verify issues 303 to return_to
  if (res.status >= 300 && res.status < 400) {
    const loc = res.headers.get("Location");
    return { html: "", redirectedTo: loc, status: res.status };
  }

  const html = await res.text();
  return { html, redirectedTo: null, status: res.status };
}

/** Request a 6-digit login code for a henu.edu.cn mailbox. */
export async function requestLoginCode(input: {
  csrfToken: string;
  email: string;
  returnTo: string;
}): Promise<{ csrfToken: string }> {
  const email = input.email.trim().toLowerCase();
  if (!email.endsWith("@henu.edu.cn")) {
    throw new AccountCenterError("仅支持 @henu.edu.cn 邮箱", "SEND_FAILED");
  }

  const { html, status } = await postLoginForm("/login/code", {
    csrf_token: input.csrfToken,
    email,
    return_to: input.returnTo,
  });

  if (status >= 400 && status !== 200) {
    throw new AccountCenterError(`发送失败（${status}）`, "SEND_FAILED");
  }

  const err = extractError(html);
  if (err) {
    throw new AccountCenterError(err, "SEND_FAILED");
  }
  if (!isCodeStep(html)) {
    // Some responses still 200 without code field — treat as soft failure
    throw new AccountCenterError(
      "无法发送验证码，请检查邮箱或稍后重试",
      "SEND_FAILED"
    );
  }

  // CSRF rotates on each render — pull fresh token if present
  const nextCsrf = extractCsrf(html) ?? input.csrfToken;
  return { csrfToken: nextCsrf };
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

  const { html, redirectedTo, status } = await postLoginForm("/login/verify", {
    csrf_token: input.csrfToken,
    email,
    code,
    return_to: input.returnTo,
  });

  if (redirectedTo) {
    return { redirectedTo };
  }

  if (status >= 400) {
    throw new AccountCenterError(`验证失败（${status}）`, "VERIFY_FAILED");
  }

  const err = extractError(html);
  throw new AccountCenterError(
    err || "验证码无效、已过期或登录暂不可用",
    "VERIFY_FAILED"
  );
}

/** After core session exists, finish Portal Gateway OAuth session. */
export function portalOAuthStartUrl(nextPath: string): string {
  const next = nextPath.startsWith("/") ? nextPath : "/account";
  return `/api/v1/auth/login?return_to=${encodeURIComponent(next)}`;
}
