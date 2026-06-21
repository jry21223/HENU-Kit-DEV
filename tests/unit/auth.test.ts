import assert from "node:assert/strict";
import {
  createSessionToken,
  hashVerificationCode,
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

assert.equal(isValidGrade("2023级"), true);
assert.equal(isValidGrade("2024级"), true);
assert.equal(isValidGrade("23级"), false);

const hash = hashVerificationCode("student@stu.henu.edu.cn", "123456");
assert.notEqual(hash, "123456");
assert.equal(hash, hashVerificationCode(" student@STU.HENU.EDU.CN ", "123456"));
assert.notEqual(hash, hashVerificationCode("student@stu.henu.edu.cn", "654321"));

const token = createSessionToken({
  userId: "user-1",
  email: "student@stu.henu.edu.cn",
  role: "STUDENT",
  expiresAt: Date.now() + 60_000,
});
assert.equal(parseSessionToken(token)?.userId, "user-1");

const [payload, signature] = token.split(".");
const tampered = `${payload.slice(0, -1)}x.${signature}`;
assert.equal(parseSessionToken(tampered), null);

const expiredToken = createSessionToken({
  userId: "user-1",
  email: "student@stu.henu.edu.cn",
  role: "STUDENT",
  expiresAt: Date.now() - 1_000,
});
assert.equal(parseSessionToken(expiredToken), null);

console.log("auth unit tests passed");
