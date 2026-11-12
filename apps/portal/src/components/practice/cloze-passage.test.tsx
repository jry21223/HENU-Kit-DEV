import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import ClozePassage, { ClozeEvidence } from "./cloze-passage";
import type { TrainingClozeUnit } from "@/lib/api/generated/training";

it("keeps surrounding passage and stable blank controls available without filling answers", () => {
  const unit = { parts: [{ part_id: "lead", kind: "text", text: "She asked for help " }, { part_id: "gap", kind: "blank", blank_id: "blank-help" }, { part_id: "tail", kind: "text", text: " and learned to repair it." }], blanks: [{ blank_id: "blank-help", question_id: "question-help" }], questions: [{ question_id: "question-help", options: ["before", "after"] }] } as TrainingClozeUnit;
  const html = renderToStaticMarkup(createElement(ClozePassage, { unit, activeQuestionID: "question-help", selected: {}, onSelect: () => {} }));
  expect(html).toContain("She asked for help ");
  expect(html).toContain(" and learned to repair it.");
  expect(html).toContain('id="cloze-blank-help"');
  expect(html).toContain('aria-label="第 1 空，未选择"');
  expect(html).not.toContain("before");
});

it("keeps evidence and passage blank numbers aligned when blank declarations are reordered", () => {
 const unit = { parts: [{ part_id: "gap-first", kind: "blank", blank_id: "first" }], blanks: [{blank_id:"second",question_id:"q2"},{blank_id:"first",question_id:"q1"}],questions:[{question_id:"q1"},{question_id:"q2"}] } as TrainingClozeUnit;
 const html=renderToStaticMarkup(createElement(ClozeEvidence,{unit,partIDs:["gap-first"]}));
 expect(html).toContain("[第 1 空]");
 expect(html).not.toContain("[第 2 空]");
});
