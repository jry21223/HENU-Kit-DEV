import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-henukit.yml", import.meta.url),
  "utf8",
);
const step = workflow.split("      - name: Build fixed-SHA image\n")[1]
  ?.split("\n      - name:")[0];
assert.ok(step, "the real Actions build step must exist");
const script = step.split("        run: |\n")[1]
  .split("\n")
  .map((line) => line.replace(/^ {10}/, ""))
  .join("\n");

function runBuild(mode, platform = "linux/amd64") {
  const fixture = mkdtempSync(join(tmpdir(), "henukit-actions-build-"));
  const source = join(fixture, "source");
  const bin = join(fixture, "bin");
  const attempts = join(fixture, "attempts");
  mkdirSync(source);
  mkdirSync(bin);
  try {
    writeFileSync(join(source, "Dockerfile"), "FROM scratch\n");
    writeFileSync(join(source, "source.txt"), "archived source\n");
    execFileSync("git", ["init", "-q", source]);
    execFileSync("git", ["-C", source, "add", "Dockerfile", "source.txt"]);
    execFileSync("git", [
      "-C", source, "-c", "commit.gpgsign=false",
      "-c", "core.hooksPath=/dev/null", "-c", "user.name=CI fixture",
      "-c", "user.email=ci@example.invalid", "commit", "-qm", "fixture",
    ]);
    const sha = execFileSync("git", ["-C", source, "rev-parse", "HEAD"], {
      encoding: "utf8",
    }).trim();
    writeFileSync(join(source, "not-from-commit.txt"), "must not be built\n");
    writeFileSync(join(bin, "docker"), String.raw`#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == build ]]; then
  attempt=1
  [[ ! -f "$TEST_ATTEMPTS" ]] || attempt=$(( $(cat "$TEST_ATTEMPTS") + 1 ))
  printf '%s' "$attempt" > "$TEST_ATTEMPTS"
  printf '%s\0' "$@" > "$TEST_ATTEMPTS.$attempt.args"
  for context; do :; done
  [[ "$(cat "$context/source.txt")" == 'archived source' ]]
  [[ ! -e "$context/not-from-commit.txt" ]]
  shopt -s nullglob dotglob
  set -- "$context"/*
  [[ "$#" == 2 ]]
  if [[ "$TEST_MODE" == tls-always || "$TEST_MODE" == tls-once && "$attempt" == 1 || "$TEST_MODE" == tls-then-compile && "$attempt" == 1 ]]; then
    printf '%s\n' '#2 ERROR: failed to do request: Head "https://docker.m.daocloud.io/v2/library/golang/manifests/1.26.6-alpine": net/http: TLS handshake timeout' >&2
    exit 17
  fi
  if [[ "$TEST_MODE" == compile || "$TEST_MODE" == tls-then-compile ]]; then
    printf '%s\n' 'ERROR: compiler rejected source' >&2
    exit 42
  fi
  if [[ "$TEST_MODE" == compile-tls ]]; then
    printf '%s\n' '#5 0.123 failed to do request: Head "https://example.invalid/v2/dependency": net/http: TLS handshake timeout' >&2
    exit 42
  fi
  if [[ "$TEST_MODE" == auth ]]; then
    printf '%s\n' '#2 ERROR: failed to do request: Head "https://docker.m.daocloud.io/v2/library/golang/manifests/1.26.6-alpine": 401 unauthorized' >&2
    exit 23
  fi
  exit 0
fi
[[ "$1" == image && "$2" == inspect ]]
printf '%s\n' "$TEST_PLATFORM"
`, { mode: 0o755 });
    writeFileSync(join(bin, "sleep"), "#!/bin/sh\nexit 0\n", { mode: 0o755 });
    const result = spawnSync("bash", ["-c", script], {
      cwd: source,
      encoding: "utf8",
      timeout: 10_000,
      env: {
        ...process.env,
        PATH: `${bin}:${process.env.PATH}`,
        GITHUB_SHA: sha,
        IMAGE: "henukit-fixture",
        CONTEXT: ".",
        DOCKERFILE: "Dockerfile",
        BUILD_ARGS: "PUBLIC_FLAG=1\nLABEL=two words",
        TEST_ATTEMPTS: attempts,
        TEST_MODE: mode,
        TEST_PLATFORM: platform,
      },
    });
    assert.equal(result.signal, null, result.stderr);
    const count = Number(readFileSync(attempts, "utf8"));
    for (let attempt = 1; attempt <= count; attempt++) {
      assert.deepEqual(
        readFileSync(`${attempts}.${attempt}.args`, "utf8").split("\0").slice(0, -1),
        [
          "build", "--platform", "linux/amd64",
          "--build-arg", "PUBLIC_FLAG=1", "--build-arg", "LABEL=two words",
          "--file", "Dockerfile", "--tag", `henukit-fixture:${sha}`, ".",
        ],
        "every attempt must keep the original source identity and build arguments",
      );
    }
    return { status: result.status, count, stderr: result.stderr };
  } finally {
    rmSync(fixture, { recursive: true, force: true });
  }
}

test("a transient base-image metadata TLS failure still produces the fixed-SHA build", () => {
  const result = runBuild("tls-once");
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.count, 2);
});

test("an exhausted base-image TLS failure fails closed after three attempts", () => {
  const result = runBuild("tls-always");
  assert.equal(result.status, 17, result.stderr);
  assert.equal(result.count, 3);
});

test("a compilation failure is not retried", () => {
  const result = runBuild("compile");
  assert.equal(result.status, 42, result.stderr);
  assert.equal(result.count, 1);
});

test("a compilation failure after a transient TLS failure stops immediately", () => {
  const result = runBuild("tls-then-compile");
  assert.equal(result.status, 42, result.stderr);
  assert.equal(result.count, 2);
});

test("a compiler TLS message cannot trigger a base-image retry", () => {
  const result = runBuild("compile-tls");
  assert.equal(result.status, 42, result.stderr);
  assert.equal(result.count, 1);
});

test("a registry authentication failure is not retried", () => {
  const result = runBuild("auth");
  assert.equal(result.status, 23, result.stderr);
  assert.equal(result.count, 1);
});

test("a successful build still rejects the wrong image platform", () => {
  const result = runBuild("success", "linux/arm64");
  assert.equal(result.status, 1, result.stderr);
  assert.equal(result.count, 1);
});

test("an immediately successful build needs no retry", () => {
  const result = runBuild("success");
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.count, 1);
});
