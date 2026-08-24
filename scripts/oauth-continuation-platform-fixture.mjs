import { createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import { createServer } from "node:http";

const required = (name) => {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
};

const config = {
  address: required("OAUTH_FIXTURE_ADDRESS"),
  portalOrigin: required("OAUTH_FIXTURE_PORTAL_ORIGIN"),
  clientID: required("OAUTH_FIXTURE_CLIENT_ID"),
  clientSecret: required("OAUTH_FIXTURE_CLIENT_SECRET"),
  keyID: required("OAUTH_FIXTURE_KEY_ID"),
  redirectURI: required("OAUTH_FIXTURE_REDIRECT_URI"),
  productName: required("OAUTH_FIXTURE_PRODUCT_NAME"),
  cookieNamespace: required("OAUTH_FIXTURE_COOKIE_NAMESPACE"),
  testEmail: required("OAUTH_FIXTURE_TEST_EMAIL"),
  testPassword: required("OAUTH_FIXTURE_TEST_PASSWORD"),
  exchangeToken: required("OAUTH_FIXTURE_EXCHANGE_TOKEN"),
  userID: required("OAUTH_FIXTURE_USER_ID"),
  displayName: process.env.OAUTH_FIXTURE_DISPLAY_NAME?.trim() ?? "",
  authorizationMode:
    process.env.OAUTH_FIXTURE_AUTHORIZATION_MODE?.trim() ?? "disabled",
  unsupportedClientID:
    process.env.OAUTH_FIXTURE_UNSUPPORTED_CLIENT_ID?.trim() ??
    "retired-practice-client",
  unsupportedRedirectURI:
    process.env.OAUTH_FIXTURE_UNSUPPORTED_REDIRECT_URI?.trim() ??
    "https://practice.henukit.test/auth/callback",
};

if (!/^[A-Za-z0-9_]+$/.test(config.cookieNamespace)) {
  throw new Error("OAUTH_FIXTURE_COOKIE_NAMESPACE must be alphanumeric");
}
if (!new Set(["disabled", "allow"]).has(config.authorizationMode)) {
  throw new Error(
    "OAUTH_FIXTURE_AUTHORIZATION_MODE must be disabled or allow"
  );
}

const cookieNames = {
  device: `henukit_core_device_${config.cookieNamespace}`,
  csrf: `henukit_core_csrf_${config.cookieNamespace}`,
  session: `henukit_core_session_${config.cookieNamespace}`,
};
const continuations = new Map();
const codes = new Map();
const events = [];

const randomToken = (size) => randomBytes(size).toString("base64url");

const recordEvent = (clientID, outcome, startedAt = Date.now()) => {
  const event = {
    request_id: `req_${randomToken(12)}`,
    client_id: clientID,
    outcome,
    duration_ms: Math.max(0, Date.now() - startedAt),
  };
  events.push(event);
  console.log(JSON.stringify(event));
  return event.request_id;
};

const activeContinuation = (handle) => {
  const stored = continuations.get(handle);
  if (!stored) return undefined;
  if (stored.expiresAt <= Date.now()) {
    continuations.delete(handle);
    return undefined;
  }
  return stored;
};

const cookies = (request) =>
  Object.fromEntries(
    (request.headers.cookie ?? "")
      .split(";")
      .map((part) => part.trim())
      .filter(Boolean)
      .map((part) => {
        const separator = part.indexOf("=");
        return separator < 0
          ? [part, ""]
          : [part.slice(0, separator), part.slice(separator + 1)];
      })
  );

const browserCookie = (name, value, maxAge) =>
  `${name}=${value}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}`;

const writeJSON = (response, status, value) => {
  response.writeHead(status, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
    "Referrer-Policy": "no-referrer",
  });
  response.end(JSON.stringify(value));
};

const readBody = async (request) => {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > 16 * 1024) throw new Error("request body is too large");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
};

