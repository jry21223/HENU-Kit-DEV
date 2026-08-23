#!/usr/bin/env node
/**
 * Copy the five production-proven legacy Food photos from portal to food.
 *
 * Authentication stays in libpq service definitions (and preferably
 * .pgpass), so database URLs and passwords never appear in process arguments:
 *
 *   PGSERVICEFILE=/secure/path/pg_service.conf node \
 *     <release>/bin/import-legacy-portal-food-images.mjs \
 *     --source-service portal --target-service food
 *
 * The first run is read-only and validates all five source rows. Re-run with
 * --apply only after reviewing that result. The target transaction is
 * idempotent and refuses missing posts or conflicting existing image bytes.
 */

import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join, resolve } from "node:path";

const MAX_IMAGE_BYTES = 2 * 1024 * 1024;
const MAX_STORED_IMAGE_BYTES = 4 * 1024 * 1024;
const ALLOWED_CONTENT_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);
const DEFAULT_SANITIZER = join(dirname(fileURLToPath(import.meta.url)), "food-sanitize-post-image");

export const LEGACY_IMAGE_MAPPINGS = Object.freeze([
  Object.freeze({
    legacyPostID: "survey-01",
    foodPostID: "098458b1-f95e-5a4e-01bd-0c8f015c5c61",
    foodImageID: "2caa2eeb-c4fc-c9ce-cfb7-128d960d9782",
  }),
  Object.freeze({
    legacyPostID: "survey-02",
    foodPostID: "ba86bc17-886c-2a62-804a-cbbb09991a07",
    foodImageID: "263af4d4-708a-2416-b128-cbcb0f83aad6",
  }),
  Object.freeze({
    legacyPostID: "survey-03",
    foodPostID: "f730ede0-e04f-d96a-6f23-11758ae12b44",
    foodImageID: "b794fde7-7bc2-e626-d997-b6109b76aafc",
  }),
  Object.freeze({
    legacyPostID: "survey-06",
    foodPostID: "331fa019-945a-db31-db64-70f465e6c8e6",
    foodImageID: "118d94ad-6dbd-d8f9-6e14-f7e3fbfd7abe",
  }),
  Object.freeze({
    legacyPostID: "survey-07",
    foodPostID: "a814437c-b84f-7fdd-479f-ef9f95c34ca4",
    foodImageID: "5e2d910e-d4d4-3752-85cd-53f498b4567a",
  }),
]);

const sourceIDs = LEGACY_IMAGE_MAPPINGS
  .map(({ legacyPostID }) => `'${legacyPostID}'`)
  .join(", ");

export const SOURCE_SQL = `
SELECT COALESCE(
  json_agg(
    json_build_object(
      'post_id', post_id,
      'position', position,
      'content_type', content_type,
      'byte_size', byte_size,
      'sha256', sha256,
      'data_base64', encode(bytes, 'base64')
    ) ORDER BY post_id
  ),
  '[]'::json
)::text
FROM portal_food_post_images
WHERE position = 0
  AND post_id IN (${sourceIDs});
`.trim();

function strictBase64(value) {
  if (typeof value !== "string") throw new Error("legacy image base64 is missing");
  const normalized = value.replace(/[\r\n\t ]/g, "");
  if (
    normalized.length === 0 ||
    normalized.length % 4 !== 0 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(normalized)
  ) {
    throw new Error("legacy image base64 is invalid");
  }
  const bytes = Buffer.from(normalized, "base64");
  if (bytes.toString("base64") !== normalized) {
    throw new Error("legacy image base64 is not canonical");
  }
  return { normalized, bytes };
}

function matchesMagic(contentType, bytes) {
  if (contentType === "image/jpeg") {
    return bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff;
  }
  if (contentType === "image/png") {
    return bytes.length >= 8 && bytes.subarray(0, 8).equals(Buffer.from("89504e470d0a1a0a", "hex"));
  }
  return bytes.length >= 12 &&
    bytes.subarray(0, 4).toString("ascii") === "RIFF" &&
    bytes.subarray(8, 12).toString("ascii") === "WEBP";
}

