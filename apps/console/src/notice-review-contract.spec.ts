/// <reference types="vite/client" />
import generated from "./lib/console-gateway.ts?raw";
import { describe, expect, it } from "vitest";

// Validate the actual generated contract artifact, including its oneOf. This
// private predicate is currently unused by writes; this is not authorization.
const recordBody = generated.match(/function isRecord\(value: unknown\): value is Record<string, unknown> \{([\s\S]*?)\n\}/)?.[1];
const reviewBody = generated.match(/function isNoticeReviewRequest\(value: unknown\): value is NoticeReviewRequest \{([\s\S]*?)\n\}/)?.[1];
if (!recordBody || !reviewBody) throw new Error("Generated Notice review contract predicates are missing");
const isRecord = new Function("value", recordBody);
const checkReview = new Function("value", "isRecord", reviewBody);

describe("generated Notice review contract", () => {
  it.each([
    { input: { decision: "approved", expected_revision: 1 }, allowed: true },
    { input: { decision: "approved", note: "已核对来源", expected_revision: 1 }, allowed: true },
    { input: { decision: "rejected", expected_revision: 1 }, allowed: false },
    { input: { decision: "rejected", note: " \t ", expected_revision: 1 }, allowed: false },
    { input: { decision: "rejected", note: "与原文不符", expected_revision: 1 }, allowed: true },
  ])("classifies $input according to the review contract", ({ input, allowed }) => {
    expect(checkReview(input, isRecord)).toBe(allowed);
  });
});
