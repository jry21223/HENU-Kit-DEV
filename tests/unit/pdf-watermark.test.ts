import assert from "node:assert/strict";
import { PDFDocument } from "pdf-lib";
import {
  addPdfWatermark,
  buildWatermarkLines,
  isPdfFileName,
} from "../../src/lib/pdf-watermark";

assert.equal(isPdfFileName("note.pdf"), true);
assert.equal(isPdfFileName("note.PDF"), true);
assert.equal(isPdfFileName("note.txt"), false);
assert.equal(isPdfFileName("../note.pdf"), true);

const fixedDate = new Date("2026-06-19T12:00:00.000Z");
const lines = buildWatermarkLines({
  notice: "Unit test notice",
  userEmail: "student@stu.henu.edu.cn",
  downloadedAt: fixedDate,
});
assert.equal(lines[0], "Unit test notice");
assert.equal(lines[1].includes("student@stu.henu.edu.cn"), true);
assert.equal(lines[1].includes("2026-06-19T12:00:00.000Z"), true);

async function createPdf() {
  const pdf = await PDFDocument.create();
  const page = pdf.addPage([320, 240]);
  page.drawText("Original review material", { x: 32, y: 180, size: 14 });
  return Buffer.from(await pdf.save());
}

async function main() {
  const original = await createPdf();
  const originalCopy = Buffer.from(original);
  const watermarked = await addPdfWatermark(original, {
    notice: "Unit test notice",
    userEmail: "student@stu.henu.edu.cn",
    downloadedAt: fixedDate,
  });

  assert.equal(original.equals(originalCopy), true);
  assert.equal(watermarked.subarray(0, 5).toString("utf8"), "%PDF-");
  assert.notEqual(Buffer.compare(watermarked, original), 0);

  const originalDoc = await PDFDocument.load(original);
  const watermarkedDoc = await PDFDocument.load(watermarked);
  assert.equal(watermarkedDoc.getPageCount(), originalDoc.getPageCount());

  console.log("pdf watermark unit tests passed");
}

void main();
