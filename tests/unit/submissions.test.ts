import assert from "node:assert/strict";
import { SubmissionStatus } from "@prisma/client";
import {
  canReviewSubmissions,
  canTransitionSubmission,
  mapSubmissionStatus,
  parseReviewAction,
  parseSubmissionStatus,
  validateReviewComment,
} from "../../src/lib/submissions";

assert.equal(canReviewSubmissions("ADMIN"), true);
assert.equal(canReviewSubmissions("REVIEWER"), true);
assert.equal(canReviewSubmissions("STUDENT"), false);
assert.equal(canReviewSubmissions(undefined), false);

assert.equal(mapSubmissionStatus(SubmissionStatus.PENDING), "pending");
assert.equal(parseSubmissionStatus("approved"), SubmissionStatus.APPROVED);
assert.equal(parseSubmissionStatus("unknown"), undefined);

assert.equal(parseReviewAction("approve"), "approve");
assert.equal(parseReviewAction("reject"), "reject");
assert.equal(parseReviewAction("publish"), null);

assert.equal(canTransitionSubmission(SubmissionStatus.PENDING, "approve"), true);
assert.equal(canTransitionSubmission(SubmissionStatus.PENDING, "reject"), true);
assert.equal(canTransitionSubmission(SubmissionStatus.APPROVED, "reject"), false);

assert.deepEqual(validateReviewComment("approve", ""), { ok: true });
assert.equal(validateReviewComment("reject", "").ok, false);
assert.equal(validateReviewComment("reject", "资料来源不清楚").ok, true);
assert.equal(validateReviewComment("approve", "x".repeat(501)).ok, false);

console.log("submission unit tests passed");
