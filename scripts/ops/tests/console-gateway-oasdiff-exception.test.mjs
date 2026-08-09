import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const exceptionFile = fileURLToPath(
  new URL("../../../docs/adr/0025-portal-notice-feed-oasdiff.err-ignore.txt", import.meta.url),
);
const workflowFile = fileURLToPath(
  new URL("../../../.github/workflows/console-gateway.yml", import.meta.url),
);
const contractFile = fileURLToPath(
  new URL("../../../packages/api-contracts/openapi/console-gateway.yaml", import.meta.url),
);

const expectedRules = [
  "GET /notices the `allOf[subschema #2]/data/items/items/source_url` response's property `format` changed from `uri` to `iri` for status `200`",
  "POST /notices/sources the `canonical_url` request property `format` changed from `uri` to `iri`",
  "POST /notices/sources/{source_id}/versions the `source_url` request property `format` changed from `uri` to `iri`",
];

function oasdiff(baseFile, revisionFile) {
  return spawnSync(
    "go",
    [
      "run",
      "github.com/oasdiff/oasdiff@v1.23.0",
      "breaking",
      "--color=never",
      "--fail-on",
      "ERR",
      "--err-ignore",
      exceptionFile,
      baseFile,
      revisionFile,
    ],
    { cwd: repoRoot, encoding: "utf8" },
  );
}

test("Console OAS exception is exact and leaves unrelated breaking changes gated", (t) => {
  const rules = readFileSync(exceptionFile, "utf8")
    .split(/\r?\n/)
    .filter((line) => line && !line.startsWith("#"));
  assert.deepEqual(rules, expectedRules);

  const workflow = readFileSync(workflowFile, "utf8");
  assert.match(
    workflow,
    /oasdiff@v1\.23\.0 breaking --err-ignore docs\/adr\/0025-portal-notice-feed-oasdiff\.err-ignore\.txt --fail-on ERR \/tmp\/console-gateway-base\.yaml packages\/api-contracts\/openapi\/console-gateway\.yaml/,
  );

  const contract = readFileSync(contractFile, "utf8");
  assert.equal((contract.match(/format: iri/g) ?? []).length, 3);
  const base = contract.replaceAll("format: iri", "format: uri");
  const unrelatedBreak = contract.replace(
    "title: { type: string, minLength: 1, maxLength: 200 }",
    "title: { type: integer }",
  );
  assert.notEqual(unrelatedBreak, contract, "fixture must contain the independent type change");

  const directory = mkdtempSync(join(tmpdir(), "hc301-console-oasdiff-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const baseFile = join(directory, "base.yaml");
  const compatibleRevision = join(directory, "iri-only.yaml");
  const breakingRevision = join(directory, "unrelated-break.yaml");
  writeFileSync(baseFile, base);
  writeFileSync(compatibleRevision, contract);
  writeFileSync(breakingRevision, unrelatedBreak);

  const intentionalRelaxation = oasdiff(baseFile, compatibleRevision);
  assert.equal(
    intentionalRelaxation.status,
    0,
    `${intentionalRelaxation.stdout}\n${intentionalRelaxation.stderr}`,
  );
  const unrelated = oasdiff(baseFile, breakingRevision);
  assert.notEqual(unrelated.status, 0, `${unrelated.stdout}\n${unrelated.stderr}`);
  assert.match(`${unrelated.stdout}\n${unrelated.stderr}`, /title.*type|type.*title/i);
});
