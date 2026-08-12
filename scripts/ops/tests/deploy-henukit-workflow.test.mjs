import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-henukit.yml", import.meta.url),
  "utf8",
);
const portalDockerfile = readFileSync(
  new URL("../../../apps/portal/Dockerfile", import.meta.url),
  "utf8",
);
const developmentCompose = readFileSync(
  new URL("../../../docker-compose.henukit.yml", import.meta.url),
  "utf8",
);
const exampleEnvironment = readFileSync(
  new URL("../../../.env.henukit.example", import.meta.url),
  "utf8",
);
const runtimePackager = readFileSync(
  new URL("../package-henukit-runtime.sh", import.meta.url),
  "utf8",
);
const imageInventory = fileURLToPath(
  new URL("../henukit-release-images.sh", import.meta.url),
);

function releaseImageMatrix() {
  return JSON.parse(
    execFileSync(imageInventory, ["--github-matrix"], { encoding: "utf8" }),
  );
}

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
    "henukit-notice",
    "henukit-notice-worker",
    "henukit-food",
    "henukit-library",
    "henukit-portal-gateway",
  ];

  assert.match(workflow, /release-image-matrix:/);
  assert.match(workflow, /scripts\/ops\/henukit-release-images\.sh --github-matrix/);
  assert.match(workflow, /fromJSON\(needs\.release-image-matrix\.outputs\.matrix\)/);
  assert.match(workflow, /docker build[\s\S]*--platform linux\/amd64/);
  assert.match(workflow, /\.Os}}\/{{\.Architecture}}.*linux\/amd64/);
  assert.deepEqual(
    releaseImageMatrix().include.map(({ image }) => image),
    expectedImages,
  );
  assert.deepEqual(
    execFileSync(imageInventory, ["--load-images"], { encoding: "utf8" })
      .trim()
      .split("\n"),
    expectedImages,
  );
});

test("CI runs the Account Portfolio browser behavior spec", () => {
  assert.match(workflow, /pnpm --filter @henukit\/portal test:e2e:account/);
});

test("CI runs the enabled QuizCraft V2 ranking behavior spec", () => {
  assert.match(workflow, /Verify enabled QuizCraft V2 stats and rankings/);
  assert.match(workflow, /pnpm --filter @henukit\/portal test:e2e:stats/);
});

test("release artifacts carry an exact-SHA Account mock-free boundary manifest", () => {
  assert.match(
    workflow,
    /scripts\/ops\/package-henukit-runtime\.sh --sha "\$GITHUB_SHA" --output-dir release/,
  );
});

test("Portal V2 cutover flags are enabled in production artifacts after HC-166", () => {
  assert.match(
    portalDockerfile,
    /ARG NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=0/,
  );
  assert.match(
    portalDockerfile,
    /ENV NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=\$NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS/,
  );
  assert.match(
    developmentCompose,
    /NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS:\s+\$\{NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS:-0\}/,
  );
  const portal = releaseImageMatrix().include.find(({ name }) => name === "portal");
  assert.match(portal.build_args, /NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1/);
  assert.match(portal.build_args, /NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=1/);
});

test("development Compose forwards an explicit Portal V2 read build flag", () => {
  const config = JSON.parse(
    execFileSync(
      "docker",
      [
        "compose",
        "-f",
        "docker-compose.henukit.yml",
        "config",
        "--format",
        "json",
        "--no-path-resolution",
      ],
      {
        cwd: new URL("../../../", import.meta.url),
        encoding: "utf8",
        env: {
          ...process.env,
          NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS: "1",
        },
      },
    ),
  );
  assert.equal(
    config.services.portal.build.args.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS,
    "1",
  );
});

