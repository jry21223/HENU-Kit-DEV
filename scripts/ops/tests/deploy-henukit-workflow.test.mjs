import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-henukit.yml", import.meta.url),
  "utf8",
);

test("CI builds the primary HENU runtime without legacy Study or QuizCraft images", () => {
  const expectedImages = [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-api",
    "henukit-account-portfolio",
    "henukit-portal-gateway",
  ];

  for (const image of expectedImages) {
    assert.match(workflow, new RegExp(`image: ${image.replaceAll("-", "\\-")}`));
  }
  assert.doesNotMatch(workflow, /image: henukit-study/);
  assert.doesNotMatch(workflow, /image: henukit-quizcraft/);
  assert.doesNotMatch(workflow, /VITE_QUIZCRAFT_WORKSHOP_URL=\/quiz/);
});

test("CI runs the Account Portfolio browser behavior spec", () => {
  assert.match(workflow, /pnpm --filter @henukit\/portal test:e2e:account/);
});

test("every Docker image artifact includes an independent SHA-256 checksum", () => {
  assert.match(
    workflow,
    /sha256sum "\$\{IMAGE\}-\$\{GITHUB_SHA\}\.docker\.tar\.gz" > "\$\{IMAGE\}-\$\{GITHUB_SHA\}\.docker\.tar\.gz\.sha256"/,
  );
  assert.match(
    workflow,
    /path: \|-\s+release\/\$\{\{ matrix\.image \}\}-\$\{\{ github\.sha \}\}\.docker\.tar\.gz\s+release\/\$\{\{ matrix\.image \}\}-\$\{\{ github\.sha \}\}\.docker\.tar\.gz\.sha256/,
  );
});

