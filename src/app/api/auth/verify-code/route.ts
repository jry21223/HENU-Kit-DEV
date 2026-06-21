import { NextResponse } from "next/server";
import { prisma } from "@/lib/db";
import { hashVerificationCode, setSessionCookie } from "@/lib/auth";
import { isAllowedStudentEmail, isValidGrade, normalizeEmail } from "@/lib/validators";

type VerifyCodeBody = {
  email?: string;
  code?: string;
  school_id?: string;
  major_id?: string;
  grade?: string;
};

export async function POST(request: Request) {
  const body = (await request.json().catch(() => null)) as VerifyCodeBody | null;
  const email = normalizeEmail(body?.email ?? "");
  const code = (body?.code ?? "").trim();
  const schoolId = body?.school_id ?? "";
  const majorId = body?.major_id ?? "";
  const grade = body?.grade ?? "";

  if (!isAllowedStudentEmail(email)) {
    return NextResponse.json({ error: "请使用河南大学学生邮箱登录。" }, { status: 400 });
  }
  if (!/^\d{6}$/.test(code)) {
    return NextResponse.json({ error: "验证码格式不正确。" }, { status: 400 });
  }
  if (!schoolId || !majorId || !isValidGrade(grade)) {
    return NextResponse.json({ error: "请选择学校、专业和年级。" }, { status: 400 });
  }

  const school = await prisma.school.findFirst({
    where: {
      id: schoolId,
      emailDomains: { has: email.split("@")[1] },
      status: "PUBLISHED",
    },
  });

  const major = await prisma.major.findFirst({
    where: {
      id: majorId,
      schoolId,
    },
  });

  if (!school || !major) {
    return NextResponse.json({ error: "学校或专业不存在。" }, { status: 400 });
  }

  const hashedCode = hashVerificationCode(email, code);
  const verification = await prisma.emailVerification.findFirst({
    where: {
      email,
      code: hashedCode,
      used: false,
      expiresAt: { gt: new Date() },
    },
    orderBy: { createdAt: "desc" },
  });

  if (!verification) {
    return NextResponse.json({ error: "验证码错误、已过期或已使用。" }, { status: 400 });
  }

  const user = await prisma.$transaction(async (tx) => {
    await tx.emailVerification.update({
      where: { id: verification.id },
      data: { used: true },
    });

    return tx.user.upsert({
      where: { email },
      update: {
        schoolId,
        majorId,
        grade,
        emailVerified: true,
      },
      create: {
        email,
        name: email.split("@")[0],
        schoolId,
        majorId,
        grade,
        emailVerified: true,
        role: "STUDENT",
      },
      select: {
        id: true,
        email: true,
        role: true,
      },
    });
  });

  await setSessionCookie(user);

  return NextResponse.json({
    ok: true,
    user: {
      id: user.id,
      email: user.email,
      role: user.role.toLowerCase(),
    },
  });
}

