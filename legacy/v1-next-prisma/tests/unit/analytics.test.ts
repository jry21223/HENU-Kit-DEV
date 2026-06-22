import assert from "node:assert/strict";
import {
  topCourseDownloads,
  topMaterialDownloads,
  topWeakPoints,
} from "../../src/lib/analytics";

assert.deepEqual(
  topCourseDownloads([
    { courseId: "course-a", courseName: "离散数学" },
    { courseId: "course-a", courseName: "离散数学" },
    { courseId: "course-b", courseName: "大学物理" },
  ]),
  [
    { courseId: "course-a", courseName: "离散数学", count: 2 },
    { courseId: "course-b", courseName: "大学物理", count: 1 },
  ],
);

assert.deepEqual(
  topMaterialDownloads(
    [
      { materialId: "m1", materialTitle: "讲义", courseName: "离散数学" },
      { materialId: "m2", materialTitle: "模拟卷", courseName: "离散数学" },
      { materialId: "m1", materialTitle: "讲义", courseName: "离散数学" },
    ],
    1,
  ),
  [{ materialId: "m1", materialTitle: "讲义", courseName: "离散数学", count: 2 }],
);

assert.deepEqual(
  topWeakPoints([
    {
      knowledgePointId: "kp-1",
      knowledgePointTitle: "命题逻辑",
      courseId: "course-a",
      courseName: "离散数学",
    },
    {
      knowledgePointId: null,
      knowledgePointTitle: null,
      courseId: "course-b",
      courseName: "大学物理",
    },
  ]),
  [
    {
      knowledgePointId: "kp-1",
      knowledgePointTitle: "命题逻辑",
      courseId: "course-a",
      courseName: "离散数学",
      count: 1,
    },
    {
      knowledgePointId: "course:course-b",
      knowledgePointTitle: "未关联知识点",
      courseId: "course-b",
      courseName: "大学物理",
      count: 1,
    },
  ],
);

console.log("analytics unit tests passed");
