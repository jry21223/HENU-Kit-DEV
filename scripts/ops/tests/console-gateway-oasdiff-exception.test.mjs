import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const exceptionRelativePath = "docs/adr/0025-portal-notice-feed-oasdiff.err-ignore.txt";
const testRelativePath = "scripts/ops/tests/console-gateway-oasdiff-exception.test.mjs";
const exceptionFile = fileURLToPath(new URL(`../../../${exceptionRelativePath}`, import.meta.url));

const gates = [
  {
    name: "Notice",
    workflow: ".github/workflows/notice.yml",
    contract: "packages/api-contracts/openapi/notice.yaml",
    baseFile: "/tmp/notice-base.yaml",
    iriCount: 4,
  },
  {
    name: "Portal Gateway",
    workflow: ".github/workflows/portal-gateway.yml",
    contract: "packages/api-contracts/openapi/portal-gateway.yaml",
    baseFile: "/tmp/portal-gateway-base.yaml",
  },
  {
    name: "Console Gateway",
    workflow: ".github/workflows/console-gateway.yml",
    contract: "packages/api-contracts/openapi/console-gateway.yaml",
    baseFile: "/tmp/console-gateway-base.yaml",
    iriCount: 3,
  },
].map((gate) => ({
  ...gate,
  workflowFile: join(repoRoot, ...gate.workflow.split("/")),
  contractFile: join(repoRoot, ...gate.contract.split("/")),
}));

const expectedRules = [
  "GET /api/v1/console-notices the `data/items/items/source_url` response's property `format` changed from `uri` to `iri` for status `200`",
  "POST /api/v1/sources the `canonical_url` request property `format` changed from `uri` to `iri`",
  "POST /api/v1/sources/{source_id}/versions the `source_url` request property `format` changed from `uri` to `iri`",
  "GET /notices the `allOf[subschema #2]/data/items/items/source_url` response's property `format` changed from `uri` to `iri` for status `200`",
  "POST /notices/sources the `canonical_url` request property `format` changed from `uri` to `iri`",
  "POST /notices/sources/{source_id}/versions the `source_url` request property `format` changed from `uri` to `iri`",
];

function oasdiff(baseFile, revisionFile) {
  const result = spawnSync(
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
  assert.ifError(result.error);
  return result;
}

function replaceAfter(source, anchor, needle, replacement) {
  const anchorIndex = source.indexOf(anchor);
  assert.notEqual(anchorIndex, -1, `fixture must contain ${anchor}`);
  const before = source.slice(0, anchorIndex);
  const after = source.slice(anchorIndex);
  const revised = after.replace(needle, replacement);
  assert.notEqual(revised, after, `fixture after ${anchor} must contain ${needle}`);
  return before + revised;
}

function replaceFirst(source, needle, replacement, count) {
  let revised = source;
  for (let index = 0; index < count; index += 1) {
    const next = revised.replace(needle, replacement);
    assert.notEqual(next, revised, `fixture must contain ${count} occurrences of ${needle}`);
    revised = next;
  }
  return revised;
}

test("Portal Notice OAS gates use one exact ADR manifest and fail closed", () => {
  const rules = readFileSync(exceptionFile, "utf8")
    .split(/\r?\n/)
    .filter((line) => line && !line.startsWith("#"));
  assert.deepEqual(rules, expectedRules);

  for (const gate of gates) {
    const workflow = readFileSync(gate.workflowFile, "utf8");
    assert.match(workflow, /fetch-depth:\s*0/, `${gate.name} must fetch the PR base`);
    assert.ok(
      workflow.split(exceptionRelativePath).length >= 3,
      `${gate.name} must trigger on and consume the ADR manifest`,
    );
    assert.ok(
      workflow.split(testRelativePath).length >= 3,
      `${gate.name} must trigger on and execute the exact gate test`,
    );
    assert.ok(
      workflow.includes(`git cat-file -e "origin/\${{ github.base_ref }}:${gate.contract}" ||`),
      `${gate.name} must fail when the PR base contract is unavailable`,
    );
    assert.ok(
      workflow.includes(
        `go run github.com/oasdiff/oasdiff@v1.23.0 breaking --err-ignore ${exceptionRelativePath} --fail-on ERR ${gate.baseFile} ${gate.contract}`,
      ),
      `${gate.name} must fail on every ERR outside the ADR manifest`,
    );
  }
});

test("only the recorded Notice and Console URI-to-IRI diagnostics are ignored", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "hc301-portal-notice-oasdiff-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));

  for (const gate of gates.filter((candidate) => candidate.name !== "Portal Gateway")) {
    const contract = readFileSync(gate.contractFile, "utf8");
    assert.equal((contract.match(/format: iri/g) ?? []).length, gate.iriCount, `${gate.name} IRI fixture`);
    const base = replaceFirst(contract, "format: iri", "format: uri", 3);
    const unrelatedBreak = contract.replace(
      "title: { type: string, minLength: 1, maxLength: 200 }",
      "title: { type: integer }",
    );
    assert.notEqual(unrelatedBreak, contract, `${gate.name} unrelated fixture`);

    const baseFile = join(directory, `${gate.name}-base.yaml`);
    const compatibleRevision = join(directory, `${gate.name}-iri-only.yaml`);
    const breakingRevision = join(directory, `${gate.name}-unrelated-break.yaml`);
    writeFileSync(baseFile, base);
    writeFileSync(compatibleRevision, contract);
    writeFileSync(breakingRevision, unrelatedBreak);

    const intentionalRelaxation = oasdiff(baseFile, compatibleRevision);
    assert.equal(
      intentionalRelaxation.status,
      0,
      `${gate.name}\n${intentionalRelaxation.stdout}\n${intentionalRelaxation.stderr}`,
    );
    const unrelated = oasdiff(baseFile, breakingRevision);
    assert.notEqual(unrelated.status, 0, `${gate.name} unrelated ERR must fail`);
    assert.match(`${unrelated.stdout}\n${unrelated.stderr}`, /title.*type|type.*title/i);
  }

  const consoleGate = gates.find((gate) => gate.name === "Console Gateway");
  const consoleContract = readFileSync(consoleGate.contractFile, "utf8");
  const unrecordedIRI = consoleContract.replace(
    "source_resource_url: { type: string, format: uri }",
    "source_resource_url: { type: string, format: iri }",
  );
  assert.notEqual(unrecordedIRI, consoleContract, "unrecorded IRI fixture");
  const consoleBase = join(directory, "console-current.yaml");
  const consoleUnrecorded = join(directory, "console-unrecorded-iri.yaml");
  writeFileSync(consoleBase, consoleContract);
  writeFileSync(consoleUnrecorded, unrecordedIRI);
  const unrecorded = oasdiff(consoleBase, consoleUnrecorded);
  assert.notEqual(unrecorded.status, 0, "an unrecorded URI-to-IRI diagnostic must fail");

  const portalGate = gates.find((gate) => gate.name === "Portal Gateway");
  const portalContract = readFileSync(portalGate.contractFile, "utf8");
  const portalBreak = replaceAfter(portalContract, "    PortalNotice:", "type: string", "type: integer");
  const portalBase = join(directory, "portal-current.yaml");
  const portalBreaking = join(directory, "portal-unrelated-break.yaml");
  writeFileSync(portalBase, portalContract);
  writeFileSync(portalBreaking, portalBreak);
  const portalUnrelated = oasdiff(portalBase, portalBreaking);
  assert.notEqual(portalUnrelated.status, 0, "Portal Gateway unrelated ERR must fail");
});
