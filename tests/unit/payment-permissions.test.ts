import assert from "node:assert/strict";
import { canDownloadMaterial } from "../../src/lib/permissions";
import { centsToYuanString, yuanToCents } from "../../src/lib/payments/money";
import {
  canStartWechatNativePayment,
  shouldCreatePackageOrder,
} from "../../src/lib/payments/order-permissions";

assert.equal(yuanToCents("19.90"), 1990);
assert.equal(yuanToCents(0.1 + 0.2), 30);
assert.equal(yuanToCents("-1"), null);
assert.equal(centsToYuanString(1990), "19.90");
assert.equal(centsToYuanString(-1), "0.00");

assert.equal(shouldCreatePackageOrder(false), true);
assert.equal(shouldCreatePackageOrder(true), false);

assert.deepEqual(
  canStartWechatNativePayment({ userId: "user-1", status: "PENDING", amountTotal: 1990 }, "user-1"),
  { allowed: true },
);
assert.deepEqual(
  canStartWechatNativePayment({ userId: "user-1", status: "PAYING", amountTotal: 1990 }, "user-1"),
  { allowed: true },
);

const otherUserResult = canStartWechatNativePayment(
  { userId: "user-1", status: "PENDING", amountTotal: 1990 },
  "user-2",
);
assert.equal(otherUserResult.allowed, false);
assert.equal(otherUserResult.allowed === false ? otherUserResult.status : 0, 404);

const paidOrderResult = canStartWechatNativePayment(
  { userId: "user-1", status: "PAID", amountTotal: 1990 },
  "user-1",
);
assert.equal(paidOrderResult.allowed, false);
assert.equal(paidOrderResult.allowed === false ? paidOrderResult.status : 0, 409);

const zeroAmountResult = canStartWechatNativePayment(
  { userId: "user-1", status: "PENDING", amountTotal: 0 },
  "user-1",
);
assert.equal(zeroAmountResult.allowed, false);
assert.equal(zeroAmountResult.allowed === false ? zeroAmountResult.status : 0, 400);

const verifiedUser = { id: "user-1", emailVerified: true };
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

console.log("payment permission unit tests passed");
