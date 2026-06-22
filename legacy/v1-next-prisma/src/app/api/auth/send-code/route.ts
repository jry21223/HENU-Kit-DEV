import { NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { generateVerificationCode, sendVerificationEmail } from "@/lib/email";
import { hashVerificationCode } from "@/lib/auth";
import { isAllowedStudentEmail, normalizeEmail } from "@/lib/validators";

const CODE_TTL_MINUTES = 10;

export async function POST(request: Request) {
  const body = (await request.json().catch(() => null)) as { email?: string } | null;
  const email = normalizeEmail(body?.email ?? "");

  if (!isAllowedStudentEmail(email)) {
    return NextResponse.json(
      { error: "请使用河南大学学生邮箱登录。" },
      { status: 400 },
    );
  }

  const school = await prisma.school.findFirst({
    where: {
      emailDomains: { has: email.split("@")[1] },
      status: "PUBLISHED",
    },
    select: { id: true },
  });

  if (!school) {
    return NextResponse.json(
      { error: "当前邮箱域名未配置学校白名单。" },
      { status: 400 },
    );
  }

  const code = generateVerificationCode();
  const expiresAt = new Date(Date.now() + CODE_TTL_MINUTES * 60 * 1000);

  await prisma.emailVerification.create({
    data: {
      email,
      code: hashVerificationCode(email, code),
      expiresAt,
      used: false,
    },
  });

  await sendVerificationEmail(email, code);

  return NextResponse.json({
    ok: true,
    message: "验证码已发送。开发环境请查看服务端日志。",
    expiresInSeconds: CODE_TTL_MINUTES * 60,
  });
}

