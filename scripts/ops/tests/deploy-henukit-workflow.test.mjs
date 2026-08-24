import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-henukit.yml", import.meta.url),
  "utf8",
);
const rootPackage = JSON.parse(
  readFileSync(new URL("../../../package.json", import.meta.url), "utf8"),
);
const portalDockerfile = readFileSync(
  new URL("../../../apps/portal/Dockerfile", import.meta.url),
  "utf8",
);
const developmentCompose = readFileSync(
  new URL("../../../docker-compose.henukit.yml", import.meta.url),
  "utf8",
);
const prebuiltCompose = readFileSync(
  new URL("../../../docker-compose.henukit.prebuilt.yml", import.meta.url),
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
const localBuilder = readFileSync(
  new URL("../build-henukit-release-local.sh", import.meta.url),
  "utf8",
);
const quickBuilder = readFileSync(
  new URL("../build-henukit-release-quick.sh", import.meta.url),
  "utf8",
);
const oauthGate = readFileSync(
  new URL("../oauth-continuation-release-gate.sh", import.meta.url),
  "utf8",
);
const actionsWatcher = readFileSync(
  new URL("../watch-henukit-actions.sh", import.meta.url),
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

test("CI builds the primary HENU runtime without legacy Study or standalone QuizCraft images", () => {
  const expectedImages = [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-summary",
    "henukit-portal-api",
    "henukit-account-portfolio",
    "henukit-notice",
    "henukit-notice-worker",
    "henukit-food",
    "henukit-food-mcp",
    "henukit-library",
    "henukit-career-opportunities",
    "henukit-career-mcp",
    "henukit-portal-gateway",
    "henukit-quizcraft",
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

test("release artifacts are blocked on the cumulative cross-product OAuth journey", () => {
  assert.equal(
    rootPackage.scripts["test:oauth-continuation"],
    "node --test scripts/tests/oauth-continuation-journey.test.mjs && go -C services/platform-core test ./internal/httpapi -run '^TestOAuthContinuationAuditUsesBoundedSchema$' -count=1 && go -C services/platform-core test ./tests -run '^TestPortalOAuthContinuationRestoresValidatedAuthorizeRequest$' -count=1 && pnpm --filter @henukit/portal run test:e2e:oauth-continuation && pnpm --filter @henukit/console run test:e2e:oauth-continuation",
  );
  assert.match(workflow, /oauth-continuation:\s+name: oauth-continuation/);
  assert.match(
    workflow,
    /build-image:[\s\S]*needs: \[validate-release-contract, oauth-continuation, release-image-matrix\]/,
  );
  assert.match(
    workflow,
    /package-runtime:[\s\S]*needs: \[validate-release-contract, oauth-continuation\]/,
  );
  assert.match(workflow, /oauth-continuation-release-gate\.sh run[\s\S]*--sha "\$GITHUB_SHA"/);
  assert.match(workflow, /actions\/upload-artifact@v4[\s\S]*henukit-oauth-continuation-gate-/);
  assert.match(workflow, /actions\/download-artifact@v4[\s\S]*henukit-oauth-continuation-gate-/);
  assert.match(workflow, /oauth-continuation-release-gate\.sh verify/);
  assert.match(
    workflow,
    /Build fixed-SHA image[\s\S]*git archive --format=tar "\$GITHUB_SHA"[\s\S]*cd "\$source_root"[\s\S]*docker build/,
  );
  assert.match(workflow, /scripts\/ops\/tests\/oauth-continuation-release-gate\.test\.mjs/);
  for (const builder of [localBuilder, quickBuilder]) {
    assert.match(builder, /"\$oauth_gate" run --sha "\$release_sha" --output "\$oauth_gate_receipt"/);
    assert.match(builder, /"\$oauth_gate" verify --sha "\$release_sha" --receipt "\$oauth_gate_receipt"/);
    assert.match(builder, /"\$runtime_packager"[\s\S]*--oauth-gate-receipt "\$oauth_gate_receipt"/);
    assert.ok(
      builder.indexOf('"$oauth_gate" run') < builder.indexOf("docker build"),
      "local builders must run the gate before the first image build",
    );
    assert.match(
      builder,
      /git -C "\$repo_root" archive --format=tar "\$release_sha" \| tar -xf - -C "\$source_root"/,
    );
    assert.ok(
      builder.indexOf('archive --format=tar "$release_sha"') <
        builder.indexOf("docker build"),
      "local builders must build images from an exact Git snapshot",
    );
  }
  assert.match(
    quickBuilder,
    /\[\[ -z "\$output_dir" \]\] && output_dir="\$repo_root\/artifacts\/henukit-release-quick"/,
  );
  assert.match(quickBuilder, /source_tree=.*rev-parse "\$\{release_sha\}\^\{tree\}"/);
  assert.equal(
    (quickBuilder.match(/assert_source_snapshot/g) ?? []).length,
    4,
    "the quick builder rechecks the exact source before and after construction",
  );
  assert.match(oauthGate, /pnpm -C "\$repo_root" run test:oauth-continuation/);
  assert.match(oauthGate, /release_sha=%s/);
  assert.match(oauthGate, /source_tree=%s/);
  assert.match(runtimePackager, /oauth-continuation-release-gate\.sh/);
  assert.match(runtimePackager, /git -C "\$repo_root" archive --format=tar "\$release_sha"/);
  assert.match(runtimePackager, /release-gates\/oauth-continuation\.env/);
});

test("CI runs the enabled QuizCraft V2 ranking behavior spec", () => {
  assert.match(workflow, /Verify enabled QuizCraft V2 stats and rankings/);
  assert.match(workflow, /pnpm --filter @henukit\/portal test:e2e:stats/);
});

test("release artifacts carry an exact-SHA Account mock-free boundary manifest", () => {
  assert.match(
    workflow,
    /scripts\/ops\/package-henukit-runtime\.sh[\s\S]*--sha "\$GITHUB_SHA"[\s\S]*--oauth-gate-receipt/,
  );
});

test("the production watcher extracts runtime artifacts as the root release owner", () => {
  assert.match(
    actionsWatcher,
    /tar --no-same-owner -xzf "\$runtime_archive" -C "\$release_incoming"/,
  );
});

test("release-contract CI exercises the production artifact deployment seam", () => {
  assert.match(
    workflow,
    /scripts\/ops\/tests\/deploy-henukit-artifact\.test\.mjs/,
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
  assert.doesNotMatch(prebuiltCompose, /STUDY_LEGACY_API_URL/);
  assert.doesNotMatch(prebuiltCompose, /STUDY_LEGACY_ADMIN_TOKEN/);
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
      "CAREER_DATABASE_URL",
      "CAREER_CLIENT_SECRET",
      "CAREER_SOURCE_ALLOWLIST",
      "CAREER_AI_BASE_URL",
      "CAREER_AI_API_KEY",
      "CAREER_AI_MODEL",
      "PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID",
      "PLATFORM_CORE_CAREER_DIGEST_KEY_ID",
      "PLATFORM_CORE_CAREER_DIGEST_SECRET",
      "FOOD_POST_CREATE_SECRET",
      "FOOD_POST_READ_SECRET",
      "FOOD_MCP_ACCESS_TOKEN",
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
      "PORTAL_VERSION",
      "PORTAL_DEPLOYED_AT",
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
      "QUIZCRAFT_SUMMARY_CLIENT_ID",
      "QUIZCRAFT_SUMMARY_KEY_ID",
      "QUIZCRAFT_V2_DATABASE_URL",
      "QUIZCRAFT_AUTH_HMAC_SECRET",
      "QUIZCRAFT_CUTOVER_EVIDENCE_SECRET",
      "QUIZCRAFT_WRITES_ENABLED",
      "QUIZCRAFT_PORTAL_COMMANDS_ENABLED",
      "QUIZCRAFT_PORTAL_COMMAND_CLIENT_ID",
      "QUIZCRAFT_PORTAL_COMMAND_CLIENT_SECRET",
      "QUIZCRAFT_PORTAL_COMMAND_KEY_ID",
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
            QUIZCRAFT_CORE_URL: "http://quizcraft:10089",
            CAREER_DATABASE_URL: "postgres://test",
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
    "henukit-portal-summary",
    "henukit-portal-api",
    "henukit-account-portfolio",
    "henukit-portal-gateway",
    "henukit-notice",
    "henukit-notice-worker",
    "henukit-food",
    "henukit-food-mcp",
    "henukit-career-opportunities",
    "henukit-career-mcp",
    "henukit-library",
    "henukit-quizcraft",
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
    config.services.quizcraft.environment.DATABASE_URL,
    "test-required-value",
    "the containerized Core must read the V2 database URL from QUIZCRAFT_V2_DATABASE_URL",
  );
  assert.equal(
    config.services.quizcraft.environment.QUIZCRAFT_HTTP_ADDR,
    ":10089",
    "the containerized Core must listen on the port the Gateway seam addresses",
  );
  assert.deepEqual(
    config.services.quizcraft.extra_hosts,
    ["host.docker.internal=host-gateway"],
    "the containerized Core needs the host-gateway mapping until quizcraft_v2 moves into the Docker postgres",
  );
  assert.equal(
    Object.values(config.services).some((service) => "build" in service),
    false,
  );
  assert.equal(
    Object.keys(config.services).some((service) => /^study/.test(service)),
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
    "http://quizcraft:10089",
    "the fixed-SHA Gateway catalog seam must call the containerized QuizCraft Core service, not legacy Portal API",
  );
  assert.equal(
    config.services["career-opportunities"].environment.CAREER_SOURCE_ALLOWLIST,
    "test-required-value",
    "production Career must require an explicit authorized source allowlist",
  );
  assert.equal(
    config.services["career-opportunities"].environment.PLATFORM_CORE_CAREER_DIGEST_URL,
    "http://platform-core:8081",
    "completed searches must enqueue their digest through the internal Platform Core service",
  );
  assert.equal(
    config.services["career-opportunities"].environment.PLATFORM_CORE_CAREER_DIGEST_SECRET,
    config.services["platform-core"].environment.PLATFORM_CORE_CAREER_DIGEST_SECRET,
    "Career and Platform Core must receive the same dedicated digest credential",
  );
  assert.equal(
    config.services["career-opportunities"].environment.CAREER_REQUIRE_AI,
    "1",
    "production Career must refuse startup without a real extraction LLM",
  );
  for (const key of ["CAREER_AI_BASE_URL", "CAREER_AI_API_KEY", "CAREER_AI_MODEL"]) {
    assert.equal(
      config.services["career-opportunities"].environment[key],
      "test-required-value",
      `${key} must be injected into the production Career container`,
    );
  }
  assert.deepEqual(
    config.services["portal-gateway"].extra_hosts,
    ["host.docker.internal=host-gateway"],
    "the containerized Gateway keeps the host-gateway mapping for legacy env values that still use host.docker.internal:10089",
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
    "http://notice:8094/api/v1/console-summary",
    "Notice summary must use the private owner endpoint",
  );
  assert.equal(config.services["console-gateway"].environment.PORTAL_SUMMARY_URL, "http://portal-summary:8083/api/v1/console-summary");
  assert.equal(config.services["console-gateway"].environment.LIBRARY_SUMMARY_URL, "http://library:8095/api/v1/console-summary");
  assert.equal(config.services["console-gateway"].environment.FOOD_SUMMARY_URL, "http://food:8096/api/v1/console-summary");
  assert.equal(config.services["console-gateway"].environment.PLATFORM_SUMMARY_URL, "");
  assert.equal(config.services["console-gateway"].environment.QUIZCRAFT_SUMMARY_URL, "");
  const withoutSummaryEndpoints = renderRuntimeConfig({
    PORTAL_SUMMARY_URL: "",
    NOTICE_SUMMARY_URL: "",
    LIBRARY_SUMMARY_URL: "",
    FOOD_SUMMARY_URL: "",
  });
  for (const name of [
    "PORTAL_SUMMARY_URL",
    "NOTICE_SUMMARY_URL",
    "LIBRARY_SUMMARY_URL",
    "FOOD_SUMMARY_URL",
  ]) {
    assert.equal(
      withoutSummaryEndpoints.services["console-gateway"].environment[name],
      "",
      `an explicit empty ${name} must disable that summary owner`,
    );
  }
  assert.deepEqual(
    config.services["portal-summary"].healthcheck.test,
    ["CMD", "wget", "--quiet", "--spider", "http://127.0.0.1:8083/readyz"],
  );
  assert.equal(
    config.services["console-gateway"].depends_on["portal-summary"].condition,
    "service_healthy",
  );
  assert.doesNotMatch(
    releaseImageMatrix().include.find(({ name }) => name === "console").build_args,
    /VITE_QUIZCRAFT_WORKSHOP_URL/,
    "production Console must not retain a retired workshop build argument",
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
  assert.match(
    workflow,
    /scripts\/ops\/package-henukit-runtime\.sh[\s\S]*--sha "\$GITHUB_SHA"[\s\S]*--oauth-gate-receipt/,
  );
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
    /cp "\$source_root"\/services\/platform-core\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry the registration migration",
  );
  assert.match(
    runtimePackager,
    /cp "\$source_root"\/services\/account-portfolio\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Account Portfolio recovery migrations",
  );
  assert.match(
    runtimePackager,
    /cp "\$source_root"\/services\/notice\/db\/migrations\/\*\.up\.sql/,
    "the fixed-SHA runtime must carry Notice recovery migrations",
  );
  assert.match(
    runtimePackager,
    /cp "\$source_root"\/services\/food\/db\/migrations\/\*\.up\.sql/,
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