test("runtime artifact starts HENU images without compiling or replacing Study", () => {
  const repoRoot = new URL("../../../", import.meta.url);
  const releaseSha = "a".repeat(40);
  const requiredEnvironment = Object.fromEntries(
    [
      "CONSOLE_PLATFORM_CLIENT_SECRET",
      "CONSOLE_SESSION_KEY",
      "ACCOUNT_PORTFOLIO_CLIENT_ID",
      "ACCOUNT_PORTFOLIO_CLIENT_SECRET",
      "ACCOUNT_PORTFOLIO_KEY_ID",
      "ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID",
      "ACCOUNT_PORTFOLIO_CONSOLE_SECRET",
      "ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID",
      "ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY",
      "FOOD_CLIENT_SECRET",
      "FOOD_SUMMARY_CLIENT_SECRET",
      "LIBRARY_CLIENT_SECRET",
      "LIBRARY_SUMMARY_CLIENT_SECRET",
      "NOTICE_CLIENT_SECRET",
      "NOTICE_SUMMARY_CLIENT_SECRET",
      "PLATFORM_CLIENT_SECRET",
      "PLATFORM_CORE_IDEMPOTENCY_KEY",
      "PLATFORM_CORE_MAIL_DELIVERY_TOKEN",
      "PLATFORM_CORE_MAIL_PROVIDER_TOKEN",
      "PLATFORM_CORE_SMTP_ADDRESS",
      "PLATFORM_CORE_SMTP_FROM",
      "PLATFORM_CORE_SMTP_PASSWORD",
      "PLATFORM_CORE_SMTP_USERNAME",
      "PLATFORM_CORE_VERIFICATION_KEY",
      "PLATFORM_SUMMARY_CLIENT_SECRET",
      "PORTAL_SESSION_KEY",
      "PORTAL_SUMMARY_CLIENT_SECRET",
      "POSTGRES_DB",
      "POSTGRES_PASSWORD",
      "POSTGRES_USER",
      "PRACTICE_CLIENT_SECRET",
      "QUIZCRAFT_SUMMARY_CLIENT_SECRET",
    ].map((name) => [name, "test-required-value"]),
  );
  const renderRuntimeConfig = (overrides = {}) =>
    JSON.parse(
      execFileSync(
        "docker",
        [
          "compose",
          "-f",
          "docker-compose.henukit.yml",
          "-f",
          "docker-compose.henukit.prebuilt.yml",
          "config",
          "--format",
          "json",
          "--no-path-resolution",
        ],
        {
          cwd: repoRoot,
          encoding: "utf8",
          env: {
            ...process.env,
            ...requiredEnvironment,
            PLATFORM_CORE_DATABASE_URL: "postgres://test",
            PLATFORM_CORE_REDIS_URL: "redis://test",
            ACCOUNT_PORTFOLIO_DATABASE_URL: "postgres://test",
            QUIZCRAFT_DATABASE_URL: "postgres://test",
            RELEASE_SHA: releaseSha,
            STUDY_DATABASE_URL: "postgres://test",
            ...overrides,
          },
        },
      ),
    );
  const config = renderRuntimeConfig();
  const expectedImages = [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-api",
    "henukit-account-portfolio",
    "henukit-portal-gateway",
  ];

  for (const image of expectedImages) {
    assert.equal(
      Object.values(config.services).some(
        (service) => service.image === `${image}:${releaseSha}`,
      ),
      true,
      `${image} must use RELEASE_SHA`,
    );
  }
  assert.equal(
    Object.values(config.services).some((service) => "build" in service),
    false,
  );
  assert.equal(
    Object.keys(config.services).some((service) => /^(study|quizcraft)/.test(service)),
    false,
  );
  assert.equal(
    config.services.nginx.volumes[0].source,
    "./infra/nginx/henukit.conf.example",
  );
  assert.equal(
    config.services["console-gateway"].environment.PLATFORM_ACCOUNT_ORIGIN,
    "http://localhost:8088/account-auth",
    "the local Console login must use the browser-facing Account Center path",
  );
  assert.notEqual(
    config.services["console-gateway"].environment.PLATFORM_ACCOUNT_ORIGIN,
    config.services["console-gateway"].environment.PLATFORM_CORE_URL,
    "the Console browser redirect must not receive the private Core URL",
  );
  assert.equal(
    config.services["account-portfolio"].environment.ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET,
    "1",
    "production Account Portfolio must reject the default client-secret placeholder",
  );
  assert.equal(
    config.services["portal-gateway"].environment.ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET,
    "1",
    "production Portal Gateway must reject the default Account Portfolio client-secret placeholder",
  );
  assert.equal(
    config.services["account-portfolio"].environment.ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID,
    "test-required-value",
    "production Account Portfolio must require the distinct Console caller identity",
  );
  assert.equal(
    config.services["account-portfolio"].environment.ACCOUNT_PORTFOLIO_CONSOLE_SECRET,
    "test-required-value",
    "production Account Portfolio must require the distinct Console caller secret",
  );
  assert.equal(
    config.services["account-portfolio"].environment.ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY,
    "test-required-value",
    "production Account Portfolio must require its independent encrypted point-cursor key",
  );
  assert.equal(
    Object.hasOwn(config.services["portal-gateway"].environment, "ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY"),
    false,
    "the point-cursor key must stay in the Account Portfolio owner environment",
  );
  assert.equal(
    Object.hasOwn(config.services["console-gateway"].environment, "ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY"),
    false,
    "Console must not receive the point-cursor key",
  );
  assert.throws(
    () => renderRuntimeConfig({ ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY: "" }),
    /ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY is required/,
    "production Compose must fail closed without the independent point-cursor key",
  );
  assert.equal(
    config.services["console-gateway"].environment.ACCOUNT_PORTFOLIO_CONSOLE_SECRET,
    "test-required-value",
    "production Console Gateway must receive its Account Portfolio caller secret",
  );
  assert.equal(
    config.services["console-gateway"].environment.ACCOUNT_PORTFOLIO_API_URL,
    "http://account-portfolio:8097",
    "Console must use the private Account Portfolio owner endpoint",
  );
  assert.equal(
    config.services["console-gateway"].environment.ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET,
    "1",
    "production Console Gateway must reject the default Account Portfolio caller-secret placeholder",
  );
  const publicConfig = renderRuntimeConfig({
    PLATFORM_ACCOUNT_ORIGIN: "https://henukit.cn/account-auth",
  });
  assert.equal(
    publicConfig.services["console-gateway"].environment.PLATFORM_ACCOUNT_ORIGIN,
    "https://henukit.cn/account-auth",
    "production can provide the public Account Center URL explicitly",
  );
  assert.doesNotMatch(
    JSON.stringify(config),
    /henukit_dev_change_me|replace-[a-z]|0123456789abcdef|cUUpjiEH/,
  );
  assert.match(workflow, /name: henukit-runtime-\$\{\{ github\.sha \}\}/);
  assert.match(
    workflow,
    /config --no-interpolate --no-path-resolution > "\$runtime\/docker-compose\.henukit\.release\.yml"[\s\S]*infra\/nginx\/henukit\.conf\.example/,
  );
  assert.match(workflow, /install -m 0555 scripts\/ops\/deploy-henukit-artifact\.sh/);
  assert.match(workflow, /install -m 0555 scripts\/ops\/watch-henukit-actions\.sh/);
  assert.match(workflow, /infra\/systemd\/henukit-actions-watch\.service/);
  assert.match(workflow, /migrations\/platform-core/);
  assert.match(workflow, /migrations\/account-portfolio/);
  assert.doesNotMatch(
    workflow,
    /cp docker-compose\.henukit\.yml|cp docker-compose\.henukit\.prebuilt\.yml|init-henukit-dbs\.sh/,
  );
  assert.match(
    workflow,
    /cp services\/platform-core\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry the registration migration",
  );
  assert.match(
    workflow,
    /cp services\/account-portfolio\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Account Portfolio recovery migrations",
  );
});

test("production Compose fails closed without release identity and secrets", () => {
  assert.throws(
    () =>
      execFileSync(
        "docker",
        [
          "compose",
          "-f",
          "docker-compose.henukit.yml",
          "-f",
          "docker-compose.henukit.prebuilt.yml",
          "config",
          "--quiet",
        ],
        {
          cwd: new URL("../../../", import.meta.url),
          env: { PATH: process.env.PATH },
          stdio: "pipe",
        },
      ),
    /Command failed/,
  );
});
