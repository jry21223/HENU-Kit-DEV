import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(
  new URL("../download-henukit-artifacts.sh", import.meta.url),
);
const script = readFileSync(scriptPath, "utf8");

test("download helper requires a full git SHA and successful Actions run", () => {
  assert.match(script, /\[0-9a-f\]\{40\}/);
  assert.match(script, /deploy-henukit\.yml/);
  assert.match(script, /gh run download/);
  assert.match(script, /conclusion" == "success"/);
});

test("download helper verifies the full primary image set and runtime tarball", () => {
  for (const image of [
    "henukit-console",
    "henukit-console-gateway",
    "henukit-platform-core",
    "henukit-platform-mail-worker",
    "henukit-platform-smtp-provider",
    "henukit-portal",
    "henukit-portal-api",
    "henukit-portal-gateway",
    "henukit-runtime",
  ]) {
    assert.match(script, new RegExp(image.replaceAll("-", "\\-")));
  }
  assert.match(script, /sha256sum -c/);
  assert.doesNotMatch(script, /docker build/);
  assert.doesNotMatch(script, /compose build/);
});