const hasValidServiceSignature = (request, body) => {
  const expectedAuthorization = `Basic ${Buffer.from(
    `${config.clientID}:${config.clientSecret}`
  ).toString("base64")}`;
  if (
    request.headers.authorization !== expectedAuthorization ||
    request.headers["x-service-id"] !== config.clientID ||
    request.headers["x-key-id"] !== config.keyID
  ) {
    return false;
  }
  const digest = createHash("sha256").update(body).digest("hex");
  const canonical = [
    request.method,
    request.url,
    request.headers["x-timestamp"] ?? "",
    request.headers["x-nonce"] ?? "",
    digest,
  ].join("\n");
  const expected = createHmac("sha256", config.clientSecret)
    .update(canonical)
    .digest("base64url");
  const actual = request.headers["x-signature"] ?? "";
  const expectedBytes = Buffer.from(expected);
  const actualBytes = Buffer.from(actual);
  return (
    expectedBytes.length === actualBytes.length &&
    timingSafeEqual(expectedBytes, actualBytes)
  );
};

const handleAuthorize = (request, response, url) => {
  const startedAt = Date.now();
  const query = url.searchParams;
  const clientID = query.get("client_id") ?? "unknown-client";
  const observableClientID =
    clientID === config.clientID || clientID === config.unsupportedClientID
      ? clientID
      : "unknown-client";
  const validShape =
    query.get("response_type") === "code" &&
    query.get("code_challenge_method") === "S256" &&
    (query.get("state")?.length ?? 0) >= 8 &&
    (query.get("code_challenge")?.length ?? 0) === 43;

  if (clientID === config.unsupportedClientID) {
    if (
      !validShape ||
      query.get("redirect_uri") !== config.unsupportedRedirectURI
    ) {
      recordEvent(clientID, "authorize_invalid", startedAt);
      response.writeHead(400).end("invalid authorize");
      return;
    }
    const requestID = recordEvent(
      clientID,
      "unsupported_client_rejected",
      startedAt
    );
    response
      .writeHead(303, {
        Location: `${config.portalOrigin}/account/login?continuation_error=unsupported&request_id=${requestID}`,
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
      })
      .end();
    return;
  }

  if (
    !validShape ||
    clientID !== config.clientID ||
    query.get("redirect_uri") !== config.redirectURI ||
    query.get("code_challenge_method") !== "S256"
  ) {
    recordEvent(observableClientID, "authorize_invalid", startedAt);
    response.writeHead(400).end("invalid authorize");
    return;
  }

  const stored = {
    state: query.get("state"),
    challenge: query.get("code_challenge"),
    redirectURI: query.get("redirect_uri"),
  };
  if (cookies(request)[cookieNames.session]) {
    const code = randomToken(32);
    codes.set(code, stored);
    const callback = new URL(stored.redirectURI);
    callback.searchParams.set("code", code);
    callback.searchParams.set("state", stored.state);
    recordEvent(clientID, "core_session_fast_path", startedAt);
    response
      .writeHead(302, {
        Location: callback.toString(),
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
      })
      .end();
    return;
  }

  let device = cookies(request)[cookieNames.device];
  const headers = {
    "Cache-Control": "no-store",
    "Referrer-Policy": "no-referrer",
  };
  if (!device) {
    device = randomToken(16);
    headers["Set-Cookie"] = browserCookie(cookieNames.device, device, 1800);
  }
  const handle = randomToken(32);
  continuations.set(handle, {
    ...stored,
    device,
    expiresAt:
      Date.now() +
      (stored.state.startsWith("expires_soon_") ? 250 : 60_000),
  });
  recordEvent(clientID, "continuation_created", startedAt);
  headers.Location = `${config.portalOrigin}/account/login?continuation=${encodeURIComponent(handle)}`;
  response.writeHead(302, headers).end();
};