export function validateLegacyImages(value) {
  if (!Array.isArray(value) || value.length !== LEGACY_IMAGE_MAPPINGS.length) {
    throw new Error(`expected exactly ${LEGACY_IMAGE_MAPPINGS.length} legacy images`);
  }
  const byLegacyID = new Map();
  for (const row of value) {
    const mapping = LEGACY_IMAGE_MAPPINGS.find(
      ({ legacyPostID }) => legacyPostID === row?.post_id,
    );
    if (!mapping || row?.position !== 0) {
      throw new Error("unexpected legacy image ID or position");
    }
    if (byLegacyID.has(mapping.legacyPostID)) {
      throw new Error("duplicate legacy image");
    }
    if (!ALLOWED_CONTENT_TYPES.has(row.content_type)) {
      throw new Error("legacy image content type is invalid");
    }
    const { normalized, bytes } = strictBase64(row.data_base64);
    if (
      !Number.isInteger(row.byte_size) ||
      row.byte_size < 1 ||
      row.byte_size > MAX_IMAGE_BYTES ||
      row.byte_size !== bytes.length
    ) {
      throw new Error("legacy image byte size is invalid");
    }
    if (!matchesMagic(row.content_type, bytes)) {
      throw new Error("legacy image content type does not match its magic bytes");
    }
    const calculatedSHA256 = createHash("sha256").update(bytes).digest("hex");
    if (
      typeof row.sha256 !== "string" ||
      !/^[0-9a-f]{64}$/.test(row.sha256) ||
      row.sha256 !== calculatedSHA256
    ) {
      throw new Error("legacy image SHA-256 is invalid");
    }
    byLegacyID.set(mapping.legacyPostID, Object.freeze({
      ...mapping,
      position: 0,
      contentType: row.content_type,
      byteSize: row.byte_size,
      sha256: calculatedSHA256,
      dataBase64: normalized,
    }));
  }
  return LEGACY_IMAGE_MAPPINGS.map(({ legacyPostID }) => byLegacyID.get(legacyPostID));
}

function validateSanitizedResult(value, mapping) {
  if (
    value === null ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !== "byte_size,content_type,data_base64,sha256"
  ) {
    throw new Error("Food owner sanitizer returned an invalid response");
  }
  if (!ALLOWED_CONTENT_TYPES.has(value.content_type)) {
    throw new Error("Food owner sanitizer returned an invalid content type");
  }
  const { normalized, bytes } = strictBase64(value.data_base64);
  if (
    !Number.isInteger(value.byte_size) ||
    value.byte_size < 1 ||
    value.byte_size > MAX_STORED_IMAGE_BYTES ||
    value.byte_size !== bytes.length
  ) {
    throw new Error("Food owner sanitizer returned an invalid byte size");
  }
  if (!matchesMagic(value.content_type, bytes)) {
    throw new Error("Food owner sanitizer content type does not match its magic bytes");
  }
  const calculatedSHA256 = createHash("sha256").update(bytes).digest("hex");
  if (value.sha256 !== calculatedSHA256) {
    throw new Error("Food owner sanitizer returned an invalid SHA-256");
  }
  return Object.freeze({
    ...mapping,
    position: 0,
    contentType: value.content_type,
    byteSize: value.byte_size,
    sha256: calculatedSHA256,
    dataBase64: normalized,
    sanitizedByFoodOwner: true,
  });
}

export function sanitizeLegacyImages(value, sanitizer) {
  if (typeof sanitizer !== "string" || sanitizer.trim() === "") {
    throw new Error("Food owner sanitizer executable is required");
  }
  return validateLegacyImages(value).map((row) => {
    const result = spawnSync(sanitizer, [], {
      encoding: "utf8",
      input: JSON.stringify({ content_type: row.contentType, data_base64: row.dataBase64 }),
      maxBuffer: 8 * 1024 * 1024,
    });
    if (result.error || result.status !== 0) {
      throw new Error(`Food owner sanitizer failed for ${row.legacyPostID}`);
    }
    let response;
    try {
      response = JSON.parse(result.stdout);
    } catch {
      throw new Error("Food owner sanitizer returned invalid JSON");
    }
    return validateSanitizedResult(response, row);
  });
}

function validateSanitizedImages(rows) {
  if (!Array.isArray(rows) || rows.length !== LEGACY_IMAGE_MAPPINGS.length) {
    throw new Error("import SQL requires exactly five Food-owner-sanitized images");
  }
  return rows.map((row, index) => {
    const mapping = LEGACY_IMAGE_MAPPINGS[index];
    if (
      row?.sanitizedByFoodOwner !== true ||
      row.legacyPostID !== mapping.legacyPostID ||
      row.foodPostID !== mapping.foodPostID ||
      row.foodImageID !== mapping.foodImageID ||
      row.position !== 0 ||
      !ALLOWED_CONTENT_TYPES.has(row.contentType)
    ) {
      throw new Error("Food-owner-sanitized image identity is invalid");
    }
    const { normalized, bytes } = strictBase64(row.dataBase64);
    const calculatedSHA256 = createHash("sha256").update(bytes).digest("hex");
    if (
      !Number.isInteger(row.byteSize) ||
      row.byteSize < 1 ||
      row.byteSize > MAX_STORED_IMAGE_BYTES ||
      row.byteSize !== bytes.length ||
      !matchesMagic(row.contentType, bytes) ||
      row.sha256 !== calculatedSHA256
    ) {
      throw new Error("Food-owner-sanitized image bytes are invalid");
    }
    return Object.freeze({ ...row, dataBase64: normalized });
  });
}

function sqlString(value) {
  return `'${String(value).replaceAll("'", "''")}'`;
}

