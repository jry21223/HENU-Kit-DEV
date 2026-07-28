import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/account-portfolio.yml", import.meta.url),
  "utf8",
);

test("Account Portfolio CI enforces Go vulnerability and image scans", () => {
  assert.match(workflow, /golang\.org\/x\/vuln\/cmd\/govulncheck@v1\.6\.0 \.\/\.\.\./);
  assert.match(workflow, /aquasec\/trivy:0\.70\.0 fs --scanners secret --exit-code 1/);
  assert.match(workflow, /aquasec\/trivy:0\.70\.0 image --scanners vuln,secret --severity HIGH,CRITICAL --exit-code 1 henukit-account-portfolio/);
  assert.match(workflow, /go build -o \/tmp\/account-portfolio-healthcheck \.\/cmd\/healthcheck/);
});
