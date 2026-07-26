import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-henukit.yml", import.meta.url),
  "utf8",
);

test("CI builds every HENU production image and no Study image", () => {
  const expectedImages = [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-api",
    "henukit-portal-gateway",
    "henukit-quizcraft-api",
    "henukit-quizcraft-web",
  ];

  for (const image of expectedImages) {
    assert.match(workflow, new RegExp(`image: ${image.replaceAll("-", "\\-")}`));
  }
  assert.doesNotMatch(workflow, /image: henukit-study/);
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
      "FOOD_CLIENT_SECRET",
      "FOOD_SUMMARY_CLIENT_SECRET",
      "LIBRARY_API_URL",
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
      "QUIZCRAFT_ADMIN_TOKEN",
      "QUIZCRAFT_SUMMARY_CLIENT_SECRET",
    ].map((name) => [name, "test-required-value"]),
  );
  const config = JSON.parse(
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
          QUIZCRAFT_DATABASE_URL: "postgres://test",
          RELEASE_SHA: releaseSha,
          STUDY_DATABASE_URL: "postgres://test",
        },
      },
    ),
  );
  const expectedImages = [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-api",
    "henukit-portal-gateway",
    "henukit-quizcraft-api",
    "henukit-quizcraft-web",
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
    Object.keys(config.services).some((service) => service.startsWith("study")),
    false,
  );
  assert.equal(
    config.services.nginx.volumes[0].source,
    "./infra/nginx/henukit.conf.example",
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
  assert.doesNotMatch(
    workflow,
    /cp docker-compose\.henukit\.yml|cp docker-compose\.henukit\.prebuilt\.yml|init-henukit-dbs\.sh/,
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
