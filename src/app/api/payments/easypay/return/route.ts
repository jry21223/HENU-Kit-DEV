import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json(
    { error: "EasyPay is deprecated. Use WeChat Pay Native instead." },
    { status: 410 },
  );
}
