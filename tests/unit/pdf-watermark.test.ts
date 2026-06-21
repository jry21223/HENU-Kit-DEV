import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { PDFDocument } from "pdf-lib";
import {
  addPdfWatermark,
  buildWatermarkLines,
  isPdfFileName,
} from "../../src/lib/pdf-watermark";

assert.equal(isPdfFileName("note.pdf"), true);
assert.equal(isPdfFileName("note.PDF"), true);
assert.equal(isPdfFileName("note.txt"), false);

const fixedDate = new Date("2026-06-19T12:00:00.000Z");
assert.deepEqual(buildWatermarkLines({ userEmail: "student@stu.henu.edu.cn", downloadedAt: fixedDate }), [
  "Jerry 制作｜仅供个人复习使用｜禁止盗印与商用传播",
  "用户：student@stu.henu.edu.cn｜下载时间：2026-06-19T12:00:00.000Z",
]);

async function main() {
  const original = await readFile("uploads/mock/discrete-math-sample.pdf");
  const originalCopy = Buffer.from(original);
  const watermarked = await addPdfWatermark(original, {
    userEmail: "student@stu.henu.edu.cn",
    downloadedAt: fixedDate,
  });

  assert.equal(original.equals(originalCopy), true);
  assert.equal(watermarked.subarray(0, 5).toString("utf8"), "%PDF-");
  assert.equal(watermarked.length > original.length, true);

  const originalDoc = await PDFDocument.load(original);
  const watermarkedDoc = await PDFDocument.load(watermarked);
  assert.equal(watermarkedDoc.getPageCount(), originalDoc.getPageCount());

  console.log("pdf watermark unit tests passed");
}

void main();
