import { NextResponse } from "next/server";
import { getEasypayConfig, validateEasypaySignedParams, type EasypayParams } from "@/lib/easypay";
import { settleEasypayOrder } from "@/services/order-service";

async function collectParams(request: Request): Promise<EasypayParams> {
  const url = new URL(request.url);
  const params: EasypayParams = Object.fromEntries(url.searchParams.entries());
  const contentType = request.headers.get("content-type") ?? "";

  if (contentType.includes("application/json")) {
    const body = (await request.json().catch(() => null)) as EasypayParams | null;
    return { ...params, ...(body ?? {}) };
  }

  if (contentType.includes("application/x-www-form-urlencoded") || contentType.includes("multipart/form-data")) {
    const formData = await request.formData().catch(() => null);
    if (formData) {
      for (const [key, value] of formData.entries()) {
        params[key] = typeof value === "string" ? value : value.name;
      }
    }
  }

  return params;
}

async function handleNotify(request: Request) {
  const params = await collectParams(request);
  const config = getEasypayConfig();

  if (!validateEasypaySignedParams(params, config)) {
    return new NextResponse("fail", { status: 400 });
  }

  const result = await settleEasypayOrder(params);
  if (!result.ok) {
    return new NextResponse("fail", { status: result.status });
  }

  return new NextResponse("success", { status: 200 });
}

export async function POST(request: Request) {
  return handleNotify(request);
}

export async function GET(request: Request) {
  return handleNotify(request);
}
