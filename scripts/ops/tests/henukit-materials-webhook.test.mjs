import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  createWebhookServer,
  isMirroredPush,
  signatureMatches,
} from "../henukit-materials-webhook.mjs";

const SECRET = "materials-webhook-secret-with-enough-entropy";

function sign(secret, body) {
  return "sha256=" + createHmac("sha256", secret).update(body).digest("hex");
}

test("signatureMatches accepts only the signature GitHub would send", () => {
  const body = Buffer.from(JSON.stringify({ ref: "refs/heads/main" }));

  assert.equal(signatureMatches(SECRET, body, sign(SECRET, body)), true);
  assert.equal(signatureMatches(SECRET, body, sign("another-secret", body)), false);
  assert.equal(signatureMatches(SECRET, body, undefined), false);
  assert.equal(signatureMatches(SECRET, body, ""), false);
  // A bare digest without the algorithm prefix must not pass.
  assert.equal(
    signatureMatches(SECRET, body, createHmac("sha256", SECRET).update(body).digest("hex")),
    false
  );
  // Truncating the digest must not pass either.
  assert.equal(signatureMatches(SECRET, body, sign(SECRET, body).slice(0, 20)), false);
  // A different body under the right secret is still a mismatch.
  assert.equal(signatureMatches(SECRET, Buffer.from("{}"), sign(SECRET, body)), false);
});

test("isMirroredPush only matches a push to the mirrored branch", () => {
  assert.equal(isMirroredPush("push", { ref: "refs/heads/main" }, "main"), true);
  assert.equal(isMirroredPush("push", { ref: "refs/heads/draft" }, "main"), false);
  assert.equal(isMirroredPush("push", { ref: "refs/tags/v1" }, "main"), false);
  assert.equal(isMirroredPush("pull_request", { ref: "refs/heads/main" }, "main"), false);
  assert.equal(isMirroredPush("push", null, "main"), false);
});

/** Starts the server on an ephemeral port and returns a request helper. */
async function withServer(syncScript, run) {
  const server = createWebhookServer({
    secret: SECRET,
    ref: "main",
    sync: syncScript,
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();

  const post = async (body, headers) =>
    fetch(`http://127.0.0.1:${port}/webhook/materials`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...headers },
      body,
    });

  try {
    await run({ post, port });
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

/** A stand-in sync that records each run, so triggering can be asserted. */
function makeMarkerScript(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "materials-webhook-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const marker = path.join(dir, "runs");
  const script = path.join(dir, "sync.sh");
  fs.writeFileSync(script, `#!/usr/bin/env bash\necho run >> ${JSON.stringify(marker)}\n`);
  return { script, runs: () => (fs.existsSync(marker) ? fs.readFileSync(marker, "utf8").trim().split("\n").length : 0) };
}

test("a correctly signed push triggers the mirror", async (t) => {
  const { script, runs } = makeMarkerScript(t);
  const body = JSON.stringify({ ref: "refs/heads/main" });

  await withServer(script, async ({ post }) => {
    const response = await post(body, {
      "X-GitHub-Event": "push",
      "X-Hub-Signature-256": sign(SECRET, body),
    });
    assert.equal(response.status, 202);

    // The reply is sent before the sync finishes, so wait for the marker.
    for (let attempt = 0; attempt < 50 && runs() === 0; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    assert.equal(runs(), 1);
  });
});

test("an unsigned or wrongly signed push never runs the mirror", async (t) => {
  const { script, runs } = makeMarkerScript(t);
  const body = JSON.stringify({ ref: "refs/heads/main" });

  await withServer(script, async ({ post }) => {
    assert.equal((await post(body, { "X-GitHub-Event": "push" })).status, 401);
    assert.equal(
      (
        await post(body, {
          "X-GitHub-Event": "push",
          "X-Hub-Signature-256": sign("wrong-secret-with-enough-entropy", body),
        })
      ).status,
      401
    );

    await new Promise((resolve) => setTimeout(resolve, 100));
    assert.equal(runs(), 0);
  });
});

test("a push to another branch is accepted but does not mirror", async (t) => {
  const { script, runs } = makeMarkerScript(t);
  const body = JSON.stringify({ ref: "refs/heads/draft" });

  await withServer(script, async ({ post }) => {
    const response = await post(body, {
      "X-GitHub-Event": "push",
      "X-Hub-Signature-256": sign(SECRET, body),
    });
    assert.equal(response.status, 202);

    await new Promise((resolve) => setTimeout(resolve, 100));
    assert.equal(runs(), 0);
  });
});

test("ping is answered without mirroring", async (t) => {
  const { script, runs } = makeMarkerScript(t);
  const body = JSON.stringify({ zen: "Keep it logically awesome." });

  await withServer(script, async ({ post }) => {
    const response = await post(body, {
      "X-GitHub-Event": "ping",
      "X-Hub-Signature-256": sign(SECRET, body),
    });
    assert.equal(response.status, 200);

    await new Promise((resolve) => setTimeout(resolve, 100));
    assert.equal(runs(), 0);
  });
});

test("only the webhook path and POST are served", async (t) => {
  const { script } = makeMarkerScript(t);

  await withServer(script, async ({ port }) => {
    const wrongPath = await fetch(`http://127.0.0.1:${port}/`, { method: "POST" });
    assert.equal(wrongPath.status, 404);

    const wrongMethod = await fetch(`http://127.0.0.1:${port}/webhook/materials`);
    assert.equal(wrongMethod.status, 404);
  });
});
