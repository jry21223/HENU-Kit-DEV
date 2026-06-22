import assert from "node:assert/strict";
import { AiJobStatus, MaterialType } from "@prisma/client";
import {
  buildAiDraftContent,
  getAiOutputTypeLabel,
  mapAiJobStatus,
  mapAiOutputType,
  parseAiJobStatus,
  parseAiOutputType,
} from "../../src/lib/ai-jobs";

assert.equal(parseAiOutputType("knowledge_note"), MaterialType.KNOWLEDGE_NOTE);
assert.equal(parseAiOutputType("mock_paper"), MaterialType.MOCK_PAPER);
assert.equal(parseAiOutputType("invalid"), null);

assert.equal(mapAiOutputType(MaterialType.ANSWER), "answer");
assert.equal(getAiOutputTypeLabel(MaterialType.QUICK_REVIEW), "考前速背版");

assert.equal(parseAiJobStatus("succeeded"), AiJobStatus.SUCCEEDED);
assert.equal(parseAiJobStatus("failed"), AiJobStatus.FAILED);
assert.equal(parseAiJobStatus("unknown"), undefined);
assert.equal(mapAiJobStatus(AiJobStatus.QUEUED), "queued");

const draft = buildAiDraftContent({
  courseName: "离散数学",
  outputType: MaterialType.KNOWLEDGE_NOTE,
  sourceTitles: ["命题逻辑讲义", "图论模拟卷"],
});

assert.equal(draft.title, "离散数学知识点讲义草稿");
assert.equal(draft.description.includes("需人工审核后才能发布"), true);
assert.equal(draft.previewContent.includes("命题逻辑讲义、图论模拟卷"), true);
assert.equal(draft.previewContent.includes("确认无误后再由管理员发布"), true);

console.log("ai job unit tests passed");
