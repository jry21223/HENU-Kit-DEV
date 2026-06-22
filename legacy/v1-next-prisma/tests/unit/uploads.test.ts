import assert from "node:assert/strict";
import {
  MAX_UPLOAD_FILE_SIZE,
  getUploadFileExtension,
  sanitizeFileName,
  validateUploadFile,
} from "../../src/lib/uploads";

assert.equal(sanitizeFileName("discrete math notes.pdf"), "discrete-math-notes.pdf");
assert.equal(sanitizeFileName("../../../bad.exe"), "bad.exe");
assert.equal(sanitizeFileName("..\\..\\bad.pdf"), "bad.pdf");
assert.equal(sanitizeFileName("   "), "upload");

assert.equal(getUploadFileExtension("NOTE.PDF"), ".pdf");
assert.equal(getUploadFileExtension("archive.tar.gz"), ".gz");

assert.deepEqual(
  validateUploadFile({ name: "note.pdf", type: "application/pdf", size: 1024 }),
  { ok: true },
);

assert.deepEqual(
  validateUploadFile({ name: "note.txt", type: "text/plain", size: 1024 }),
  { ok: true },
);

assert.equal(
  validateUploadFile({ name: "script.js", type: "application/javascript", size: 1024 }).ok,
  false,
);

assert.equal(
  validateUploadFile({ name: "script.exe", type: "application/pdf", size: 1024 }).ok,
  false,
);

assert.equal(
  validateUploadFile({ name: "note.pdf", type: "text/plain", size: 1024 }).ok,
  false,
);

assert.equal(
  validateUploadFile({ name: "note.txt", type: "application/pdf", size: 1024 }).ok,
  false,
);

assert.equal(validateUploadFile({ name: "empty.pdf", type: "application/pdf", size: 0 }).ok, false);
assert.equal(
  validateUploadFile({
    name: "huge.pdf",
    type: "application/pdf",
    size: MAX_UPLOAD_FILE_SIZE + 1,
  }).ok,
  false,
);

console.log("upload unit tests passed");
