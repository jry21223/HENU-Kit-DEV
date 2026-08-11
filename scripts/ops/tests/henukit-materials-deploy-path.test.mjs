import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const read = (path) => readFileSync(join(repoRoot, path), "utf8");

test("installer preserves literal environment values while reconciling existing files", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-installer-"));
  try {
    const installer = read("services/deploy-webhook/deploy/install.sh");
    const functionStart = installer.indexOf("set_env_value() {");
    const functionEnd = installer.indexOf("\n\ncopy_env_value_if_present()", functionStart);
    assert.notEqual(functionStart, -1);
    assert.notEqual(functionEnd, -1);

    const envFile = join(root, "materials-runner.env");
    const expected = "postgres://user:p&ss|word\\with=equals@db.invalid/study?sslmode=require";
    writeFileSync(
      envFile,
      "# keep this comment\nHENUKIT_MATERIALS_DATABASE_URL=old\nUNCHANGED=value\n",
    );
    const result = spawnSync(
      "bash",
      [
        "-c",
        `${installer.slice(functionStart, functionEnd)}\nset_env_value \"$1\" HENUKIT_MATERIALS_DATABASE_URL \"$2\"`,
        "installer-env-test",
        envFile,
        expected,
      ],
      { encoding: "utf8" },
    );

    assert.equal(result.status, 0, result.stderr);
    assert.equal(
      readFileSync(envFile, "utf8"),
      `# keep this comment\nHENUKIT_MATERIALS_DATABASE_URL=${expected}\nUNCHANGED=value\n`,
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials receiver has one latest-only queue and one fixed privileged command", () => {
  const env = read("services/deploy-webhook/deploy/materials.env.example");
  const runnerEnv = read("services/deploy-webhook/deploy/materials-runner.env.example");

  assert.match(env, /^HENUKIT_WEBHOOK_LISTEN_ADDR=127\.0\.0\.1:10088$/m);
  assert.match(env, /^HENUKIT_WEBHOOK_PATH=\/webhooks\/materials$/m);
  assert.match(env, /^HENUKIT_WEBHOOK_REPOSITORY=jry21223\/HENU-Final-Review$/m);
  assert.match(env, /^HENUKIT_WEBHOOK_BRANCH=main$/m);
  assert.match(env, /^HENUKIT_WEBHOOK_MAX_QUEUE=1$/m);
  assert.match(env, /^HENUKIT_WEBHOOK_QUEUE_MODE=latest$/m);
  assert.doesNotMatch(env, /HENUKIT_MATERIALS_DATABASE_URL/);
  assert.doesNotMatch(env, /HENUKIT_DEPLOY_COMMAND/);
  assert.doesNotMatch(runnerEnv, /HENUKIT_WEBHOOK_/);
  assert.doesNotMatch(runnerEnv, /HENUKIT_DEPLOY_COMMAND/);
  assert.match(runnerEnv, /HENUKIT_MATERIALS_DATABASE_URL/);
});

test("manual sync has one fixed root-helper entry and still consumes the root-only runner policy", () => {
  const rootHelper = read("services/deploy-webhook/deploy/henukit-materials-root");
  const runbook = read("docs/operations/henukit-materials-sync.md");

  assert.match(rootHelper, /materials-runner\.env/);
  assert.match(rootHelper, /\$# == 1[^\n]*--manual|--manual[^\n]*\$# == 1/);
  assert.match(rootHelper, /manual_mode[\s\S]*SUDO_USER[\s\S]*henukit-deploy/);
  assert.match(
    runbook,
    /sudo \/usr\/local\/libexec\/henukit\/henukit-materials-root --manual/,
  );
  assert.doesNotMatch(
    runbook,
    /sudo \/usr\/local\/libexec\/henukit\/henukit-materials-sync(?:\.sh)? --manual/,
  );
});

test("the production database env default has only root-controlled ancestors", () => {
  const installer = read("services/deploy-webhook/deploy/install.sh");
  const runnerEnv = read("services/deploy-webhook/deploy/materials-runner.env.example");
  const driver = read("scripts/ops/henukit-materials-sync.sh");

  assert.match(
    runnerEnv,
    /^HENUKIT_MATERIALS_ENV_FILE=\/etc\/henukit-deploy\/materials-production\.env$/m,
  );
  assert.match(
    driver,
    /HENUKIT_MATERIALS_ENV_FILE:-\/etc\/henukit-deploy\/materials-production\.env/,
  );
  assert.match(driver, /validate_root_controlled_path "\$env_file"/);
  assert.match(installer, /materials-production\.env/);
  assert.match(installer, /\/opt\/henukit\/\.env\.henukit/);
});

test("materials installer quiesce ignores only absent units and confirms inactive state", () => {
  const installer = read("services/deploy-webhook/deploy/install.sh");
  const functionStart = installer.indexOf("quiesce_materials_unit() {");
  const functionEnd = installer.indexOf("\n}\n", functionStart);
  assert.notEqual(functionStart, -1, "quiesce_materials_unit must exist");
  assert.notEqual(functionEnd, -1, "quiesce_materials_unit must have a complete body");
  const functionSource = installer.slice(functionStart, functionEnd + 3);

  const run = (scenario, action = "stop") =>
    spawnSync(
      "bash",
      [
        "-c",
        `${functionSource}
systemctl() {
  case "$1" in
    show)
      case "$HENUKIT_TEST_SCENARIO" in
        missing) printf 'not-found\\n'; return 0 ;;
        show-error) return 9 ;;
        *) printf 'loaded\\n'; return 0 ;;
      esac
      ;;
    stop) [[ "$HENUKIT_TEST_SCENARIO" != stop-fail ]] ;;
    disable) [[ "$HENUKIT_TEST_SCENARIO" != disable-fail ]] ;;
    is-active)
      if [[ "$HENUKIT_TEST_SCENARIO" == active ]]; then
        printf 'active\\n'
        return 0
      fi
      printf 'inactive\\n'
      return 3
      ;;
    *) return 10 ;;
  esac
}
quiesce_materials_unit fixture.service "$1"`,
        "installer-quiesce-test",
        action,
      ],
      {
        encoding: "utf8",
        env: { ...process.env, HENUKIT_TEST_SCENARIO: scenario },
      },
    );

  assert.equal(run("missing").status, 0);
  assert.notEqual(run("show-error").status, 0);
  assert.notEqual(run("stop-fail").status, 0);
  assert.notEqual(run("disable-fail", "disable").status, 0);
  assert.notEqual(run("active").status, 0);
  assert.equal(run("inactive").status, 0);
  assert.doesNotMatch(
    installer,
    /systemctl (?:disable --now|stop) henukit-materials-[^\n]*\|\| true/,
  );
});

test("installer and systemd units converge on the same receiver and runner", () => {
  const installer = read("services/deploy-webhook/deploy/install.sh");
  const receiver = read(
    "services/deploy-webhook/deploy/systemd/henukit-materials-webhook.service",
  );
  const watcher = read(
    "services/deploy-webhook/deploy/systemd/henukit-materials-webhook.path",
  );
  const runner = read(
    "services/deploy-webhook/deploy/systemd/henukit-materials-runner.service",
  );

  assert.match(
    installer,
    /scripts\/ops\/henukit-materials-sync\.sh" \/usr\/local\/libexec\/henukit\/henukit-materials-sync\n/,
  );
  assert.doesNotMatch(installer, /henukit-materials-webhook\.mjs/);
  assert.match(receiver, /^User=henukit-deploy$/m);
  assert.match(receiver, /^EnvironmentFile=\/etc\/henukit-deploy\/materials-webhook\.env$/m);
  assert.match(receiver, /^LoadCredential=webhook_secret:/m);
  assert.match(receiver, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook serve$/m);
  assert.match(watcher, /^Unit=henukit-materials-runner\.service$/m);
  assert.match(runner, /^User=henukit-deploy$/m);
  assert.match(runner, /^Group=henukit-deploy$/m);
  assert.match(runner, /^EnvironmentFile=\/etc\/henukit-deploy\/materials-webhook\.env$/m);
  assert.match(
    runner,
    /^Environment=HENUKIT_DEPLOY_COMMAND=\/usr\/local\/libexec\/henukit\/henukit-materials-sudo$/m,
  );
  assert.match(runner, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook run$/m);
  assert.match(
    runner,
    /^ReadWritePaths=\/var\/lib\/henukit-materials-webhook \/opt\/henukit-materials$/m,
  );

  assert.match(
    installer,
    /henukit-materials-root[^\n]*\/usr\/local\/libexec\/henukit\/henukit-materials-root/,
  );
  assert.match(
    installer,
    /henukit-materials-sudo[^\n]*\/usr\/local\/libexec\/henukit\/henukit-materials-sudo/,
  );
  assert.match(installer, /visudo -cf/);
  assert.match(
    installer,
    /Defaults!\/usr\/local\/libexec\/henukit\/henukit-materials-root secure_path=/,
  );
  assert.match(
    installer,
    /quiesce_materials_unit henukit-materials-runner\.service stop[\s\S]*install[^\n]*henukit-materials-sync/,
  );
  assert.match(installer, /systemctl restart henukit-materials-webhook\.service/);
  assert.match(
    installer,
    /ln -sfn henukit-materials-sync \/usr\/local\/libexec\/henukit\/henukit-materials-sync\.sh/,
  );
  assert.ok(
    installer.indexOf('mv -f -- "$materials_runner_stage" "$runner_env"') <
      installer.indexOf('mv -f -- "$materials_receiver_stage" "$receiver_env"'),
    "complete root config must be installed before legacy secrets leave the receiver config",
  );
});

test(
  "the privileged helpers use absolute Bash and never resolve it from the caller PATH",
  { skip: process.platform !== "linux" },
  () => {
    const root = mkdtempSync(join(tmpdir(), "henukit-materials-path-"));
    try {
      const fakeBin = join(root, "bin");
      const sentinel = join(root, "caller-bash-ran");
      mkdirSync(fakeBin);
      const fakeBash = join(fakeBin, "bash");
      writeFileSync(
        fakeBash,
        '#!/bin/sh\n: > "$HENUKIT_MALICIOUS_BASH_SENTINEL"\nexit 97\n',
      );
      chmodSync(fakeBash, 0o755);

      const helper = join(
        repoRoot,
        "services/deploy-webhook/deploy/henukit-materials-root",
      );
      const sudoHelper = read(
        "services/deploy-webhook/deploy/henukit-materials-sudo",
      );
      assert.match(readFileSync(helper, "utf8"), /^#!\/bin\/bash -p\n/);
      assert.match(sudoHelper, /^#!\/bin\/bash\n/);
      const result = spawnSync(
        helper,
        [
          "--sha",
          "a".repeat(40),
          "--delivery",
          "delivery-path-test",
          "--repository",
          "jry21223/HENU-Final-Review",
          "--ref",
          "refs/heads/main",
        ],
        {
          encoding: "utf8",
          env: {
            ...process.env,
            PATH: `${fakeBin}:/usr/bin:/bin`,
            HENUKIT_MALICIOUS_BASH_SENTINEL: sentinel,
          },
        },
      );

      assert.notEqual(result.status, 97, result.stderr);
      assert.equal(existsSync(sentinel), false);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  },
);

test("the versioned Study migration owns DDL and the runtime sync path is DML-only", () => {
  const installer = read("services/deploy-webhook/deploy/install.sh");
  const migration = read(
    "services/api/migrations/0002_henukit_materials_sync_expand.up.sql",
  );
  const rollback = read(
    "services/api/migrations/0002_henukit_materials_sync_expand.down.sql",
  );
  const driver = read("scripts/ops/henukit-materials-sync.sh");

  assert.match(installer, /0002_henukit_materials_sync_expand\.up\.sql/);
  assert.match(migration, /CREATE TABLE IF NOT EXISTS public\.henukit_materials_sync_state/);
  assert.match(migration, /ALTER TABLE public\.materials ADD COLUMN IF NOT EXISTS sha256/);
  assert.match(rollback, /irreversible/i);
  assert.match(driver, /Study materials expand migration 0002 is required/);
  assert.doesNotMatch(
    driver,
    /\b(?:CREATE(?:\s+UNIQUE)?|ALTER|DROP)\s+(?:DATABASE|SCHEMA|TABLE|INDEX)\b/i,
  );
});

test("database access is pinned to the watcher active release and never falls back to development credentials", () => {
  const driver = read("scripts/ops/henukit-materials-sync.sh");

  assert.match(driver, /\/var\/lib\/henukit-actions-watch\/last-activated-sha/);
  assert.match(driver, /\/opt\/henukit-releases\/\$database_release_sha/);
  assert.match(
    driver,
    /HENUKIT_MATERIALS_ENV_FILE:-\/etc\/henukit-deploy\/materials-production\.env/,
  );
  assert.match(driver, /release directory does not match the watcher active SHA/);
  assert.match(driver, /validate_root_policy_file "\$active_sha_file" 0/);
  assert.match(driver, /validate_root_policy_file "\$compose_file" 0/);
  assert.match(driver, /validate_root_policy_file "\$release_marker" 0/);
  assert.match(driver, /validate_root_policy_file "\$env_file" 1/);
  assert.match(driver, /validate_root_controlled_path "\$env_file"/);
  assert.match(driver, /RELEASE_SHA="\$database_release_sha"/);
  assert.doesNotMatch(driver, /for candidate in \/opt\/henukit-releases/);
  assert.doesNotMatch(driver, /henukit_dev_change_me/);
  assert.match(driver, /STUDY_DATABASE_URL not found/);
  assert.match(driver, /POSTGRES_USER is missing or invalid/);
});

test("materials serving keeps every documented Nginx hardening control", () => {
  const compose = read("docker-compose.henukit.yml");
  const nginx = read("infra/nginx/henukit.conf.example");

  assert.match(
    compose,
    /source: \$\{HENUKIT_MATERIALS_ROOT:-\/opt\/henukit-materials\/public\}\s+target: \/srv\/materials\s+read_only: true/,
  );
  const dotfileLocation = nginx.match(/location ~ (\^\/materials\/[^ ]+) \{/);
  assert.ok(dotfileLocation, "materials dotfile location must exist");
  const dotfilePattern = new RegExp(dotfileLocation[1]);
  assert.equal(dotfilePattern.test("/materials/.git/config"), true);
  assert.equal(dotfilePattern.test("/materials/course/.hidden.pdf"), true);
  assert.equal(dotfilePattern.test("/materials/course/week-1.pdf"), false);
  assert.match(nginx, /location \/materials\/ \{/);
  assert.match(nginx, /alias \/srv\/materials\/current\/files\//);
  assert.match(nginx, /autoindex off;/);
  assert.match(nginx, /Cache-Control "public, max-age=86400" always;/);
  assert.match(nginx, /X-Content-Type-Options "nosniff" always;/);
  assert.match(nginx, /Content-Disposition "attachment" always;/);
  assert.match(nginx, /Content-Security-Policy "default-src 'none'; sandbox" always;/);
});

test(
  "the production Nginx regex rejects root and nested dotfiles at request time",
  { skip: spawnSync("docker", ["info"], { stdio: "ignore" }).status !== 0 },
  () => {
    const root = mkdtempSync(join(tmpdir(), "henukit-materials-nginx-"));
    let container = "";
    try {
      const nginx = read("infra/nginx/henukit.conf.example");
      const dotfileLocation = nginx.match(/location ~ (\^\/materials\/[^ ]+) \{/);
      assert.ok(dotfileLocation);
      const materials = join(root, "materials", "current", "files", "course");
      mkdirSync(materials, { recursive: true });
      writeFileSync(join(root, "materials", "current", "files", ".hidden.pdf"), "must not be served\n");
      writeFileSync(join(materials, ".hidden.pdf"), "must not be served\n");
      writeFileSync(join(materials, "week-1.pdf"), "public material\n");
      const config = join(root, "nginx.conf");
      writeFileSync(
        config,
        `events {}
http {
  server {
    listen 8080;
    location ~ ${dotfileLocation[1]} { return 404; }
    location /materials/ { alias /srv/materials/current/files/; }
  }
}
`,
      );
      container = execFileSync(
        "docker",
        [
          "run",
          "--rm",
          "-d",
          "-p",
          "127.0.0.1::8080",
          "-v",
          `${config}:/etc/nginx/nginx.conf:ro`,
          "-v",
          `${join(root, "materials")}:/srv/materials:ro`,
          "nginx:1.27-alpine",
        ],
        { encoding: "utf8" },
      ).trim();
      const address = execFileSync("docker", ["port", container, "8080/tcp"], {
        encoding: "utf8",
      }).trim();
      const status = (path) =>
        execFileSync(
          "curl",
          ["--retry", "10", "--retry-connrefused", "--retry-delay", "0", "-sS", "-o", "/dev/null", "-w", "%{http_code}", `http://${address}${path}`],
          { encoding: "utf8" },
        ).trim();

      assert.equal(status("/materials/.hidden.pdf"), "404");
      assert.equal(status("/materials/course/.hidden.pdf"), "404");
      assert.equal(status("/materials/course/week-1.pdf"), "200");
    } finally {
      if (container) {
        spawnSync("docker", ["rm", "-f", container], { stdio: "ignore" });
      }
      rmSync(root, { recursive: true, force: true });
    }
  },
);

test("deploy-webhook CI owns every canonical materials path and its integration tests", () => {
  const workflow = read(".github/workflows/deploy-webhook.yml");
  for (const path of [
    "scripts/ops/sync-henukit-materials.sh",
    "scripts/ops/henukit-materials-sync.sh",
    "scripts/ops/convert-henukit-slides.py",
    "scripts/ops/import-henukit-materials.mjs",
    "scripts/ops/tests/henukit-materials-sync.test.mjs",
    "scripts/ops/tests/henukit-materials-deploy-path.test.mjs",
    "services/portal-api/internal/library/db.go",
    "services/portal-api/internal/library/db_test.go",
    "docs/operations/henukit-materials-sync.md",
    "infra/nginx/henukit.conf.example",
    "docker-compose.henukit.yml",
  ]) {
    assert.match(workflow, new RegExp(path.replaceAll(".", "\\.").replaceAll("/", "\\/")));
  }
  assert.match(
    workflow,
    /node --test[\s\\\n]+scripts\/ops\/tests\/henukit-materials-sync\.test\.mjs/,
  );
  assert.match(workflow, /python3 -m py_compile scripts\/ops\/convert-henukit-slides\.py/);
  assert.match(workflow, /bash -n[\s\\\n]+scripts\/ops\/sync-henukit-materials\.sh/);
  assert.match(workflow, /staticcheck@v0\.7\.0/);
  assert.match(workflow, /HENUKIT_TEST_POSTGRES_URL:/);
  assert.match(
    workflow,
    /working-directory: services\/portal-api[\s\S]*go test -count=1 \.\/internal\/library/,
  );
});

test("runbook names the only webhook, queue, privileged runner, and rollback path", () => {
  const runbook = read("docs/operations/henukit-materials-sync.md");

  assert.match(runbook, /https:\/\/henukit\.cn\/webhooks\/materials/);
  assert.match(runbook, /127\.0\.0\.1:10088/);
  assert.match(runbook, /\/etc\/henukit-deploy\/materials-webhook-secret/);
  assert.match(runbook, /HENUKIT_WEBHOOK_QUEUE_MODE=latest/);
  assert.match(runbook, /henukit-materials-runner\.service/);
  assert.match(runbook, /^## 回滚$/m);
  assert.match(runbook, /systemctl disable --now henukit-materials-webhook\.path/);
});

test("runbook lands and probes the materials Nginx location before enabling the watcher", () => {
  const runbook = read("docs/operations/henukit-materials-sync.md");
  const fragment = runbook.indexOf(
    "services/deploy-webhook/deploy/nginx-materials.conf.example",
  );
  const liveConfig = runbook.indexOf("sudoedit /etc/nginx/sites-enabled/henukit.cn", fragment);
  const configTest = runbook.indexOf("sudo nginx -t", liveConfig);
  const reload = runbook.indexOf("sudo systemctl reload nginx", configTest);
  const publicProbe = runbook.indexOf(
    "curl --fail --silent --show-error --head",
    reload,
  );
  const enableWatcher = runbook.indexOf(
    "sudo systemctl enable --now henukit-materials-webhook.path",
    publicProbe,
  );

  assert.notEqual(fragment, -1, "runbook must name the materials webhook fragment");
  assert.ok(liveConfig > fragment, "runbook must name the live Nginx vhost");
  assert.ok(configTest > liveConfig, "nginx -t must follow config installation");
  assert.ok(reload > configTest, "reload must follow nginx -t");
  assert.ok(publicProbe > reload, "the public materials probe must follow reload");
  assert.ok(enableWatcher > publicProbe, "watcher enablement must follow the public probe");
});

test("runbook activates and verifies the current-files edge alias before the first manual sync", () => {
  const runbook = read("docs/operations/henukit-materials-sync.md");
  const activation = runbook.indexOf(
    "/usr/local/sbin/activate-henukit-release <full-main-sha> --execute",
  );
  const activeSHA = runbook.indexOf(
    "/var/lib/henukit-actions-watch/last-activated-sha",
    activation,
  );
  const edgeConfig = runbook.indexOf("exec -T nginx nginx -T", activeSHA);
  const aliasProof = runbook.indexOf("alias /srv/materials/current/files/;", edgeConfig);
  const manual = runbook.indexOf(
    "sudo /usr/local/libexec/henukit/henukit-materials-root --manual",
  );

  assert.notEqual(activation, -1, "runbook must activate the fixed-SHA edge release");
  assert.ok(activeSHA > activation, "active SHA verification must follow release activation");
  assert.ok(edgeConfig > activeSHA, "live edge config inspection must follow active SHA verification");
  assert.ok(aliasProof > edgeConfig, "the live edge must prove its current/files alias");
  assert.ok(manual > aliasProof, "manual sync must not migrate the layout before the new alias is live");
});
