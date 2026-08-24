const challenge = "c".repeat(43);
const unsupportedClientID = "retired-practice-client";
const unsupportedRedirectURI = "https://practice.henukit.test/auth/callback";
const forbiddenPublicCopy = [
  /QuizCraft/i,
  /\bQC-[A-Za-z0-9_-]*/i,
  /\bV2\b/i,
  /题库工坊/,
  /后台口令/
];
function authorizeURL(product, overrides = {}) {
  const query = new URLSearchParams({
    response_type: "code",
    client_id: overrides.clientID ?? product.clientID,
    redirect_uri: overrides.redirectURI ?? product.redirectURI,
    state: overrides.state ?? `state_${product.name.toLowerCase()}_fresh_01`,
    code_challenge: challenge,
    code_challenge_method: "S256"
  });
  return `${product.platformOrigin}/api/v1/oauth/authorize?${query}`;
}
async function expectAccountCenter(page, product) {
  const { expect } = product;
  const escapedOrigin = product.accountCenterOrigin.replace(
    /[.*+?^${}()|[\]\\]/g,
    "\\$&"
  );
  await expect(page).toHaveURL(
    new RegExp(
      `^${escapedOrigin}/account/login\\?continuation=[A-Za-z0-9_-]+$`
    )
  );
  const url = new URL(page.url());
  expect([...url.searchParams.keys()]).toEqual(["continuation"]);
  const handle = url.searchParams.get("continuation") ?? "";
  expect(handle).toMatch(/^[A-Za-z0-9_-]{32,}$/);
  await expect(
    page.getByText(`登录后继续前往 ${product.productName}`)
  ).toBeVisible();
  await expect(page.getByText("ACC-01")).toBeVisible();
  await expectAccountCenterStructure(page, product);
  await expectNoRetiredPublicCopy(page, product);
  await expectDOMNoSecrets(page, product, [handle, product.redirectURI]);
  return { url: url.toString(), handle };
}
async function expectAccountCenterStructure(page, product) {
  const { expect } = product;
  await expect(page.getByText("ACC-01")).toBeVisible();
  await expect(
    page.locator(
      'form[action], input[name="return_to"], input[name="csrf_token"]',
    ),
  ).toHaveCount(0);
}
async function startProductContinuation(page, product) {
  await page.addInitScript(() => window.localStorage.clear());
  await product.start(page);
  return expectAccountCenter(page, product);
}
async function finishPasswordLogin(page, product) {
  await page.getByRole("button", { name: "密码登录" }).click();
  await page.getByLabel("学校邮箱").fill(product.emailLocalPart);
  await page.getByLabel("密码 / PASSWORD").fill(product.password);
  await page.getByLabel("密码 / PASSWORD").press("Enter");
  await expectProductSession(page, product);
}
async function expectProductSession(page, product) {
  const { expect } = product;
  await expect(page).toHaveURL(product.finalURL);
  const session = await page.evaluate(async () => {
    const response = await fetch("/api/v1/session", {
      credentials: "include",
      cache: "no-store"
    });
    return { status: response.status, body: await response.json() };
  });
  expect(session.status).toBe(200);
  expect(session.body).toMatchObject(product.expectedSession);
}
async function expectNoRetiredPublicCopy(page, product) {
  const { expect } = product;
  const text = await page.locator("body").innerText();
  for (const pattern of forbiddenPublicCopy) {
    expect(text, `visible copy matched ${pattern}`).not.toMatch(pattern);
  }
}
async function expectDOMNoSecrets(page, product, secrets) {
  const currentURL = new URL(page.url());
  const currentContinuation =
    currentURL.origin === product.accountCenterOrigin &&
    currentURL.pathname === "/account/login" &&
    [...currentURL.searchParams.keys()].every((key) => key === "continuation")
      ? currentURL.searchParams.get("continuation")
      : null;
  const serializedDOM = await page.content();
  assertSerializedDOMNoSecrets(serializedDOM, secrets, {
    currentContinuation,
  });
}
function redactExactContinuationHydration(serializedDOM, handle) {
  if (!handle) return serializedDOM;
  const fragments = [
    String.raw`\"c\":[\"\",\"account\",\"login?continuation=${handle}\"],\"q\":\"?continuation=${handle}\"`,
    String.raw`\"children\":[\"__PAGE__?{\\\"continuation\\\":\\\"${handle}\\\"}\"`,
    String.raw`\"serverProvidedParams\":{\"searchParams\":{\"continuation\":\"${handle}\"},\"params\":{},\"promises\":null}`,
  ];
  let redacted = serializedDOM;
  for (const fragment of fragments) {
    redacted = redacted.replaceAll(
      fragment,
      fragment.replaceAll(handle, "[current-continuation]"),
    );
  }
  return redacted;
}
function assertSerializedDOMNoSecrets(serializedDOM, secrets, options = {}) {
  const inspectedDOM = redactExactContinuationHydration(
    serializedDOM,
    options.currentContinuation,
  );
  for (const [secretIndex, secret] of secrets.filter(Boolean).entries()) {
    const variants = [
      ["raw", secret],
      ["uri", encodeURI(secret)],
      ["component", encodeURIComponent(secret)],
    ];
    for (const [encoding, variant] of variants) {
      if (inspectedDOM.includes(variant)) {
        throw new Error(
          `serialized DOM contains an OAuth continuation secret (index ${secretIndex}, ${encoding})`,
        );
      }
    }
  }
}
function assertPublicRouteNavigation({ status, finalURL, expectedURL }) {
  if (status < 200 || status >= 400) {
    throw new Error(`public route returned status ${status}`);
  }
  if (finalURL !== expectedURL) {
    throw new Error(
      `public route reached unexpected URL ${finalURL}; expected ${expectedURL}`,
    );
  }
}
async function expectUnavailableRecovery(page, product, secrets = []) {
  const { expect } = product;
  await expect(
    page.getByRole("heading", { name: "登录链接已过期或不可继续" })
  ).toBeVisible();
  await expect(
    page.getByText(
      "这次登录无法继续。请重新开始登录，我们会为你创建一条新的安全链接。"
    )
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "重新开始登录" })).toBeVisible();
  await expectNoRetiredPublicCopy(page, product);
  await expectDOMNoSecrets(page, product, secrets);
}
async function eventCursor(request, product) {
  const { expect } = product;
  const response = await request.get(`${product.platformOrigin}/__e2e/events`);
  expect(response.status()).toBe(200);
  const envelope = await response.json();
  expect(envelope.cursor).toBeGreaterThanOrEqual(0);
  return envelope.cursor;
}
async function expectSafeEvents(request, product, after, expectedOutcomes, secrets) {
  const { expect } = product;
  const response = await request.get(`${product.platformOrigin}/__e2e/events?after=${after}`);
  expect(response.status()).toBe(200);
  const envelope = await response.json();
  expect(envelope.events.length).toBeGreaterThan(0);
  const allowedClientIDs = new Set([
    product.clientID,
    unsupportedClientID,
    "unknown-client"
  ]);
  for (const event of envelope.events) {
    expect(Object.keys(event).sort()).toEqual([
      "client_id",
      "duration_ms",
      "outcome",
      "request_id"
    ]);
    expect(event.request_id).toMatch(/^req_[A-Za-z0-9_-]+$/);
    expect(allowedClientIDs.has(event.client_id)).toBe(true);
    expect(event.outcome).toMatch(/^[a-z0-9_]+$/);
    expect(event.duration_ms).toBeGreaterThanOrEqual(0);
  }
  const outcomes = new Set(envelope.events.map((event) => event.outcome));
  for (const outcome of expectedOutcomes) {
    expect(outcomes.has(outcome), `missing current-scenario outcome ${outcome}`).toBe(true);
  }
  const serialized = JSON.stringify(envelope);
  for (const secret of secrets.filter(Boolean)) {
    expect(serialized).not.toContain(secret);
  }
}
async function expectNoBrowserLeakage(page, product, observed, continuationHandle) {
  const { expect } = product;
  const callbackPath = new URL(product.redirectURI).pathname;
  const authorize = observed.find((item) => {
    const url = new URL(item.url);
    return url.origin === product.platformOrigin && url.pathname === "/api/v1/oauth/authorize";
  });
  const callback = observed.find((item) => {
    const url = new URL(item.url);
    return url.origin === product.productOrigin && url.pathname === callbackPath && url.searchParams.has("code");
  });
  expect(authorize).toBeDefined();
  expect(callback).toBeDefined();
  const authorizeURLValue = new URL(authorize.url);
  const callbackURLValue = new URL(callback.url);
  const state = authorizeURLValue.searchParams.get("state") ?? "";
  const codeChallenge = authorizeURLValue.searchParams.get("code_challenge") ?? "";
  const code = callbackURLValue.searchParams.get("code") ?? "";
  expect(state.length).toBeGreaterThanOrEqual(8);
  expect(codeChallenge).toHaveLength(43);
  expect(code.length).toBeGreaterThanOrEqual(32);
  const trustedOrigins = /* @__PURE__ */ new Set([
    product.platformOrigin,
    product.accountCenterOrigin,
    product.productOrigin
  ]);
  for (const item of observed) {
    const url = new URL(item.url);
    for (const secret of [
      state,
      codeChallenge,
      code,
      continuationHandle,
      product.redirectURI
    ]) {
      expect(item.referer).not.toContain(secret);
    }
    if (item !== authorize) expect(item.url).not.toContain(codeChallenge);
    if (item !== authorize && item !== callback) {
      expect(item.url).not.toContain(state);
    }
    if (item !== callback) expect(item.url).not.toContain(code);
    if (!trustedOrigins.has(url.origin)) {
      for (const secret of [continuationHandle, product.redirectURI]) {
        expect(item.url).not.toContain(secret);
        expect(item.referer).not.toContain(secret);
      }
    }
  }
  expect(page.url()).toBe(product.finalURL);
  const finalSecrets = [
    state,
    codeChallenge,
    code,
    continuationHandle,
    product.redirectURI
  ];
  for (const secret of finalSecrets) {
    expect(page.url()).not.toContain(secret);
  }
  await expectDOMNoSecrets(page, product, finalSecrets);
  expect(await page.evaluate(() => document.referrer)).not.toContain(code);
  return finalSecrets;
}
async function directContinuation(page, product, state) {
  await page.goto(authorizeURL(product, { state }));
  const continuation = await expectAccountCenter(page, product);
  await expectDOMNoSecrets(page, product, [
    state,
    challenge,
    product.redirectURI,
    continuation.handle
  ]);
  return continuation;
}
async function expectBrowserBindingFailure(browser, product, accountCenterURL) {
  const otherContext = await browser.newContext();
  const otherPage = await otherContext.newPage();
  try {
    await otherPage.goto(accountCenterURL);
    await expectUnavailableRecovery(otherPage, product, [
      new URL(accountCenterURL).searchParams.get("continuation"),
      product.redirectURI
    ]);
  } finally {
    await otherContext.close();
  }
}
function defineOAuthContinuationJourney(productConfig, playwright) {
  const product = { ...productConfig, expect: playwright.expect };
  const { expect, test } = playwright;
  test.describe(`${product.name} cumulative Account Center continuation`, () => {
    test("fresh authentication returns to the exact product path without browser or log leakage", async ({
      page,
      request
    }) => {
      const cursor = await eventCursor(request, product);
      const observed = [];
      page.on("request", (browserRequest) => {
        observed.push({
          url: browserRequest.url(),
          referer: browserRequest.headers()["referer"] ?? ""
        });
      });
      const continuation = await startProductContinuation(page, product);
      const authorize = observed.find((item) => {
        const url = new URL(item.url);
        return url.origin === product.platformOrigin && url.pathname === "/api/v1/oauth/authorize";
      });
      expect(authorize).toBeDefined();
      const authorizeValue = new URL(authorize.url);
      await expectDOMNoSecrets(page, product, [
        authorizeValue.searchParams.get("state"),
        authorizeValue.searchParams.get("code_challenge"),
        product.redirectURI,
        continuation.handle
      ]);
      await finishPasswordLogin(page, product);
      const secrets = await expectNoBrowserLeakage(
        page,
        product,
        observed,
        continuation.handle
      );
      await expectSafeEvents(request, product, cursor, [
        "continuation_created",
        "continuation_bootstrapped",
        "authentication_succeeded",
        "authorization_code_issued",
        "product_session_issued"
      ], [...secrets, product.password]);
      await expectNoRetiredPublicCopy(page, product);
      for (const route of product.publicRoutes) {
        const isolatedContext =
          route.isolatedSignedOut === true
            ? await page.context().browser()?.newContext()
            : null;
        const routePage = isolatedContext
          ? await isolatedContext.newPage()
          : page;
        const requestedURL = `${product.productOrigin}${route.path}`;
        const expectedURL = `${product.productOrigin}${route.expectedPath ?? route.path}`;
        try {
          const response = await routePage.goto(requestedURL, {
            waitUntil: "domcontentloaded"
          });
          expect(response, `missing navigation response for ${route.path}`).not.toBeNull();
          assertPublicRouteNavigation({
            status: response?.status() ?? 0,
            finalURL: routePage.url(),
            expectedURL
          });
          await expect(
            routePage.locator(route.readySelector),
            `ready marker for ${route.path}`,
          ).toBeVisible();
          await routePage.waitForLoadState("load");
          await expect(
            routePage.locator('[aria-busy="true"]'),
            `settled state for ${route.path}`,
          ).toHaveCount(0);
          if (route.accountCenter === true) {
            await expectAccountCenterStructure(routePage, product);
          }
          await expectNoRetiredPublicCopy(routePage, product);
        } finally {
          await isolatedContext?.close();
        }
      }
    });
    test("an existing Core Session takes the authorize fast path without reopening Account Center", async ({
      page
    }) => {
      await startProductContinuation(page, product);
      await finishPasswordLogin(page, product);
      const cursor = await eventCursor(page.request, product);
      const observed = [];
      page.on("request", (browserRequest) => observed.push(browserRequest.url()));
      await product.restart(page);
      await expect(page).toHaveURL(product.finalURL);
      expect(
        observed.some((value) => {
          const url = new URL(value);
          return url.origin === product.accountCenterOrigin && url.pathname === "/account/login";
        })
      ).toBe(false);
      expect(
        observed.some((value) => {
          const url = new URL(value);
          return url.origin === product.productOrigin && url.pathname === new URL(product.redirectURI).pathname && url.searchParams.has("code");
        })
      ).toBe(true);
      await expectSafeEvents(page.request, product, cursor, [
        "core_session_fast_path",
        "product_session_issued"
      ], []);
    });
    test("a consumed continuation cannot be replayed", async ({ page, request }) => {
      const continuation = await startProductContinuation(page, product);
      await finishPasswordLogin(page, product);
      const cursor = await eventCursor(request, product);
      await page.goto(continuation.url);
      await expectUnavailableRecovery(page, product, [
        continuation.handle,
        product.redirectURI
      ]);
      await expectSafeEvents(request, product, cursor, ["continuation_unavailable"], [
        continuation.handle,
        product.redirectURI
      ]);
    });
    test("tampered and browser-mismatched handles fail closed", async ({
      page,
      browser,
      request
    }) => {
      const cursor = await eventCursor(request, product);
      const continuation = await directContinuation(
        page,
        product,
        `state_${product.name.toLowerCase()}_binding_01`
      );
      const finalCharacter = continuation.handle.endsWith("A") ? "B" : "A";
      const tamperedHandle = `${continuation.handle.slice(0, -1)}${finalCharacter}`;
      await page.goto(
        `${product.accountCenterOrigin}/account/login?continuation=${tamperedHandle}`
      );
      await expectUnavailableRecovery(page, product, [
        continuation.handle,
        tamperedHandle,
        product.redirectURI,
        challenge
      ]);
      const bound = await directContinuation(
        page,
        product,
        `state_${product.name.toLowerCase()}_binding_02`
      );
      await expectBrowserBindingFailure(browser, product, bound.url);
      await expectSafeEvents(request, product, cursor, ["continuation_unavailable"], [
        continuation.handle,
        bound.handle,
        product.redirectURI,
        challenge
      ]);
    });
    test("a real short TTL expires before Account Center bootstrap", async ({
      page,
      request
    }) => {
      const cursor = await eventCursor(request, product);
      const state = `expires_soon_${product.name.toLowerCase()}_01`;
      await page.route("**/account-auth/account/bootstrap**", async (route) => {
        await new Promise((resolve) => setTimeout(resolve, 400));
        await route.continue();
      });
      await page.goto(
        authorizeURL(product, {
          state
        })
      );
      await expectUnavailableRecovery(page, product, [state, challenge, product.redirectURI]);
      await expectSafeEvents(request, product, cursor, [
        "continuation_created",
        "continuation_unavailable"
      ], [state, challenge, product.redirectURI]);
    });
    test("unknown, unsupported, and callback-mismatched authorize requests fail closed", async ({
      page,
      request
    }) => {
      const cursor = await eventCursor(request, product);
      const untrustedClientID = "untrusted-client-input";
      const unsupportedState = `state_${product.name.toLowerCase()}_unsupported`;
      const callbackMismatch = await request.get(
        authorizeURL(product, {
          redirectURI: "https://evil.example/auth/callback",
          state: `state_${product.name.toLowerCase()}_callback_mismatch`
        }),
        { maxRedirects: 0 }
      );
      expect(callbackMismatch.status()).toBe(400);
      expect(callbackMismatch.headers()["location"]).toBeUndefined();
      const unknown = await request.get(
        authorizeURL(product, {
          clientID: untrustedClientID,
          redirectURI: "https://unknown.example/auth/callback",
          state: `state_${product.name.toLowerCase()}_unknown`
        }),
        { maxRedirects: 0 }
      );
      expect(unknown.status()).toBe(400);
      expect(unknown.headers()["location"]).toBeUndefined();
      const unsupportedURL = authorizeURL(product, {
        clientID: unsupportedClientID,
        redirectURI: unsupportedRedirectURI,
        state: unsupportedState
      });
      const unsupported = await request.get(unsupportedURL, { maxRedirects: 0 });
      expect(unsupported.status()).toBe(303);
      const recovery = new URL(
        unsupported.headers()["location"] ?? "",
        product.platformOrigin
      );
      expect(`${recovery.origin}${recovery.pathname}`).toBe(
        `${product.accountCenterOrigin}/account/login`
      );
      expect(recovery.searchParams.get("continuation_error")).toBe("unsupported");
      expect(recovery.searchParams.get("request_id")).toMatch(
        /^req_[A-Za-z0-9_-]+$/
      );
      expect(unsupported.headers()["set-cookie"]).toBeUndefined();
      for (const secret of [unsupportedClientID, unsupportedRedirectURI, challenge]) {
        expect(recovery.toString()).not.toContain(secret);
      }
      await page.goto(unsupportedURL);
      await expect(
        page.getByRole("heading", { name: "此应用暂不支持统一登录" })
      ).toBeVisible();
      await expect(
        page.getByText("请返回原应用；如需继续使用，请联系该应用的维护者。")
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "返回上一步" })).toBeVisible();
      await expectNoRetiredPublicCopy(page, product);
      await expectDOMNoSecrets(page, product, [
        untrustedClientID,
        unsupportedClientID,
        unsupportedRedirectURI,
        challenge,
        unsupportedState
      ]);
      await expectSafeEvents(request, product, cursor, [
        "authorize_invalid",
        "unsupported_client_rejected"
      ], [untrustedClientID, unsupportedRedirectURI, challenge, unsupportedState]);
    });
    test("the 360px reduced-motion journey completes authentication and recovery by keyboard", async ({
      page,
      request
    }) => {
      const cursor = await eventCursor(request, product);
      await page.setViewportSize({ width: 360, height: 800 });
      await page.emulateMedia({ reducedMotion: "reduce" });
      const continuation = await startProductContinuation(page, product);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBe(true);
      const passwordMode = page.getByRole("button", { name: "密码登录" });
      await passwordMode.focus();
      await page.keyboard.press("Enter");
      const email = page.getByLabel("学校邮箱");
      await email.focus();
      await page.keyboard.insertText(product.emailLocalPart);
      const password = page.getByLabel("密码 / PASSWORD");
      await password.focus();
      await page.keyboard.insertText(product.password);
      await page.keyboard.press("Enter");
      await expectProductSession(page, product);
      await page.goto(continuation.url);
      await expectUnavailableRecovery(page, product, [
        continuation.handle,
        product.redirectURI
      ]);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBe(true);
      await page.keyboard.press("Tab");
      await expect(page.getByRole("link", { name: "重新开始登录" })).toBeFocused();
      await expectSafeEvents(request, product, cursor, [
        "authorization_code_issued",
        "product_session_issued",
        "continuation_unavailable"
      ], [continuation.handle, product.redirectURI, product.password]);
      await expectNoRetiredPublicCopy(page, product);
    });
  });
}
export {
  assertPublicRouteNavigation,
  assertSerializedDOMNoSecrets,
  defineOAuthContinuationJourney
};
