import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const templatePath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-activate");

function shellLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function executable(path, body) {
  writeFileSync(path, body, { mode: 0o700 });
  chmodSync(path, 0o700);
}

function createFixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-activate-wrapper-"));
  const sealedRoot = join(root, "sealed");
  const publicRoot = join(root, "public");
  const ossAuditRoot = join(root, "oss-audit");
  const activationStagingRoot = join(root, "activation-staging");
  mkdirSync(sealedRoot, { mode: 0o700 });
  mkdirSync(publicRoot, { mode: 0o700 });
  mkdirSync(ossAuditRoot, { mode: 0o700 });
  mkdirSync(activationStagingRoot, { mode: 0o700 });
  const importer = join(root, "importer");
  const psql = join(root, "psql");
  for (const tool of [importer, psql]) executable(tool, "#!/bin/sh\nexit 0\n");
  const output = join(root, "invocation.json");
  const activation = join(root, "activate-henukit-materials.mjs");
  writeFileSync(
    activation,
    `import { writeFileSync } from "node:fs";\nwriteFileSync(${JSON.stringify(output)}, JSON.stringify({ argv: process.argv.slice(2), env: process.env }));\n`,
    { mode: 0o600 },
  );
  chmodSync(activation, 0o600);
  const bundleBuilder = join(root, "build-henukit-library-activation-bundle.mjs");
  writeFileSync(bundleBuilder, "// fixed bundle builder\n", { mode: 0o600 });
  chmodSync(bundleBuilder, 0o600);
  const libraryActivator = join(root, "library-activate-public-release");
  executable(libraryActivator, "#!/bin/sh\nexit 0\n");
  const flock = join(root, "flock");
  executable(flock, "#!/bin/sh\nshift 2\nexec \"$@\"\n");
  const config = join(root, "materials-activate.env");
  const pgServiceFile = join(root, "pg_service.conf");
  writeFileSync(pgServiceFile, "[materials]\nhost=trusted.example\ndbname=study\n", { mode: 0o600 });
  chmodSync(pgServiceFile, 0o600);
  const legacyInventory = join(root, "legacy-inventory.json");
  writeFileSync(legacyInventory, JSON.stringify({ version: 1, storage_keys: [] }), { mode: 0o600 });
  chmodSync(legacyInventory, 0o600);
  writeFileSync(
    config,
    [
      `HENUKIT_MATERIALS_SEALED_ROOT=${sealedRoot}`,
      `HENUKIT_MATERIALS_PUBLIC_ROOT=${publicRoot}`,
      `HENUKIT_MATERIALS_IMPORTER=${importer}`,
      `HENUKIT_MATERIALS_PSQL=${psql}`,
      `HENUKIT_MATERIALS_PG_SERVICE_FILE=${pgServiceFile}`,
      "HENUKIT_MATERIALS_PG_SERVICE=materials",
      `HENUKIT_MATERIALS_LEGACY_INVENTORY=${legacyInventory}`,
      `HENUKIT_MATERIALS_OSS_AUDIT_ROOT=${ossAuditRoot}`,
      `HENUKIT_MATERIALS_ACTIVATION_STAGING_ROOT=${activationStagingRoot}`,
      "LIBRARY_DATABASE_URL=postgres://fixed.example/library",
      "LIBRARY_OSS_ECS_RAM_ROLE=henukit-library-activation",
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
  chmodSync(config, 0o600);

  let template = readFileSync(templatePath, "utf8");
  template = template.replace('readonly config_path="/etc/henukit-deploy/materials-activate.env"', `readonly config_path=${shellLiteral(config)}`);
  template = template.replace('readonly runtime_owner="0"', `readonly runtime_owner="${process.getuid()}"`);
  template = template.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  template = template.replace('readonly lock_path="/run/henukit-materials-activate.lock"', `readonly lock_path=${shellLiteral(join(root, "activation.lock"))}`);
  template = template.replace('readonly flock_bin="/usr/bin/flock"', `readonly flock_bin=${shellLiteral(flock)}`);
  template = template.replace('readonly activate_script="$runtime_dir/activate-henukit-materials.mjs"', `readonly activate_script=${shellLiteral(activation)}`);
  const wrapper = join(root, "henukit-materials-activate");
  writeFileSync(wrapper, template, { mode: 0o700 });
  chmodSync(wrapper, 0o700);
  return { root, wrapper, output, config, activation, sealedRoot, publicRoot, importer, psql, pgServiceFile, legacyInventory, ossAuditRoot, activationStagingRoot, bundleBuilder, libraryActivator };
}

test("root activation wrapper accepts only one approved release identity and fixes every authority-bearing input", () => {
  const fixture = createFixture();
  try {
    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
    const receipt = "c".repeat(64);
    const result = spawnSync(fixture.wrapper, ["--release-id", releaseID, "--receipt-sha256", receipt], {
      encoding: "utf8",
      env: {
        ...process.env,
        HENUKIT_MATERIALS_PUBLIC_ROOT: "/tmp/attacker-public",
        HENUKIT_MATERIALS_DATABASE_URL: "postgres://attacker.example/study",
        NODE_OPTIONS: "--trace-warnings",
      },
    });

    assert.equal(result.status, 0, result.stderr);
    const invocation = JSON.parse(readFileSync(fixture.output, "utf8"));
    assert.deepEqual(invocation.argv, [
      "--release-id", releaseID,
      "--receipt-sha256", receipt,
      "--sealed-root", fixture.sealedRoot,
      "--public-root", fixture.publicRoot,
      "--importer", fixture.importer,
      "--psql", fixture.psql,
      "--legacy-inventory", fixture.legacyInventory,
      "--oss-audit-root", fixture.ossAuditRoot,
      "--activation-staging-root", fixture.activationStagingRoot,
      "--bundle-builder", fixture.bundleBuilder,
      "--library-activator", fixture.libraryActivator,
      "--activation-owner", String(process.getuid()),
    ]);
    assert.equal(invocation.env.PATH, "/usr/bin:/bin");
    assert.equal(invocation.env.HENUKIT_MATERIALS_PUBLIC_ROOT, undefined);
    assert.equal(invocation.env.HENUKIT_MATERIALS_DATABASE_URL, undefined);
    assert.equal(invocation.env.PGSERVICEFILE, fixture.pgServiceFile);
    assert.equal(invocation.env.PGSERVICE, "materials");
    assert.equal(invocation.env.LIBRARY_DATABASE_URL, "postgres://fixed.example/library");
    assert.equal(invocation.env.LIBRARY_OSS_ECS_RAM_ROLE, "henukit-library-activation");
    assert.doesNotMatch(invocation.argv.join("\n"), /postgres:\/\//);
    assert.equal(invocation.env.NODE_OPTIONS, undefined);
  } finally {
    rmSync(fixture.root, { recursive: true, force: true });
  }
});

test("root activation wrapper rejects caller authority injection and unsafe configuration metadata", () => {
  const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
  const receipt = "c".repeat(64);
  for (const args of [
    ["--release-id", releaseID, "--receipt-sha256", receipt, "--public-root", "/tmp/attacker"],
    ["--release-id", `${releaseID};touch /tmp/attacker`, "--receipt-sha256", receipt],
    ["--release-id", releaseID, "--receipt-sha256", "C".repeat(64)],
  ]) {
    const fixture = createFixture();
    try {
      const result = spawnSync(fixture.wrapper, args, { encoding: "utf8" });
      assert.notEqual(result.status, 0, args.join(" "));
      assert.equal(existsSync(fixture.output), false);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  }

  const writable = createFixture();
  try {
    chmodSync(writable.config, 0o640);
    const result = spawnSync(writable.wrapper, ["--release-id", releaseID, "--receipt-sha256", receipt], { encoding: "utf8" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /configuration must have mode 600/);
    assert.equal(existsSync(writable.output), false);
  } finally {
    rmSync(writable.root, { recursive: true, force: true });
  }

  const linked = createFixture();
  try {
    const linkedConfig = join(linked.root, "linked-activate.env");
    symlinkSync(linked.config, linkedConfig);
    const source = readFileSync(linked.wrapper, "utf8").replace(shellLiteral(linked.config), shellLiteral(linkedConfig));
    writeFileSync(linked.wrapper, source, { mode: 0o700 });
    chmodSync(linked.wrapper, 0o700);
    const result = spawnSync(linked.wrapper, ["--release-id", releaseID, "--receipt-sha256", receipt], { encoding: "utf8" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /configuration must be a regular file/);
    assert.equal(existsSync(linked.output), false);
  } finally {
    rmSync(linked.root, { recursive: true, force: true });
  }

  const wrongOwner = createFixture();
  try {
    const source = readFileSync(wrongOwner.wrapper, "utf8").replace(
      `readonly config_owner="${process.getuid()}"`,
      `readonly config_owner="${process.getuid() + 1}"`,
    );
    writeFileSync(wrongOwner.wrapper, source, { mode: 0o700 });
    chmodSync(wrongOwner.wrapper, 0o700);
    const result = spawnSync(wrongOwner.wrapper, ["--release-id", releaseID, "--receipt-sha256", receipt], { encoding: "utf8" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /configuration must be owned by root/);
    assert.equal(existsSync(wrongOwner.output), false);
  } finally {
    rmSync(wrongOwner.root, { recursive: true, force: true });
  }
});

test("root activation wrapper holds one kernel lock across the activation process", async (t) => {
  const docker = spawnSync("docker", ["info"], { encoding: "utf8" });
  if (docker.status !== 0) {
    t.skip("Docker is required to verify Linux flock behavior");
    return;
  }
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-activate-lock-"));
  const container = `henukit-materials-activate-lock-${process.pid}-${Date.now()}`;
  try {
    for (const directory of ["sealed", "public", "oss-audit", "activation-staging"]) mkdirSync(join(root, directory), { mode: 0o700 });
    for (const tool of ["importer", "psql"]) executable(join(root, tool), "#!/bin/sh\nexit 0\n");
    writeFileSync(join(root, "pg_service.conf"), "[materials]\nhost=trusted.example\ndbname=study\n", { mode: 0o600 });
    chmodSync(join(root, "pg_service.conf"), 0o600);
    writeFileSync(join(root, "legacy-inventory.json"), JSON.stringify({ version: 1, storage_keys: [] }), { mode: 0o600 });
    chmodSync(join(root, "legacy-inventory.json"), 0o600);
    writeFileSync(
      join(root, "materials-activate.env"),
      [
        "HENUKIT_MATERIALS_SEALED_ROOT=/fixture/sealed",
        "HENUKIT_MATERIALS_PUBLIC_ROOT=/fixture/public",
        "HENUKIT_MATERIALS_IMPORTER=/fixture/importer",
        "HENUKIT_MATERIALS_PSQL=/fixture/psql",
        "HENUKIT_MATERIALS_PG_SERVICE_FILE=/fixture/pg_service.conf",
        "HENUKIT_MATERIALS_PG_SERVICE=materials",
        "HENUKIT_MATERIALS_LEGACY_INVENTORY=/fixture/legacy-inventory.json",
        "HENUKIT_MATERIALS_OSS_AUDIT_ROOT=/fixture/oss-audit",
        "HENUKIT_MATERIALS_ACTIVATION_STAGING_ROOT=/fixture/activation-staging",
        "LIBRARY_DATABASE_URL=postgres://fixed.example/library",
        "LIBRARY_OSS_ECS_RAM_ROLE=henukit-library-activation",
        "",
      ].join("\n"),
      { mode: 0o600 },
    );
    chmodSync(join(root, "materials-activate.env"), 0o600);
    writeFileSync(
      join(root, "activate-henukit-materials.mjs"),
      'import { writeFileSync } from "node:fs";\nwriteFileSync("/fixture/started", "started\\n");\nAtomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 5000);\n',
      { mode: 0o600 },
    );
    chmodSync(join(root, "activate-henukit-materials.mjs"), 0o600);
    writeFileSync(join(root, "build-henukit-library-activation-bundle.mjs"), "// fixed bundle builder\n", { mode: 0o600 });
    chmodSync(join(root, "build-henukit-library-activation-bundle.mjs"), 0o600);
    executable(join(root, "library-activate-public-release"), "#!/bin/sh\nexit 0\n");
    let wrapper = readFileSync(templatePath, "utf8")
      .replace('readonly config_path="/etc/henukit-deploy/materials-activate.env"', 'readonly config_path="/fixture/materials-activate.env"')
      .replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`)
      .replace('readonly lock_path="/run/henukit-materials-activate.lock"', 'readonly lock_path="/fixture/activation.lock"');
    writeFileSync(join(root, "henukit-materials-activate"), wrapper, { mode: 0o700 });
    chmodSync(join(root, "henukit-materials-activate"), 0o700);

    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
    const receipt = "c".repeat(64);
    const dockerArgs = [
      "run", "--name", container, "--rm", "-v", `${root}:/fixture`, "node:22-alpine",
      "/fixture/henukit-materials-activate", "--release-id", releaseID, "--receipt-sha256", receipt,
    ];
    const first = spawn("docker", dockerArgs, { stdio: ["ignore", "pipe", "pipe"] });
    let firstStderr = "";
    first.stderr.on("data", (chunk) => { firstStderr += chunk; });
    const deadline = Date.now() + 5000;
    while (!existsSync(join(root, "started")) && Date.now() < deadline) {
      await new Promise((resolvePromise) => setTimeout(resolvePromise, 25));
    }
    assert.equal(existsSync(join(root, "started")), true, `first activation did not reach the locked command: ${firstStderr}`);

    const concurrent = spawnSync("docker", [
      "exec", container, "/fixture/henukit-materials-activate",
      "--release-id", releaseID, "--receipt-sha256", receipt,
    ], { encoding: "utf8" });
    assert.notEqual(concurrent.status, 0);
    const firstResult = await new Promise((resolvePromise) => {
      first.on("close", (status) => resolvePromise({ status, stderr: firstStderr }));
    });
    assert.equal(firstResult.status, 0, firstResult.stderr);
  } finally {
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials installation keeps the activation program root-readable only", () => {
  const installer = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "install.sh"), "utf8");
  assert.match(
    installer,
    /install -o root -g root -m 0700 "\$source_dir\/deploy\/henukit-materials-activate" \/usr\/local\/libexec\/henukit\/henukit-materials-activate/,
  );
  assert.match(
    installer,
    /install -o root -g root -m 0600 "\$repo_root\/scripts\/ops\/activate-henukit-materials\.mjs" \/usr\/local\/libexec\/henukit\/activate-henukit-materials\.mjs/,
  );
  assert.match(installer, /build-henukit-library-activation-bundle\.mjs/);
  assert.match(installer, /\.\/cmd\/activate-public-release/);
  assert.match(installer, /library-activate-public-release/);
  assert.doesNotMatch(installer, /materials-activate\.env\.example.*materials-activate\.env/);
  assert.match(installer, /chown root:root \/etc\/henukit-deploy\/materials-postgresql\.conf/);
  assert.match(installer, /chmod 0600 \/etc\/henukit-deploy\/materials-postgresql\.conf/);
  assert.match(installer, /chmod 0600 \/etc\/henukit-deploy\/materials-legacy-inventory\.json/);
  assert.doesNotMatch(installer, /HENUKIT_MATERIALS_DATABASE_URL=/);
});
