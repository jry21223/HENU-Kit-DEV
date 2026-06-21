import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { createPackageOrder, listOrdersForUser } from "@/services/order-service";

type OrderBody = {
  packageId?: string;
  product_type?: "package";
  product_id?: string;
};

export async function GET() {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "Please log in first." }, { status: 401 });
  }

  const orders = await listOrdersForUser(user.id);
  return NextResponse.json({ orders });
}

export async function POST(request: Request) {
  const user = await getCurrentUser();

  if (!user || !user.emailVerified) {
    return NextResponse.json({ error: "Please log in with a verified student email." }, { status: 401 });
  }

  const body = (await request.json().catch(() => null)) as OrderBody | null;
  const packageId = body?.packageId ?? body?.product_id;

  if (!packageId) {
    return NextResponse.json({ error: "Missing packageId." }, { status: 400 });
  }

  if (body?.product_type && body.product_type !== "package") {
    return NextResponse.json({ error: "Only package orders are supported." }, { status: 400 });
  }

  const result = await createPackageOrder({
    userId: user.id,
    packageId,
  });

  if (result.kind === "not_found") {
    return NextResponse.json({ error: "Package not found or unpublished." }, { status: 404 });
  }

  if (result.kind === "already_owned") {
    return NextResponse.json({
      status: "already_owned",
      packageId: result.packageId,
    });
  }

  if (result.kind === "invalid_price") {
    return NextResponse.json({ error: "Package price must be greater than 0." }, { status: 400 });
  }

  return NextResponse.json({ order: result.order }, { status: 201 });
}
