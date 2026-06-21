import { NextResponse } from "next/server";
import { getEasypayConfig, verifyEasypaySignature, type EasypayParams } from "@/lib/easypay";
import { settleEasypayOrder } from "@/services/order-service";

export async function GET(request: Request) {
  const url = new URL(request.url);
  const params: EasypayParams = Object.fromEntries(url.searchParams.entries());
  const config = getEasypayConfig();
  const orderId = String(params.out_trade_no ?? "");

  if (!config.pid || !config.key || !verifyEasypaySignature(params, config.key)) {
    return NextResponse.redirect(`${config.appUrl}/me/orders?payment=invalid`);
  }

  if (String(params.trade_status ?? "") === "TRADE_SUCCESS") {
    await settleEasypayOrder(params);
    return NextResponse.redirect(`${config.appUrl}/me/orders?payment=success&orderId=${orderId}`);
  }

  return NextResponse.redirect(`${config.appUrl}/me/orders?payment=failed&orderId=${orderId}`);
}
