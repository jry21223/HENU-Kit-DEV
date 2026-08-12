import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const inventory = fileURLToPath(
  new URL("../henukit-release-images.sh", import.meta.url),
);
const runtimeCompose = readFileSync(
  new URL("../../../docker-compose.henukit.yml", import.meta.url),
  "utf8",
);
const prebuiltCompose = readFileSync(
  new URL("../../../docker-compose.henukit.prebuilt.yml", import.meta.url),
  "utf8",
);

const expected = [
  ["console", "henukit-console", "console", "baseline"],
  ["console-gateway", "henukit-console-gateway", "console-gateway", "baseline"],
  ["platform-core", "henukit-platform-core", "platform-core", "baseline"],
  ["platform-mail-worker", "henukit-platform-mail-worker", "platform-mail-worker", "baseline"],
  ["platform-smtp-provider", "henukit-platform-smtp-provider", "platform-smtp-provider", "baseline"],
  ["portal", "henukit-portal", "portal", "baseline"],
  ["portal-summary", "henukit-portal-summary", "portal-summary", "baseline"],
  ["portal-api", "henukit-portal-api", "portal-api", "baseline"],
  ["account-portfolio", "henukit-account-portfolio", "account-portfolio", "conditional"],
  ["notice", "henukit-notice", "notice", "conditional"],
  ["notice-worker", "henukit-notice-worker", "notice-worker", "conditional"],
  ["food", "henukit-food", "food", "conditional"],
  ["library", "henukit-library", "library", "conditional"],
  ["portal-gateway", "henukit-portal-gateway", "portal-gateway", "baseline"],
];

function lines(...args) {
  const output = execFileSync(inventory, args, { encoding: "utf8" }).trim();
  return output === "" ? [] : output.split("\n");
}

test("HENU release image inventory is one validated source for artifacts and runtime roles", () => {
  execFileSync(inventory, ["--check"], { stdio: "pipe" });

  assert.deepEqual(
    lines("--records").map((line) => line.split("\t")),
    expected,
  );
  assert.deepEqual(lines("--artifact-images"), expected.map(([, image]) => image));
  assert.deepEqual(lines("--load-images"), expected.map(([, image]) => image));
  assert.deepEqual(
    lines("--baseline-images"),
    expected.filter(([, , , role]) => role === "baseline").map(([, image]) => image),
  );
  assert.deepEqual(
    lines("--conditional-services").map((line) => line.split("\t")),
    expected
      .filter(([, , , role]) => role === "conditional")
      .map(([, image, service]) => [service, image]),
  );
  for (const [, image, service] of expected) {
    assert.match(
      runtimeCompose,
      new RegExp(`^  ${service}:`, "m"),
      `${service} must remain in the base Compose contract`,
    );
    assert.match(
      prebuiltCompose,
      new RegExp(`^  ${service}:\\n[\\s\\S]*?^    image: ${image}:`, "m"),
      `${image} must remain in the fixed-SHA prebuilt Compose contract`,
    );
  }
});

test("the GitHub matrix comes from the inventory, including Library", () => {
  const matrix = JSON.parse(execFileSync(inventory, ["--github-matrix"], { encoding: "utf8" }));

  assert.deepEqual(
    matrix.include.map(({ name, image }) => [name, image]),
    expected.map(([name, image]) => [name, image]),
  );
  assert.deepEqual(
    matrix.include.find(({ name }) => name === "library"),
    {
      name: "library",
      image: "henukit-library",
      context: "services/library",
      dockerfile: "services/library/Dockerfile",
      build_args: "",
    },
  );
  assert.match(
    matrix.include.find(({ name }) => name === "portal").build_args,
    /NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1/,
  );
});