const handleBootstrap = (request, response, url) => {
  const startedAt = Date.now();
  const handle = url.searchParams.get("continuation");
  const stored = activeContinuation(handle);
  if (
    !stored ||
    stored.device !== cookies(request)[cookieNames.device] ||
    url.searchParams.get("flow") !== "login"
  ) {
    recordEvent(config.clientID, "continuation_unavailable", startedAt);
    writeJSON(response, 410, {
      error: {
        code: "OAUTH_CONTINUATION_UNAVAILABLE",
        message: "unavailable",
      },
      request_id: "req_e2e_continuation_unavailable",
    });
    return;
  }
  const csrf = randomToken(32);
  recordEvent(config.clientID, "continuation_bootstrapped", startedAt);
  response.setHeader("Set-Cookie", browserCookie(cookieNames.csrf, csrf, 600));
  writeJSON(response, 200, {
    data: {
      flow: "login",
      csrf_token: csrf,
      continuation: { available: true, product_name: config.productName },
    },
    request_id: "req_e2e_continuation_bootstrap",
  });
};

const handlePasswordLogin = async (request, response) => {
  const startedAt = Date.now();
  const form = new URLSearchParams((await readBody(request)).toString("utf8"));
  const requestCookies = cookies(request);
  if (
    request.headers["x-henukit-form-response"] !== "status" ||
    !form.get("csrf_token") ||
    form.get("csrf_token") !== requestCookies[cookieNames.csrf] ||
    form.get("email") !== config.testEmail ||
    form.get("password") !== config.testPassword
  ) {
    recordEvent(config.clientID, "authentication_failed", startedAt);
    writeJSON(response, 401, {
      error: {
        code: "AUTHENTICATION_FAILED",
        message: "authentication failed",
      },
      request_id: "req_e2e_authentication_failed",
    });
    return;
  }
  recordEvent(config.clientID, "authentication_succeeded", startedAt);
  response.writeHead(204, {
    "Set-Cookie": browserCookie(cookieNames.session, randomToken(32), 1800),
  });
  response.end();
};

const handleResume = async (request, response) => {
  const startedAt = Date.now();
  const form = new URLSearchParams((await readBody(request)).toString("utf8"));
  const requestCookies = cookies(request);
  if (
    !form.get("csrf_token") ||
    form.get("csrf_token") !== requestCookies[cookieNames.csrf] ||
    !requestCookies[cookieNames.session]
  ) {
    recordEvent(config.clientID, "resume_rejected", startedAt);
    response.writeHead(303, {
      Location: `${config.portalOrigin}/account/login?continuation_error=expired&request_id=req_e2e_resume_rejected`,
      "Referrer-Policy": "no-referrer",
    });
    response.end();
    return;
  }
  const handle = form.get("continuation");
  const stored = activeContinuation(handle);
  if (!stored || stored.device !== requestCookies[cookieNames.device]) {
    recordEvent(config.clientID, "continuation_unavailable", startedAt);
    response.writeHead(303, {
      Location: `${config.portalOrigin}/account/login?continuation_error=expired&request_id=req_e2e_resume_unavailable`,
      "Referrer-Policy": "no-referrer",
    });
    response.end();
    return;
  }
  continuations.delete(handle);
  const code = randomToken(32);
  codes.set(code, stored);
  const callback = new URL(stored.redirectURI);
  callback.searchParams.set("code", code);
  callback.searchParams.set("state", stored.state);
  recordEvent(config.clientID, "authorization_code_issued", startedAt);
  response.writeHead(303, {
    Location: callback.toString(),
    "Referrer-Policy": "no-referrer",
  });
  response.end();
};

