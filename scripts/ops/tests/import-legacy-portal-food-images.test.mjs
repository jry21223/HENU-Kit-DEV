import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
  LEGACY_IMAGE_MAPPINGS,
  SOURCE_SQL,
  buildImportSQL,
  validateLegacyImages,
} from "../import-legacy-portal-food-images.mjs";

const importer = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "import-legacy-portal-food-images.mjs",
);

const onePixelPNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
  "base64",
);

function sourceRows() {
  return LEGACY_IMAGE_MAPPINGS.map(({ legacyPostID }) => ({
    post_id: legacyPostID,
    position: 0,
    content_type: "image/png",
    byte_size: onePixelPNG.length,
    sha256: createHash("sha256").update(onePixelPNG).digest("hex"),
    data_base64: onePixelPNG.toString("base64"),
  }));
}

function sanitizedRows() {
  return LEGACY_IMAGE_MAPPINGS.map((mapping) => ({
    ...mapping,
    position: 0,
    contentType: "image/png",
    byteSize: onePixelPNG.length,
    sha256: createHash("sha256").update(onePixelPNG).digest("hex"),
    dataBase64: onePixelPNG.toString("base64"),
    sanitizedByFoodOwner: true,
  }));
}

function md5UUID(value) {
  const hash = createHash("md5").update(value).digest("hex");
  return [hash.slice(0, 8), hash.slice(8, 12), hash.slice(12, 16), hash.slice(16, 20), hash.slice(20)].join("-");
}

test("source query and import SQL are pinned to the five proven legacy photos", () => {
  assert.match(SOURCE_SQL, /FROM portal_food_post_images/);
  assert.match(SOURCE_SQL, /position = 0/);
  for (const { legacyPostID, foodPostID, foodImageID } of LEGACY_IMAGE_MAPPINGS) {
    assert.match(SOURCE_SQL, new RegExp(`'${legacyPostID}'`));
    assert.equal(
      SOURCE_SQL.match(new RegExp(`'${legacyPostID}'`, "g"))?.length,
      1,
    );
    assert.equal(foodPostID, md5UUID(`portal-food-post:${legacyPostID}`));
    assert.equal(foodImageID, md5UUID(`portal-food-post-image:${legacyPostID}:0`));
    assert.match(buildImportSQL(sanitizedRows()), new RegExp(foodPostID));
  }

  const sql = buildImportSQL(sanitizedRows());
  assert.match(sql, /^\\set ON_ERROR_STOP on\nBEGIN;/);
  assert.match(sql, /ON CONFLICT \(post_id, position\) DO NOTHING/);
  assert.match(sql, /legacy Food image target posts are incomplete/);
  assert.match(sql, /legacy Food image conflicts with existing target bytes/);
  assert.match(sql, /COMMIT;\s*$/);
});

test("import SQL refuses legacy container bytes that did not pass the Food owner sanitizer", () => {
  assert.throws(() => buildImportSQL(sourceRows()), /Food-owner-sanitized/);
});

test("validation rejects missing, unexpected, duplicate, or nonzero-position rows", () => {
  const rows = sourceRows();
  assert.throws(() => validateLegacyImages(rows.slice(1)), /exactly 5/);
  assert.throws(
    () => validateLegacyImages([...rows.slice(0, 4), { ...rows[4], post_id: "survey-99" }]),
    /unexpected legacy image/,
  );
  assert.throws(
    () => validateLegacyImages([...rows.slice(0, 4), { ...rows[0] }]),
    /duplicate legacy image/,
  );
  assert.throws(
    () => validateLegacyImages(rows.map((row, index) => index === 0 ? { ...row, position: 1 } : row)),
    /unexpected legacy image/,
  );
});

test("validation checks content type, byte size, magic bytes, base64, and SHA-256", () => {
  const cases = [
    [{ content_type: "image/gif" }, /content type/],
    [{ byte_size: onePixelPNG.length + 1 }, /byte size/],
    [{ data_base64: "not base64" }, /base64/],
    [{ content_type: "image/jpeg" }, /magic bytes/],
    [{ sha256: "0".repeat(64) }, /SHA-256/],
  ];
  for (const [patch, error] of cases) {
    const rows = sourceRows();
    rows[0] = { ...rows[0], ...patch };
    assert.throws(() => validateLegacyImages(rows), error);
  }
});

