import { afterEach, describe, expect, it, vi } from "vitest";

import nextConfig from "../../../next.config";

import {
  bootstrapAccountLogin,
  bootstrapAccountRegister,
  bootstrapAccountSecurity,
  bootstrapPasswordRecovery,
  buildOAuthContinuationResume,
  changePassword,
  passwordLogin,
  registerAccount,
  recoverPassword,
  requestLoginCode,
  requestRecoveryCode,
  requestRegistrationCode,
  requestSecurityCode,
  verifyLoginCode,
} from "./account-center";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("Account Center Bootstrap", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    ["login", () => bootstrapAccountLogin()],
    ["register", () => bootstrapAccountRegister()],
    ["recover", () => bootstrapPasswordRecovery()],
    ["security", () => bootstrapAccountSecurity()],
  ] as const)("loads the %s flow from the bounded JSON contract", async (flow, bootstrap) => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({
        data: { flow, csrf_token: "csrf-token-with-at-least-thirty-two-characters" },
        request_id: "req_account_bootstrap",
      })
    );
    vi.stubGlobal("fetch", fetch);

    await expect(bootstrap()).resolves.toEqual({
      csrfToken: "csrf-token-with-at-least-thirty-two-characters",
    });
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe(`/account-auth/account/bootstrap?flow=${flow}`);
    expect(init).toMatchObject({
      method: "GET",
      credentials: "include",
      cache: "no-store",
    });
    expect(new Headers(init.headers).get("Accept")).toBe("application/json");
    expect(path).not.toContain("return_to");
    expect(path).not.toContain("code_challenge");
  });

  it("fails closed when the Bootstrap response has the wrong flow", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse({
          data: {
            flow: "register",
            csrf_token: "csrf-token-with-at-least-thirty-two-characters",
          },
          request_id: "req_account_bootstrap_wrong_flow",
        })
      )
    );

    await expect(bootstrapAccountLogin()).rejects.toMatchObject({
      code: "PARSE",
    });
  });

  it("fails closed without exposing a dependency response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: { code: "DEPENDENCY_UNAVAILABLE", message: "redis unavailable" },
            request_id: "req_account_bootstrap_unavailable",
          },
          503
        )
      )
    );

    await expect(bootstrapAccountLogin()).rejects.toMatchObject({
      code: "NETWORK",
      message: "登录服务暂时不可用，请稍后再试",
    });
  });

  it("tells an expired security session how to recover", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: { code: "CORE_SESSION_REQUIRED", message: "internal session detail" },
            request_id: "req_security_session_expired",
          },
          401
        )
      )
    );

    await expect(bootstrapAccountSecurity()).rejects.toMatchObject({
      code: "VERIFY_FAILED",
      message: "登录状态已过期，请重新登录后再操作",
    });
  });

  it("loads a trusted OAuth continuation destination without exposing OAuth facts", async () => {
    const fetch = vi.fn().mockResolvedValue(
      jsonResponse({
        data: {
          flow: "login",
          csrf_token: "csrf-token-with-at-least-thirty-two-characters",
          continuation: { available: true, product_name: "HENU Kit" },
        },
        request_id: "req_account_continuation",
      })
    );
    vi.stubGlobal("fetch", fetch);

    await expect(
      bootstrapAccountLogin("opaque-continuation-handle/value")
    ).resolves.toEqual({
      csrfToken: "csrf-token-with-at-least-thirty-two-characters",
      continuation: { productName: "HENU Kit" },
    });
    expect(fetch.mock.calls[0]?.[0]).toBe(
      "/account-auth/account/bootstrap?flow=login&continuation=opaque-continuation-handle%2Fvalue"
    );
  });

  it("classifies an unavailable OAuth continuation with a safe request id", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: {
              code: "OAUTH_CONTINUATION_UNAVAILABLE",
              message: "internal continuation detail",
            },
            request_id: "req_continuation_unavailable",
          },
          410
        )
      )
    );

    await expect(bootstrapAccountLogin("expired-handle")).rejects.toMatchObject({
      code: "CONTINUATION_UNAVAILABLE",
      message: "登录链接已过期或不可继续，请重新开始登录",
      requestID: "req_continuation_unavailable",
    });
  });

  it("keeps the request id when a continuation dependency is temporarily unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: { code: "DEPENDENCY_UNAVAILABLE", message: "redis unavailable" },
            request_id: "req_continuation_dependency",
          },
          503
        )
      )
    );

    await expect(bootstrapAccountLogin("opaque-handle")).rejects.toMatchObject({
      code: "NETWORK",
      message: "登录服务暂时不可用，请稍后再试",
      requestID: "req_continuation_dependency",
    });
  });
});

