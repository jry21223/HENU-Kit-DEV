#!/usr/bin/env node
// Re-mirror the course materials whenever HENU-Final-Review is pushed.
//
// GitHub delivers a push event here; this process verifies the signature and
// runs sync-henukit-materials.sh. It binds to localhost by default and expects
// the host TLS terminator to forward to it, so the signature is a second gate
// rather than the only one.
//
// Environment:
//   HENUKIT_MATERIALS_WEBHOOK_SECRET  required, >= 32 bytes, matches GitHub
//   HENUKIT_MATERIALS_WEBHOOK_ADDR    default 127.0.0.1
//   HENUKIT_MATERIALS_WEBHOOK_PORT    default 8099
//   HENUKIT_MATERIALS_REPO_REF        branch to mirror, default main
//   HENUKIT_MATERIALS_SYNC            path to sync-henukit-materials.sh

import { spawn } from "node:child_process";
import { createHmac, timingSafeEqual } from "node:crypto";
import { createServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));

const SECRET = process.env.HENUKIT_MATERIALS_WEBHOOK_SECRET ?? "";
const ADDR = process.env.HENUKIT_MATERIALS_WEBHOOK_ADDR ?? "127.0.0.1";
const PORT = Number(process.env.HENUKIT_MATERIALS_WEBHOOK_PORT ?? 8099);
const REF = process.env.HENUKIT_MATERIALS_REPO_REF ?? "main";
const SYNC =
  process.env.HENUKIT_MATERIALS_SYNC ??
  path.join(HERE, "sync-henukit-materials.sh");

/** A push payload is a few KB; anything larger is not one of ours. */
const MAX_BODY_BYTES = 1024 * 1024;

/**
 * Verifies GitHub's HMAC over the raw body.
 *
 * Compares in constant time, and only after checking the length, because
 * timingSafeEqual throws on a length mismatch.
 */
export function signatureMatches(secret, body, header) {
  const prefix = "sha256=";
  if (typeof header !== "string" || !header.startsWith(prefix)) return false;

  const received = Buffer.from(header.slice(prefix.length), "hex");
  const expected = createHmac("sha256", secret).update(body).digest();
  if (received.length !== expected.length) return false;
  return timingSafeEqual(received, expected);
}

/** True when this push touched the branch we mirror. */
export function isMirroredPush(event, payload, ref) {
  return event === "push" && payload?.ref === `refs/heads/${ref}`;
}

/**
 * Runs the sync, collapsing concurrent requests.
 *
 * GitHub can deliver several pushes in a burst and the mirror rebuilds from the
 * manifest every time, so a run already in flight already covers them. One
 * queued rerun is kept so a push that landed mid-run is not missed.
 */
function createSyncRunner(command) {
  let running = null;
  let queued = false;

  const run = () => {
    running = new Promise((resolve) => {
      const child = spawn("bash", [command], { stdio: ["ignore", "pipe", "pipe"] });
      const log = (stream, prefix) => {
        stream.on("data", (chunk) => {
          for (const line of String(chunk).split("\n")) {
            if (line.trim()) console.log(`${prefix} ${line}`);
          }
        });
      };
      log(child.stdout, "sync:");
      log(child.stderr, "sync!");

      child.on("error", (error) => {
        console.error(`materials sync failed to start: ${error.message}`);
        resolve(false);
      });
      child.on("close", (code) => {
        if (code === 0) {
          console.log("materials sync completed");
        } else {
          console.error(`materials sync exited with ${code}`);
        }
        resolve(code === 0);
      });
    }).then(async (ok) => {
      running = null;
      if (queued) {
        queued = false;
        return run();
      }
      return ok;
    });
    return running;
  };

  return () => {
    if (running) {
      queued = true;
      return running;
    }
    return run();
  };
}

export function createWebhookServer({ secret, ref, sync }) {
  const runSync = createSyncRunner(sync);

  return createServer((request, response) => {
    const reply = (status, message) => {
      response.writeHead(status, { "Content-Type": "text/plain; charset=utf-8" });
      response.end(message);
    };

    if (request.method !== "POST" || request.url !== "/webhook/materials") {
      reply(404, "not found\n");
      return;
    }

    const chunks = [];
    let size = 0;
    let aborted = false;

    request.on("data", (chunk) => {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        aborted = true;
        reply(413, "payload too large\n");
        request.destroy();
        return;
      }
      chunks.push(chunk);
    });

    request.on("end", () => {
      if (aborted) return;
      const body = Buffer.concat(chunks);

      if (!signatureMatches(secret, body, request.headers["x-hub-signature-256"])) {
        // Do not distinguish a missing signature from a wrong one.
        reply(401, "signature mismatch\n");
        return;
      }

      let payload;
      try {
        payload = JSON.parse(body.toString("utf8"));
      } catch {
        reply(400, "invalid payload\n");
        return;
      }

      const event = request.headers["x-github-event"];
      if (event === "ping") {
        reply(200, "pong\n");
        return;
      }
      if (!isMirroredPush(event, payload, ref)) {
        // Accepted and ignored: GitHub should not retry a delivery we chose to skip.
        reply(202, "ignored\n");
        return;
      }

      // Answer before syncing: GitHub times deliveries out well before a mirror
      // of several hundred MB finishes.
      reply(202, "syncing\n");
      console.log(`push on ${ref} accepted, syncing materials`);
      void runSync();
    });
  });
}

function main() {
  if (Buffer.byteLength(SECRET) < 32) {
    console.error(
      "HENUKIT_MATERIALS_WEBHOOK_SECRET must be set and contain at least 32 bytes"
    );
    process.exit(64);
  }

  const server = createWebhookServer({ secret: SECRET, ref: REF, sync: SYNC });
  server.listen(PORT, ADDR, () => {
    console.log(`materials webhook listening on ${ADDR}:${PORT}, mirroring ${REF}`);
  });

  for (const signal of ["SIGTERM", "SIGINT"]) {
    process.on(signal, () => server.close(() => process.exit(0)));
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}