const handleExchange = async (request, response) => {
  const startedAt = Date.now();
  const body = await readBody(request);
  if (!hasValidServiceSignature(request, body)) {
    recordEvent(config.clientID, "exchange_rejected", startedAt);
    response.writeHead(401).end("unauthorized");
    return;
  }
  let payload;
  try {
    payload = JSON.parse(body.toString("utf8"));
  } catch {
    recordEvent(config.clientID, "exchange_rejected", startedAt);
    response.writeHead(400).end("invalid exchange");
    return;
  }
  if (
    payload.grant_type !== "authorization_code" ||
    payload.client_id !== config.clientID ||
    payload.redirect_uri !== config.redirectURI
  ) {
    recordEvent(config.clientID, "exchange_rejected", startedAt);
    response.writeHead(400).end("invalid exchange");
    return;
  }
  const stored = codes.get(payload.code);
  if (stored) codes.delete(payload.code);
  const challenge = createHash("sha256")
    .update(payload.code_verifier ?? "")
    .digest("base64url");
  if (!stored || challenge !== stored.challenge) {
    recordEvent(config.clientID, "exchange_rejected", startedAt);
    response.writeHead(400).end("invalid code");
    return;
  }
  const user = { user_id: config.userID };
  if (config.displayName) user.display_name = config.displayName;
  recordEvent(config.clientID, "product_session_issued", startedAt);
  writeJSON(response, 200, {
    data: {
      user,
      session_exchange_token: config.exchangeToken,
      expires_at: new Date(Date.now() + 8 * 60 * 60 * 1000).toISOString(),
    },
    request_id: "req_e2e_exchange",
  });
};

const handleAuthorizationCheck = async (request, response) => {
  const startedAt = Date.now();
  const body = await readBody(request);
  if (!hasValidServiceSignature(request, body)) {
    recordEvent(config.clientID, "authorization_rejected", startedAt);
    response.writeHead(401).end("unauthorized");
    return;
  }
  let payload;
  try {
    payload = JSON.parse(body.toString("utf8"));
  } catch {
    recordEvent(config.clientID, "authorization_rejected", startedAt);
    response.writeHead(400).end("invalid authorization check");
    return;
  }
  if (
    payload.session_exchange_token !== config.exchangeToken ||
    !payload.permission_code
  ) {
    recordEvent(config.clientID, "authorization_rejected", startedAt);
    response.writeHead(400).end("invalid authorization check");
    return;
  }
  recordEvent(config.clientID, "authorization_allowed", startedAt);
  writeJSON(response, 200, {
    data: { allowed: true },
    request_id: "req_e2e_authorization",
  });
};

const server = createServer(async (request, response) => {
  try {
    const url = new URL(request.url, `http://${config.address}`);
    if (url.pathname === "/healthz") {
      writeJSON(response, 200, { status: "ok" });
    } else if (url.pathname === "/__e2e/events") {
      const rawAfter = Number.parseInt(url.searchParams.get("after") ?? "0", 10);
      const after = Number.isSafeInteger(rawAfter)
        ? Math.max(0, Math.min(rawAfter, events.length))
        : 0;
      writeJSON(response, 200, {
        cursor: events.length,
        events: events.slice(after),
      });
    } else if (url.pathname === "/api/v1/oauth/authorize") {
      handleAuthorize(request, response, url);
    } else if (url.pathname === "/account/bootstrap") {
      handleBootstrap(request, response, url);
    } else if (url.pathname === "/login/password") {
      await handlePasswordLogin(request, response);
    } else if (url.pathname === "/account/continuation/resume") {
      await handleResume(request, response);
    } else if (url.pathname === "/api/v1/oauth/token") {
      await handleExchange(request, response);
    } else if (
      url.pathname === "/api/v1/authorization/check" &&
      config.authorizationMode === "allow"
    ) {
      await handleAuthorizationCheck(request, response);
    } else {
      response.writeHead(404).end("not found");
    }
  } catch {
    console.error("oauth_fixture_internal_error");
    if (!response.headersSent) response.writeHead(500);
    response.end("fixture failure");
  }
});

server.listen(Number(config.address.split(":").at(-1)), "127.0.0.1", () => {
  console.log(
    `OAuth continuation Platform fixture for ${config.clientID} listening on ${config.address}`
  );
});
