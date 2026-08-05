import assert from "node:assert/strict";
import test from "node:test";

import {
  buildSQL,
  isPublishable,
  materialTypeForRole,
  planCatalogue,
  stableUUID,
} from "../index-henukit-materials.mjs";

const MANIFEST = {
  subjects: [
    {
      name: "高等数学A（二）",
      note: "收录课件与真题。",
      assets: [
        {
          role: "复习讲义",
          title: "考前复习知识点讲义.pdf",
          publicPath: "高等数学A（二）/复习讲义/考前复习知识点讲义.pdf",
          bytes: 237837,
        },
        {
          role: "课件PPT",
          title: "D10-1二重积分概念.ppt",
          publicPath: "高等数学A（二）/课件PPT/D10-1二重积分概念.ppt",
          bytes: 2318336,
        },
        {
          role: "待复核资料",
          title: "第八章自测.docx",
          publicPath: "高等数学A（二）/待复核资料/第八章自测.docx",
          bytes: 100,
        },
      ],
    },
    {
      // Every asset pending review means the subject publishes nothing at all.
      name: "全部待复核",
      assets: [
        {
          role: "待复核课件PPT",
          title: "x.ppt",
          publicPath: "全部待复核/待复核课件PPT/x.ppt",
          bytes: 1,
        },
      ],
    },
  ],
};

test("identifiers are stable and well-formed", () => {
  const first = stableUUID("henukit.course", "高等数学A（二）");
  assert.equal(first, stableUUID("henukit.course", "高等数学A（二）"));
  assert.notEqual(first, stableUUID("henukit.course", "大学物理"));
  assert.notEqual(first, stableUUID("henukit.material", "高等数学A（二）"));
  assert.match(
    first,
    /^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
  );
});

test("pending-review assets are never publishable", () => {
  assert.equal(isPublishable({ role: "课件PPT" }), true);
  assert.equal(isPublishable({ role: "往年真题" }), true);
  assert.equal(isPublishable({ role: "待复核资料" }), false);
  assert.equal(isPublishable({ role: "待复核课件PPT" }), false);
});

test("roles map onto catalogue types, courseware included", () => {
  assert.equal(materialTypeForRole("往年真题"), "past_exam");
  assert.equal(materialTypeForRole("题库练习"), "mock_paper");
  assert.equal(materialTypeForRole("课件PPT"), "courseware");
  assert.equal(materialTypeForRole("课件资料"), "courseware");
  assert.equal(materialTypeForRole("复习讲义"), "knowledge_note");
  assert.equal(materialTypeForRole("笔记总结"), "knowledge_note");
});

test("the plan carries only publishable assets and keeps repo-relative keys", () => {
  const { courses, materials } = planCatalogue(MANIFEST);

  // The all-pending subject contributes no course at all.
  assert.equal(courses.length, 1);
  assert.equal(courses[0].name, "高等数学A（二）");

  assert.equal(materials.length, 2);
  assert.ok(!materials.some((m) => m.storageKey.includes("待复核")));

  const lecture = materials.find((m) => m.type === "courseware");
  assert.equal(
    lecture.storageKey,
    "高等数学A（二）/课件PPT/D10-1二重积分概念.ppt"
  );
  assert.equal(lecture.fileName, "D10-1二重积分概念.ppt");
  assert.equal(lecture.fileSize, 2318336);
  assert.equal(lecture.courseId, courses[0].id);
});

test("re-planning the same manifest yields the same rows", () => {
  assert.deepEqual(planCatalogue(MANIFEST), planCatalogue(MANIFEST));
});

test("SQL upserts, quotes hostile text, and withdraws what left the manifest", () => {
  const sql = buildSQL(planCatalogue(MANIFEST));

  assert.ok(sql.startsWith("BEGIN;"));
  assert.ok(sql.trimEnd().endsWith("COMMIT;"));
  assert.ok(sql.includes("ON CONFLICT (id) DO UPDATE"));
  assert.ok(sql.includes("status = 'withdrawn'"));
  // Withdrawal must not touch materials hosted outside the mirror.
  assert.ok(sql.includes("storage_key NOT LIKE '/%'"));
  assert.ok(sql.includes("storage_key NOT LIKE 'http%'"));
  assert.ok(!sql.includes("待复核"));

  const hostile = buildSQL(
    planCatalogue({
      subjects: [
        {
          name: "it's a course",
          assets: [
            {
              role: "课件PPT",
              title: "it's a title",
              publicPath: "it's/a/path.ppt",
              bytes: 1,
            },
          ],
        },
      ],
    })
  );
  assert.ok(hostile.includes("'it''s a course'"));
  assert.ok(hostile.includes("'it''s a title'"));
  assert.ok(hostile.includes("'it''s/a/path.ppt'"));
});
