import assert from "node:assert/strict";
import {
  createSessionToken,
  hashVerificationCode,
  isVerificationCodeUsable,
  parseSessionToken,
} from "../../src/lib/auth";
import {
  getEmailDomain,
  isAllowedStudentEmail,
  isValidEmail,
  isValidGrade,
  normalizeEmail,
} from "../../src/lib/validators";

process.env.AUTH_SECRET = "unit-test-secret";

assert.equal(normalizeEmail("  User@STU.HENU.EDU.CN "), "user@stu.henu.edu.cn");
assert.equal(isValidEmail("student@stu.henu.edu.cn"), true);
assert.equal(isValidEmail("not-an-email"), false);
assert.equal(getEmailDomain("student@stu.henu.edu.cn"), "stu.henu.edu.cn");

assert.equal(isAllowedStudentEmail("student@stu.henu.edu.cn"), true);
assert.equal(isAllowedStudentEmail("teacher@henu.edu.cn"), true);
assert.equal(isAllowedStudentEmail("user@qq.com"), false);
assert.equal(isAllowedStudentEmail("user@gmail.com"), false);
assert.equal(isAllowedStudentEmail("user@evil-stu.henu.edu.cn"), false);
assert.equal(isAllowedStudentEmail("user@stu.henu.edu.cn.evil.com"), false);

assert.equal(isValidGrade("2023级"), true);
assert.equal(isValidGrade("2024级"), true);
assert.equal(isValidGrade("23级"), false);

const hash = hashVerificationCode("student@stu.henu.edu.cn", "123456");
assert.notEqual(hash, "123456");
assert.equal(hash, hashVerificationCode(" student@STU.HENU.EDU.CN ", "123456"));
assert.notEqual(hash, hashVerificationCode("student@stu.henu.edu.cn", "654321"));

const now = new Date("2026-06-21T00:00:00.000Z");
assert.equal(
  isVerificationCodeUsable({ used: false, expiresAt: new Date("2026-06-21T00:01:00.000Z") }, now),
  true,
);
assert.equal(
  isVerificationCodeUsable({ used: true, expiresAt: new Date("2026-06-21T00:01:00.000Z") }, now),
  false,
);
assert.equal(
  isVerificationCodeUsable({ used: false, expiresAt: new Date("2026-06-20T23:59:59.000Z") }, now),
  false,
);

const token = createSessionToken({
  userId: "user-1",
  email: "student@stu.henu.edu.cn",
  role: "STUDENT",
  expiresAt: Date.now() + 60_000,
});
assert.equal(parseSessionToken(token)?.userId, "user-1");

const [payload, signature] = token.split(".");
const tamperedPayload = `${payload.slice(0, -1)}x.${signature}`;
assert.equal(parseSessionToken(tamperedPayload), null);

const tamperedSignature = `${payload}.${signature.slice(0, -1)}x`;
assert.equal(parseSessionToken(tamperedSignature), null);

const expiredToken = createSessionToken({
  userId: "user-1",
  email: "student@stu.henu.edu.cn",
  role: "STUDENT",
  expiresAt: Date.now() - 1_000,
});
assert.equal(parseSessionToken(expiredToken), null);

console.log("auth unit tests passed");