test("Account Portfolio payment configuration is wired but disabled by default", () => {
  const config = JSON.parse(
    execFileSync(
      "docker",
      [
        "compose",
        "-f",
        "docker-compose.henukit.yml",
        "config",
        "--format",
        "json",
        "--no-path-resolution",
      ],
      {
        cwd: new URL("../../../", import.meta.url),
        encoding: "utf8",
        env: {
          ...process.env,
          ACCOUNT_PORTFOLIO_EASYPAY_ENABLED: "1",
          ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL: "https://metaview.top/epay",
          ACCOUNT_PORTFOLIO_EASYPAY_PID: "henukit-tenant",
          ACCOUNT_PORTFOLIO_EASYPAY_KEY: "independent-test-tenant-secret",
          ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL:
            "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
          ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL:
            "https://henukit.cn/account/membership",
        },
      },
    ),
  );
  assert.deepEqual(
    Object.fromEntries(
      Object.entries(config.services["account-portfolio"].environment).filter(
        ([name]) => name.startsWith("ACCOUNT_PORTFOLIO_EASYPAY_"),
      ),
    ),
    {
      ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL: "https://metaview.top/epay",
      ACCOUNT_PORTFOLIO_EASYPAY_ENABLED: "1",
      ACCOUNT_PORTFOLIO_EASYPAY_KEY: "independent-test-tenant-secret",
      ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL:
        "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
      ACCOUNT_PORTFOLIO_EASYPAY_PID: "henukit-tenant",
      ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL:
        "https://henukit.cn/account/membership",
    },
  );
  for (const name of [
    "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED",
    "ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL",
    "ACCOUNT_PORTFOLIO_EASYPAY_PID",
    "ACCOUNT_PORTFOLIO_EASYPAY_KEY",
    "ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL",
    "ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL",
  ]) {
    assert.match(exampleEnvironment, new RegExp(`^${name}=`, "m"));
  }
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
      "FOOD_SUMMARY_CLIENT_SECRET",
      "LIBRARY_DATABASE_URL",
      "LIBRARY_DOWNLOAD_CLIENT_ID",
      "LIBRARY_DOWNLOAD_CLIENT_SECRET",
      "LIBRARY_DOWNLOAD_KEY_ID",
      "LIBRARY_DOWNLOAD_URL",
      "LIBRARY_OSS_BUCKET",
      "LIBRARY_OSS_ECS_RAM_ROLE",
      "LIBRARY_OSS_INTERNAL_ENDPOINT",
      "LIBRARY_OSS_PUBLIC_ENDPOINT",
      "LIBRARY_OSS_REGION",
      "LIBRARY_REDIS_URL",
      "LIBRARY_SUMMARY_CLIENT_ID",
      "LIBRARY_SUMMARY_CLIENT_SECRET",
      "LIBRARY_SUMMARY_KEY_ID",
      "NOTICE_CLIENT_SECRET",
      "NOTICE_SUMMARY_CLIENT_ID",
      "NOTICE_SUMMARY_CLIENT_SECRET",
      "NOTICE_SUMMARY_KEY_ID",
      "NOTICE_DATABASE_URL",
      "NOTICE_REDIS_URL",
      "FOOD_DATABASE_URL",
      "FOOD_REDIS_URL",
      "FOOD_SUMMARY_CLIENT_ID",
      "FOOD_SUMMARY_CLIENT_SECRET",
      "FOOD_SUMMARY_KEY_ID",
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
      "PRACTICE_COMMAND_CLIENT_ID",
      "PRACTICE_COMMAND_CLIENT_SECRET",
      "PRACTICE_COMMAND_KEY_ID",
      "QUIZCRAFT_CORE_URL",
      "QUIZCRAFT_PORTAL_CATALOG_CLIENT_ID",
      "QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET",
      "QUIZCRAFT_PORTAL_CATALOG_KEY_ID",
      "QUIZCRAFT_SUMMARY_CLIENT_SECRET",
      "STUDY_LEGACY_ADMIN_TOKEN",
      "STUDY_LEGACY_API_URL",
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
            LIBRARY_API_URL: "http://library:8095",
            LIBRARY_DOWNLOAD_URL: "http://library:8095",
            PLATFORM_CORE_DATABASE_URL: "postgres://test",
            PLATFORM_CORE_REDIS_URL: "redis://test",
            ACCOUNT_PORTFOLIO_DATABASE_URL: "postgres://test",
            QUIZCRAFT_CORE_URL: "http://host.docker.internal:10089",
            QUIZCRAFT_DATABASE_URL: "postgres://test",
            RELEASE_SHA: releaseSha,
            STUDY_DATABASE_URL: "postgres://test",
            ...overrides,
          },
        },
      ),
    );
  const config = renderRuntimeConfig();
  assert.equal(
    config.services["console-gateway"].environment.LIBRARY_API_URL,
    "http://library:8095",
  );
  assert.equal(
    renderRuntimeConfig({ LIBRARY_API_URL: "" }).services["console-gateway"]
      .environment.LIBRARY_API_URL,
    "",
  );
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
    "henukit-notice",
    "henukit-notice-worker",
    "henukit-food",
    "henukit-library",
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
    config.services["account-portfolio"].environment.ACCOUNT_PORTFOLIO_EASYPAY_ENABLED,
    "0",
    "fixed-SHA runtime must keep the payment Provider disabled by default",
  );
  assert.equal(
    config.services["portal-gateway"].environment.ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET,
    "1",
    "production Portal Gateway must reject the default Account Portfolio client-secret placeholder",
  );
  assert.equal(
    config.services["portal-gateway"].environment.PRACTICE_SERVICE_URL,
    "http://host.docker.internal:10089",
    "the fixed-SHA Gateway catalog seam must call QuizCraft Core, not legacy Portal API",
  );
  assert.deepEqual(
    config.services["portal-gateway"].extra_hosts,
    ["host.docker.internal=host-gateway"],
    "the containerized Gateway must have an explicit private route to the host Core",
  );
  assert.equal(
    config.services["portal-gateway"].environment.PRACTICE_CLIENT_SECRET,
    config.services["portal-gateway"].environment.QUIZCRAFT_PORTAL_CATALOG_CLIENT_SECRET,
    "the catalog seam and V2 read client must share the dedicated Core read credential",
  );
  assert.equal(
    config.services["portal-gateway"].environment.PORTAL_PRACTICE_COMMANDS_ENABLED,
    "0",
    "the fixed-SHA runtime must keep Practice writes closed until the #166 commitment point",
  );
  assert.equal(
    config.services["portal-gateway"].environment.PRACTICE_COMMAND_CLIENT_SECRET,
    "test-required-value",
    "the fixed-SHA runtime must carry the separately provisioned Practice command credential",
  );
  assert.equal(
    Object.hasOwn(config.services["portal-gateway"].environment, "LIBRARY_CLIENT_SECRET"),
    false,
    "the fixed-SHA runtime must not force the retired Library gateway client secret",
  );
  assert.equal(
    Object.hasOwn(config.services["portal-gateway"].environment, "FOOD_CLIENT_SECRET"),
    false,
    "the fixed-SHA runtime must not force the retired Food gateway client secret",
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
  assert.equal(
    config.services["console-gateway"].environment.NOTICE_API_URL,
    "http://notice:8094",
    "production Console must use the private Notice owner endpoint by default",
  );
  assert.equal(
    config.services["console-gateway"].environment.FOOD_API_URL,
    "http://food:8096",
    "production Console Gateway must use the private Food owner endpoint",
  );
  const withoutOwnerEndpoints = renderRuntimeConfig({ NOTICE_API_URL: "", FOOD_API_URL: "" });
  assert.equal(
    withoutOwnerEndpoints.services["console-gateway"].environment.NOTICE_API_URL,
    "",
    "an explicit empty Notice owner endpoint must disable the module",
  );
  assert.equal(
    withoutOwnerEndpoints.services["console-gateway"].environment.FOOD_API_URL,
    "",
    "an explicit empty Food owner endpoint must disable the module",
  );
  const withOwnerEndpoints = renderRuntimeConfig({ NOTICE_API_URL: "https://notice.internal", FOOD_API_URL: "http://food:8096" });
  assert.equal(
    withOwnerEndpoints.services["console-gateway"].environment.NOTICE_API_URL,
    "https://notice.internal",
    "Console must use the private Notice owner endpoint when the env enables it",
  );
  assert.equal(
    withOwnerEndpoints.services["console-gateway"].environment.FOOD_API_URL,
    "http://food:8096",
    "Console must use the private Food owner endpoint when the env enables it",
  );
  assert.equal(
    config.services["console-gateway"].environment.NOTICE_SUMMARY_URL,
    "",
    "Notice summary stays unset until the production env enables it",
  );
  assert.equal(
    config.services["notice"].environment.NOTICE_SERVICE_CLIENT_ID,
    config.services["console-gateway"].environment.NOTICE_SUMMARY_CLIENT_ID,
    "Notice must verify the exact Gateway caller identity",
  );
  assert.equal(
    config.services["notice"].environment.NOTICE_SERVICE_KEY_ID,
    config.services["console-gateway"].environment.NOTICE_SUMMARY_KEY_ID,
    "Notice must verify the exact Gateway key id",
  );
  assert.equal(
    config.services["notice"].environment.NOTICE_SERVICE_SECRET,
    config.services["console-gateway"].environment.NOTICE_SUMMARY_CLIENT_SECRET,
    "Notice must verify the exact Gateway caller secret",
  );
  assert.equal(
    config.services["food"].environment.FOOD_SERVICE_CLIENT_ID,
    config.services["console-gateway"].environment.FOOD_SUMMARY_CLIENT_ID,
    "Food must verify the exact Gateway caller identity",
  );
  assert.equal(
    config.services["food"].environment.FOOD_SERVICE_KEY_ID,
    config.services["console-gateway"].environment.FOOD_SUMMARY_KEY_ID,
    "Food must verify the exact Gateway key id",
  );
  assert.equal(
    config.services["food"].environment.FOOD_SERVICE_SECRET,
    config.services["console-gateway"].environment.FOOD_SUMMARY_CLIENT_SECRET,
    "Food must verify the exact Gateway caller secret",
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
  assert.match(workflow, /scripts\/ops\/package-henukit-runtime\.sh --sha "\$GITHUB_SHA" --output-dir release/);
  assert.match(
    runtimePackager,
    /config --no-interpolate --no-path-resolution > "\$runtime\/docker-compose\.henukit\.release\.yml"[\s\S]*infra\/nginx\/henukit\.conf\.example/,
  );
  assert.match(runtimePackager, /deploy-henukit-artifact\.sh/);
  assert.match(runtimePackager, /watch-henukit-actions\.sh/);
  assert.match(runtimePackager, /henukit-release-images\.sh/);
  assert.match(runtimePackager, /verify-henukit-local-release\.sh/);
  assert.match(runtimePackager, /infra\/systemd\/henukit-actions-watch\.service/);
  assert.match(runtimePackager, /migrations\/platform-core/);
  assert.match(runtimePackager, /migrations\/account-portfolio/);
  assert.doesNotMatch(
    workflow,
    /cp docker-compose\.henukit\.yml|cp docker-compose\.henukit\.prebuilt\.yml|init-henukit-dbs\.sh/,
  );
  assert.match(
    runtimePackager,
    /cp "\$repo_root"\/services\/platform-core\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry the registration migration",
  );
  assert.match(
    runtimePackager,
    /cp "\$repo_root"\/services\/account-portfolio\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Account Portfolio recovery migrations",
  );
  assert.match(
    runtimePackager,
    /cp "\$repo_root"\/services\/notice\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Notice recovery migrations",
  );
  assert.match(
    runtimePackager,
    /cp "\$repo_root"\/services\/food\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Food recovery migrations",
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
