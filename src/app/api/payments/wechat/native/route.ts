import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { WechatPayConfigError } from "@/lib/payments/wechat/config";
import { WechatPayLiveNotImplementedError } from "@/lib/payments/wechat/native";
import { startWechatNativePayment } from "@/services/order-service";

type NativePaymentBody = {
  orderId?: string;
};

export async function POST(request: Request) {
  const user = await getCurrentUser();

  if (!user || !user.emailVerified) {
    return NextResponse.json({ error: "Please log in with a verified student email." }, { status: 401 });
  }

  const body = (await request.json().catch(() => null)) as NativePaymentBody | null;
  if (!body?.orderId) {
    return NextResponse.json({ error: "Missing orderId." }, { status: 400 });
  }

  try {
    const result = await startWechatNativePayment({ orderId: body.orderId, userId: user.id });

    if (!result.ok) {
      return NextResponse.json({ error: result.error }, { status: result.status });
    }

    return NextResponse.json({
      orderId: result.orderId,
      codeUrl: result.codeUrl,
      expiresAt: result.expiresAt,
      status: result.status,
    });
  } catch (error) {
    if (error instanceof WechatPayConfigError) {
      return NextResponse.json(
        { error: "WeChat Pay is not configured correctly.", details: error.errors },
        { status: 500 },
      );
    }

    if (error instanceof WechatPayLiveNotImplementedError) {
      return NextResponse.json({ error: error.message }, { status: 501 });
    }

    throw error;
  }
}