test("CLI is read-only by default and writes only with an explicit --apply", () => {
  const fixture = mkdtempSync(join(tmpdir(), "henukit-food-image-import-"));
  const fakePSQL = join(fixture, "psql");
  const fakeSanitizer = join(fixture, "food-sanitize-post-image");
  const calls = join(fixture, "calls.jsonl");
  const targetSQL = join(fixture, "target.sql");
  writeFileSync(fakePSQL, `#!/usr/bin/env node
const fs = require("node:fs");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.FAKE_CALLS, JSON.stringify(args) + "\\n");
if (args.includes("--dbname=service=portal")) {
  process.stdout.write(process.env.FAKE_SOURCE_JSON);
} else if (args.includes("--dbname=service=food")) {
  fs.writeFileSync(process.env.FAKE_TARGET_SQL, fs.readFileSync(0, "utf8"));
} else {
  process.exitCode = 9;
}
`);
  chmodSync(fakePSQL, 0o700);
  writeFileSync(fakeSanitizer, `#!/usr/bin/env node
const fs = require("node:fs");
const input = JSON.parse(fs.readFileSync(0, "utf8"));
if (input.content_type !== "image/png" || !input.data_base64) process.exit(8);
const bytes = Buffer.from(input.data_base64, "base64");
const sha256 = require("node:crypto").createHash("sha256").update(bytes).digest("hex");
process.stdout.write(JSON.stringify({ content_type: "image/png", byte_size: bytes.length, sha256, data_base64: bytes.toString("base64") }));
`);
  chmodSync(fakeSanitizer, 0o700);
  const env = {
    ...process.env,
    FAKE_CALLS: calls,
    FAKE_TARGET_SQL: targetSQL,
    FAKE_SOURCE_JSON: JSON.stringify(sourceRows()),
  };
  try {
    const dryRun = spawnSync(process.execPath, [
      importer,
      "--source-service", "portal",
      "--target-service", "food",
      "--psql", fakePSQL,
      "--sanitizer", fakeSanitizer,
    ], { encoding: "utf8", env });
    assert.equal(dryRun.status, 0, dryRun.stderr);
    assert.match(dryRun.stdout, /no target writes were made/);
    assert.doesNotMatch(dryRun.stdout, new RegExp(sourceRows()[0].data_base64));
    assert.equal(readFileSync(calls, "utf8").trim().split("\n").length, 1);
    assert.equal(existsSync(targetSQL), false);

    const apply = spawnSync(process.execPath, [
      importer,
      "--source-service", "portal",
      "--target-service", "food",
      "--psql", fakePSQL,
      "--sanitizer", fakeSanitizer,
      "--apply",
    ], { encoding: "utf8", env });
    assert.equal(apply.status, 0, apply.stderr);
    assert.match(apply.stdout, /Imported and verified 5 legacy Food images/);
    assert.match(readFileSync(targetSQL, "utf8"), /BEGIN;[\s\S]*COMMIT;/);
    const recorded = readFileSync(calls, "utf8").trim().split("\n").map(JSON.parse);
    assert.equal(recorded.length, 3);
    assert(recorded.every((args) => args.every((arg) => !arg.includes("postgres://"))));
  } finally {
    rmSync(fixture, { recursive: true, force: true });
  }
});

const dockerAvailable = spawnSync(
  "docker",
  ["version", "--format", "{{.Server.Version}}"],
  { encoding: "utf8", timeout: 5_000 },
).status === 0;
const integration = dockerAvailable ? test : test.skip;

function docker(args, { allowFailure = false, input } = {}) {
  const result = spawnSync("docker", args, { encoding: "utf8", input, timeout: 10_000 });
  if (!allowFailure) assert.equal(result.status, 0, result.stderr);
  return result;
}

function waitForPostgres(container) {
  const sleep = (milliseconds) => Atomics.wait(
    new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds,
  );
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const result = docker([
      "exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc", "SELECT 1",
    ], { allowFailure: true });
    if (result.error?.code === "ETIMEDOUT") {
      assert.fail("Docker timed out while waiting for the PostgreSQL fixture");
    }
    if (result.status === 0 && result.stdout.trim() === "1") return;
    sleep(100);
  }
  assert.fail("PostgreSQL fixture did not become ready");
}

function psql(container, sql, { allowFailure = false } = {}) {
  return docker([
    "exec", "-i", container, "psql", "-U", "fixture", "-d", "fixture",
    "-v", "ON_ERROR_STOP=1", "-f", "-",
  ], { allowFailure, input: sql });
}

integration("target import fails closed, is idempotent, and never overwrites different bytes", () => {
  const container = `henukit-food-image-migration-${process.pid}-${Date.now()}`;
  const mappings = LEGACY_IMAGE_MAPPINGS;
  const sql = buildImportSQL(sanitizedRows());
  try {
    docker([
      "run", "--rm", "-d", "--name", container,
      "-e", "POSTGRES_USER=fixture", "-e", "POSTGRES_PASSWORD=fixture",
      "-e", "POSTGRES_DB=fixture", "postgres:17-alpine",
    ]);
    waitForPostgres(container);
    psql(container, `
      CREATE TABLE food_posts (id uuid PRIMARY KEY);
      CREATE TABLE food_post_images (
        id uuid PRIMARY KEY,
        post_id uuid NOT NULL REFERENCES food_posts(id) ON DELETE CASCADE,
        position int NOT NULL,
        content_type text NOT NULL,
        byte_size int NOT NULL,
        sha256 text NOT NULL,
        bytes bytea NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        UNIQUE (post_id, position)
      );
      INSERT INTO food_posts(id) VALUES
        ${mappings.slice(0, 4).map(({ foodPostID }) => `('${foodPostID}')`).join(",\n        ")};
    `);

    const missingTarget = psql(container, sql, { allowFailure: true });
    assert.notEqual(missingTarget.status, 0);
    assert.match(missingTarget.stderr, /target posts are incomplete/);
    assert.equal(
      docker(["exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc", "SELECT count(*) FROM food_post_images"]).stdout.trim(),
      "0",
    );

    psql(container, `INSERT INTO food_posts(id) VALUES ('${mappings[4].foodPostID}');`);
    psql(container, sql);
    psql(container, sql);
    assert.equal(
      docker(["exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc", "SELECT count(*) FROM food_post_images"]).stdout.trim(),
      "5",
    );

    psql(container, `
      UPDATE food_post_images
      SET sha256 = '${"0".repeat(64)}', bytes = decode('AA==', 'base64'), byte_size = 1
      WHERE post_id = '${mappings[0].foodPostID}' AND position = 0;
    `);
    const conflictingTarget = psql(container, sql, { allowFailure: true });
    assert.notEqual(conflictingTarget.status, 0);
    assert.match(conflictingTarget.stderr, /conflicts with existing target bytes/);
    assert.equal(
      docker(["exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc", `SELECT sha256 FROM food_post_images WHERE post_id='${mappings[0].foodPostID}' AND position=0`]).stdout.trim(),
      "0".repeat(64),
    );
  } finally {
    docker(["rm", "-f", container], { allowFailure: true });
  }
});
