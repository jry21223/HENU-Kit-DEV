import assert from "node:assert/strict";
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

const paidResult = canDownloadMaterial({ status: "PUBLISHED", accessLevel: "PAID" }, verifiedUser);
assert.equal(paidResult.allowed, false);
assert.equal(paidResult.allowed === false ? paidResult.status : 0, 402);

assert.deepEqual(
  canDownloadMaterial({ status: "PUBLISHED", accessLevel: "PAID" }, verifiedUser, true),
  { allowed: true },
);

const draftResult = canDownloadMaterial(
  { status: "DRAFT", accessLevel: "FREE" },
  verifiedUser,
);
assert.equal(draftResult.allowed, false);
assert.equal(draftResult.allowed === false ? draftResult.status : 0, 404);

console.log("permission unit tests passed");