describe("OAuth continuation resume", () => {
  it("builds a same-origin POST without putting the handle in the action URL", () => {
    expect(
      buildOAuthContinuationResume(
        "opaque-continuation-handle",
        "csrf-token-with-at-least-thirty-two-characters"
      )
    ).toEqual({
      action: "/account-auth/account/continuation/resume",
      fields: {
        continuation: "opaque-continuation-handle",
        csrf_token: "csrf-token-with-at-least-thirty-two-characters",
      },
    });
  });

  it("configures the Account Center document boundary as private and non-referring", async () => {
    const rules = await nextConfig.headers?.();
    const accountRule = rules?.find((rule) => rule.source === "/account/:path*");
    const headers = new Map(
      accountRule?.headers.map((header) => [header.key.toLowerCase(), header.value])
    );

    expect(headers.get("cache-control")).toContain("no-store");
    expect(headers.get("referrer-policy")).toBe("no-referrer");
    expect(headers.get("content-security-policy")).toContain(
      "frame-ancestors 'none'"
    );
  });
});

describe("Account Center form status contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("requests a login code without parsing a Platform Core HTML page", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 204,
        headers: { "X-Verification-Expires": "2026-08-24T07:30:00Z" },
      })
    );
    vi.stubGlobal("fetch", fetch);

    await expect(
      requestLoginCode({
        csrfToken: "csrf-token-with-at-least-thirty-two-characters",
        email: "Student@henu.edu.cn",
        returnTo: "/api/v1/auth/login?return_to=%2Faccount",
      })
    ).resolves.toEqual({
      csrfToken: "csrf-token-with-at-least-thirty-two-characters",
    });

    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/account-auth/login/code");
    expect(new Headers(init.headers).get("Accept")).toBe("application/json");
    expect(new Headers(init.headers).get("X-Henukit-Form-Response")).toBe("status");
    expect(String(init.body)).toBe(
      "csrf_token=csrf-token-with-at-least-thirty-two-characters&email=student%40henu.edu.cn&return_to=%2Fapi%2Fv1%2Fauth%2Flogin%3Freturn_to%3D%252Faccount"
    );
  });

  it.each([
    [
      "email code",
      "/account-auth/login/verify",
      () =>
        verifyLoginCode({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          email: "student@henu.edu.cn",
          code: "123456",
          returnTo: "/api/v1/auth/login?return_to=%2Faccount",
        }),
    ],
    [
      "password",
      "/account-auth/login/password",
      () =>
        passwordLogin({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          email: "student@henu.edu.cn",
          password: "correct horse battery staple",
          returnTo: "/api/v1/auth/login?return_to=%2Faccount",
        }),
    ],
  ] as const)("accepts a structured %s authentication result", async (_mode, path, authenticate) => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);

    await expect(authenticate()).resolves.not.toThrow();
    const [calledPath, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(calledPath).toBe(path);
    expect(new Headers(init.headers).get("Accept")).toBe("application/json");
    expect(new Headers(init.headers).get("X-Henukit-Form-Response")).toBe("status");
  });

  it.each([
    [
      "email-code login",
      () =>
        verifyLoginCode({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          email: "student@henu.edu.cn",
          code: "123456",
          returnTo: "/api/v1/auth/login?return_to=%2Faccount",
        }),
    ],
    [
      "password login",
      () =>
        passwordLogin({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          email: "student@henu.edu.cn",
          password: "correct horse battery staple",
          returnTo: "/api/v1/auth/login?return_to=%2Faccount",
        }),
    ],
    [
      "registration",
      () =>
        registerAccount({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          displayName: "小河同学",
          email: "student@henu.edu.cn",
          code: "123456",
          password: "correct horse battery staple",
          returnTo: "/api/v1/auth/login?return_to=%2Faccount",
        }),
    ],
    [
      "password recovery",
      () =>
        recoverPassword({
          csrfToken: "csrf-token-with-at-least-thirty-two-characters",
          email: "student@henu.edu.cn",
          code: "123456",
          password: "replacement password value",
        }),
    ],
  ] as const)("rejects a redirect as an unproven %s result", async (_journey, authenticate) => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 303,
          headers: { Location: "/unexpected-legacy-result" },
        })
      )
    );

    await expect(authenticate()).rejects.toMatchObject({ code: "VERIFY_FAILED" });
  });

  it("registers through status responses without parsing Platform Core HTML", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    const csrfToken = "csrf-token-with-at-least-thirty-two-characters";
    const returnTo = "/api/v1/auth/login?return_to=%2Faccount%2Fsecurity";

    await expect(
      requestRegistrationCode({ csrfToken, email: "student@henu.edu.cn", returnTo })
    ).resolves.toEqual({ csrfToken });
    await expect(
      registerAccount({
        csrfToken,
        displayName: "小河同学",
        email: "student@henu.edu.cn",
        code: "123456",
        password: "correct horse battery staple",
        returnTo,
      })
    ).resolves.not.toThrow();

    expect(fetch.mock.calls.map(([path]) => path)).toEqual([
      "/account-auth/register/code",
      "/account-auth/register",
    ]);
    for (const [, init] of fetch.mock.calls as [string, RequestInit][]) {
      expect(new Headers(init.headers).get("Accept")).toBe("application/json");
      expect(new Headers(init.headers).get("X-Henukit-Form-Response")).toBe("status");
    }
  });

  it("recovers a password through status responses without parsing Platform Core HTML", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    const csrfToken = "csrf-token-with-at-least-thirty-two-characters";

    await expect(
      requestRecoveryCode({ csrfToken, email: "student@henu.edu.cn" })
    ).resolves.toEqual({ csrfToken });
    await expect(
      recoverPassword({
        csrfToken,
        email: "student@henu.edu.cn",
        code: "123456",
        password: "replacement password value",
      })
    ).resolves.not.toThrow();

    expect(fetch.mock.calls.map(([path]) => path)).toEqual([
      "/account-auth/recover/code",
      "/account-auth/recover",
    ]);
  });

  it("changes a password through status responses without parsing Platform Core HTML", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    const csrfToken = "csrf-token-with-at-least-thirty-two-characters";

    await expect(
      requestSecurityCode({ csrfToken, email: "student@henu.edu.cn" })
    ).resolves.toEqual({ csrfToken });
    await expect(
      changePassword({
        csrfToken,
        email: "student@henu.edu.cn",
        code: "123456",
        currentPassword: "current password value",
        newPassword: "replacement password value",
      })
    ).resolves.not.toThrow();

    expect(fetch.mock.calls.map(([path]) => path)).toEqual([
      "/account-auth/account/security/code",
      "/account-auth/account/security/password",
    ]);
  });

  it("maps structured form errors to public Account Center guidance", async () => {
    const responses = [
      jsonResponse(
        {
          error: { code: "EMAIL_CODE_LOGIN_REQUIRED", message: "internal challenge detail" },
          request_id: "req_password_challenge",
        },
        429
      ),
      jsonResponse(
        {
          error: { code: "ACCOUNT_ALREADY_REGISTERED", message: "internal identity detail" },
          request_id: "req_registration_conflict",
        },
        409
      ),
      jsonResponse(
        {
          error: { code: "DEPENDENCY_UNAVAILABLE", message: "redis unavailable" },
          request_id: "req_login_dependency",
        },
        503
      ),
    ];
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => Promise.resolve(responses.shift())));
    const common = {
      csrfToken: "csrf-token-with-at-least-thirty-two-characters",
      email: "student@henu.edu.cn",
      returnTo: "/api/v1/auth/login?return_to=%2Faccount",
    };

    await expect(
      passwordLogin({ ...common, password: "wrong password value" })
    ).rejects.toMatchObject({
      code: "VERIFY_FAILED",
      message: "密码尝试过多，请改用邮箱验证码登录。",
    });
    await expect(
      registerAccount({
        ...common,
        displayName: "小河同学",
        code: "123456",
        password: "correct horse battery staple",
      })
    ).rejects.toMatchObject({
      code: "VERIFY_FAILED",
      message: "该邮箱已注册，请登录或找回密码。",
    });
    await expect(requestLoginCode(common)).rejects.toMatchObject({
      code: "SEND_FAILED",
      message: "验证码暂时发不出去，请稍后再试",
    });
  });

  it("distinguishes an unavailable login dependency from invalid credentials", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: { code: "DEPENDENCY_UNAVAILABLE", message: "redis unavailable" },
            request_id: "req_login_verify_dependency",
          },
          503
        )
      )
    );

    await expect(
      verifyLoginCode({
        csrfToken: "csrf-token-with-at-least-thirty-two-characters",
        email: "student@henu.edu.cn",
        code: "123456",
        returnTo: "/api/v1/auth/login?return_to=%2Faccount",
      })
    ).rejects.toMatchObject({
      code: "VERIFY_FAILED",
      message: "登录服务暂时不可用，请稍后再试",
    });
  });

  it("treats a structured CSRF rejection as an expired Account Center form", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: { code: "CSRF_REJECTED", message: "internal csrf detail" },
            request_id: "req_csrf_rejected",
          },
          403
        )
      )
    );

    await expect(
      requestLoginCode({
        csrfToken: "csrf-token-with-at-least-thirty-two-characters",
        email: "student@henu.edu.cn",
        returnTo: "/api/v1/auth/login",
      })
    ).rejects.toMatchObject({
      code: "CSRF",
      message: "登录表单已过期，请刷新页面",
    });
  });
});
