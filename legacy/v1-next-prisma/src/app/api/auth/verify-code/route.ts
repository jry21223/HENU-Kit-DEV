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

const verificationFailure = "Verification code is invalid, expired, or already used.";

export async function POST(request: Request) {
  const body = (await request.json().catch(() => null)) as VerifyCodeBody | null;
  const email = normalizeEmail(body?.email ?? "");
  const code = (body?.code ?? "").trim();
  const schoolId = body?.school_id ?? "";
  const majorId = body?.major_id ?? "";
  const grade = body?.grade ?? "";

  if (!isAllowedStudentEmail(email)) {
    return NextResponse.json({ error: "Please use an allowed student email address." }, { status: 400 });
  }
  if (!/^\d{6}$/.test(code)) {
    return NextResponse.json({ error: "Verification code format is invalid." }, { status: 400 });
  }
  if (!schoolId || !majorId || !isValidGrade(grade)) {
    return NextResponse.json({ error: "Please select school, major, and grade." }, { status: 400 });
  }

  const emailDomain = email.split("@")[1];
  const [school, major] = await Promise.all([
    prisma.school.findFirst({
      where: {
        id: schoolId,
        emailDomains: { has: emailDomain },
        status: "PUBLISHED",
      },
    }),
    prisma.major.findFirst({
      where: {
        id: majorId,
        schoolId,
      },
    }),
  ]);

  if (!school || !major) {
    return NextResponse.json({ error: "School or major does not exist." }, { status: 400 });
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
    return NextResponse.json({ error: verificationFailure }, { status: 400 });
  }

  const user = await prisma.$transaction(async (tx) => {
    const consumed = await tx.emailVerification.updateMany({
      where: {
        id: verification.id,
        used: false,
        expiresAt: { gt: new Date() },
      },
      data: { used: true },
    });

    if (consumed.count !== 1) {
      return null;
    }

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

  if (!user) {
    return NextResponse.json({ error: verificationFailure }, { status: 400 });
  }

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