export function buildImportSQL(rows) {
  const validated = validateSanitizedImages(rows);
  const postIDs = validated.map(({ foodPostID }) => `${sqlString(foodPostID)}::uuid`).join(", ");
  const insertValues = validated.map((row) => `(
    ${sqlString(row.foodImageID)}::uuid,
    ${sqlString(row.foodPostID)}::uuid,
    0,
    ${sqlString(row.contentType)},
    ${row.byteSize},
    ${sqlString(row.sha256)},
    decode(${sqlString(row.dataBase64)}, 'base64')
  )`).join(",\n  ");
  return `\\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE legacy_food_images_import (
  id uuid PRIMARY KEY,
  post_id uuid NOT NULL UNIQUE,
  position int NOT NULL CHECK (position = 0),
  content_type text NOT NULL,
  byte_size int NOT NULL,
  sha256 text NOT NULL,
  bytes bytea NOT NULL
) ON COMMIT DROP;

INSERT INTO pg_temp.legacy_food_images_import (
  id, post_id, position, content_type, byte_size, sha256, bytes
)
VALUES
  ${insertValues};

DO $preflight$
BEGIN
  IF (SELECT count(*) FROM food_posts WHERE id IN (${postIDs})) <> ${validated.length} THEN
    RAISE EXCEPTION 'legacy Food image target posts are incomplete';
  END IF;
END
$preflight$;

INSERT INTO food_post_images (
  id, post_id, position, content_type, byte_size, sha256, bytes
)
SELECT id, post_id, position, content_type, byte_size, sha256, bytes
FROM pg_temp.legacy_food_images_import
ON CONFLICT (post_id, position) DO NOTHING;

DO $verify$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_temp.legacy_food_images_import AS expected
    LEFT JOIN food_post_images AS actual
      ON actual.post_id = expected.post_id
     AND actual.position = expected.position
    WHERE actual.id IS DISTINCT FROM expected.id
       OR actual.content_type IS DISTINCT FROM expected.content_type
       OR actual.byte_size IS DISTINCT FROM expected.byte_size
       OR actual.sha256 IS DISTINCT FROM expected.sha256
       OR actual.bytes IS DISTINCT FROM expected.bytes
  ) THEN
    RAISE EXCEPTION 'legacy Food image conflicts with existing target bytes';
  END IF;
END
$verify$;

COMMIT;
`;
}

function parseArgs(argv) {
  const options = { apply: false, psql: "psql", sanitizer: DEFAULT_SANITIZER };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--apply") {
      options.apply = true;
      continue;
    }
    if (["--source-service", "--target-service", "--psql", "--sanitizer"].includes(argument)) {
      const value = argv[index + 1];
      if (!value || value.startsWith("--")) throw new Error(`${argument} requires a value`);
      options[argument.slice(2).replace("-service", "Service")] = value;
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${argument}`);
  }
  for (const key of ["sourceService", "targetService"]) {
    if (!options[key] || !/^[A-Za-z0-9._-]{1,128}$/.test(options[key])) {
      throw new Error(`--${key === "sourceService" ? "source" : "target"}-service is required`);
    }
  }
  if (options.sourceService === options.targetService) {
    throw new Error("source and target services must be different");
  }
  return options;
}

function psql(options, service, args, input, label) {
  const result = spawnSync(
    options.psql,
    [`--dbname=service=${service}`, "-X", "--set=ON_ERROR_STOP=1", ...args],
    { encoding: "utf8", input, maxBuffer: 16 * 1024 * 1024 },
  );
  if (result.error || result.status !== 0) {
    throw new Error(`${label} failed; no image data was printed`);
  }
  return result.stdout;
}

function runCLI(argv) {
  const options = parseArgs(argv);
  const exported = psql(
    options,
    options.sourceService,
    ["--tuples-only", "--no-align", "--quiet", "--command", SOURCE_SQL],
    undefined,
    "legacy Food image source read",
  );
  let sourceRows;
  try {
    sourceRows = JSON.parse(exported.trim());
  } catch {
    throw new Error("legacy Food image source returned invalid JSON");
  }
  const sanitized = sanitizeLegacyImages(sourceRows, options.sanitizer);
  if (!options.apply) {
    process.stdout.write(
      `Sanitized and validated ${sanitized.length} legacy Food images; no target writes were made. Re-run with --apply to import.\n`,
    );
    return;
  }
  psql(
    options,
    options.targetService,
    ["--file", "-"],
    buildImportSQL(sanitized),
    "legacy Food image target transaction",
  );
  process.stdout.write(`Imported and verified ${sanitized.length} legacy Food images.\n`);
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
  try {
    runCLI(process.argv.slice(2));
  } catch (error) {
    process.stderr.write(`legacy Food image import: ${error instanceof Error ? error.message : "failed"}\n`);
    process.exitCode = 1;
  }
}
