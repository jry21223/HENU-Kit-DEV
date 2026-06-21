import assert from "node:assert/strict";
import path from "node:path";
import { shouldUseMockData } from "../../src/lib/db";
import { getDownloadContentType, resolveLocalUploadPath } from "../../src/lib/downloads";
import { canDownloadMaterial } from "../../src/lib/permissions";

const verifiedUser = { id: "user-1", emailVerified: true };
const unverifiedUser = { id: "user-2", emailVerified: false };

assert.deepEqual(
  canDownloadMaterial({ status: "PUBLISHED", accessLevel: "FREE" }, null),
  { allowed: true },
);

assert.equal(
  canDownloadMaterial({ status: "PUBLISHED", accessLevel: "LOGIN_REQUIRED" }, null).allowed,
  false,
);

assert.equal(
  canDownloadMaterial(
    { status: "PUBLISHED", accessLevel: "LOGIN_REQUIRED" },
    unverifiedUser,
  ).allowed,
  false,
);

assert.deepEqual(
  canDownloadMaterial({ status: "PUBLISHED", accessLevel: "LOGIN_REQUIRED" }, verifiedUser),
  { allowed: true },
);

const paidAnonymousResult = canDownloadMaterial({ status: "PUBLISHED", accessLevel: "PAID" }, null);
assert.equal(paidAnonymousResult.allowed, false);
assert.equal(paidAnonymousResult.allowed === false ? paidAnonymousResult.status : 0, 401);

const paidWithoutEntitlement = canDownloadMaterial(
  { status: "PUBLISHED", accessLevel: "PAID" },
  verifiedUser,
);
assert.equal(paidWithoutEntitlement.allowed, false);
assert.equal(paidWithoutEntitlement.allowed === false ? paidWithoutEntitlement.status : 0, 402);

assert.deepEqual(
  canDownloadMaterial({ status: "PUBLISHED", accessLevel: "PAID" }, verifiedUser, true),
  { allowed: true },
);

for (const status of ["DRAFT", "PENDING_REVIEW", "ARCHIVED"] as const) {
  const result = canDownloadMaterial({ status, accessLevel: "FREE" }, verifiedUser);
  assert.equal(result.allowed, false);
  assert.equal(result.allowed === false ? result.status : 0, 404);
}

const uploadRoot = path.resolve("C:/project/uploads");
assert.equal(
  resolveLocalUploadPath("/uploads/mock/note.pdf", uploadRoot),
  path.resolve(uploadRoot, "mock/note.pdf"),
);
assert.equal(resolveLocalUploadPath("https://example.com/note.pdf", uploadRoot), null);
assert.equal(resolveLocalUploadPath("/uploads/../.env", uploadRoot), null);
assert.equal(resolveLocalUploadPath("/uploads/../uploads_evil/secret.pdf", uploadRoot), null);
assert.equal(resolveLocalUploadPath("/uploads\\..\\.env", uploadRoot), null);

assert.equal(getDownloadContentType("note.pdf"), "application/pdf");
assert.equal(getDownloadContentType("note.TXT"), "text/plain; charset=utf-8");
assert.equal(getDownloadContentType("note.bin"), "application/octet-stream");

const originalNodeEnv = process.env.NODE_ENV;
const originalDatabaseUrl = process.env.DATABASE_URL;

function setNodeEnv(value: string | undefined) {
  if (value === undefined) {
    delete (process.env as Record<string, string | undefined>).NODE_ENV;
    return;
  }
  (process.env as Record<string, string | undefined>).NODE_ENV = value;
}

delete process.env.DATABASE_URL;
setNodeEnv("development");
assert.equal(shouldUseMockData(), true);
setNodeEnv("test");
assert.equal(shouldUseMockData(), true);
setNodeEnv("production");
assert.equal(shouldUseMockData(), false);
process.env.DATABASE_URL = "postgresql://example";
assert.equal(shouldUseMockData(), false);
setNodeEnv(originalNodeEnv);
if (originalDatabaseUrl === undefined) {
  delete process.env.DATABASE_URL;
} else {
  process.env.DATABASE_URL = originalDatabaseUrl;
}

console.log("permission unit tests passed");
