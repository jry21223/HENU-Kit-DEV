import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { createPackageOrder, listOrdersForUser } from "@/services/order-service";

type OrderBody = {
  product_type?: "package";
  product_id?: string;
  payment_type?: string;
};

export async function GET() {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const orders = await listOrdersForUser(user.id);
  return NextResponse.json({ orders });
}

export async function POST(request: Request) {
  const user = await getCurrentUser();

  if (!user || !user.emailVerified) {
    return NextResponse.json({ error: "请先使用学生邮箱登录。" }, { status: 401 });
  }

  const body = (await request.json().catch(() => null)) as OrderBody | null;
  if (!body?.product_type || !body.product_id) {
    return NextResponse.json({ error: "缺少订单商品信息。" }, { status: 400 });
  }
  if (body.product_type !== "package") {
    return NextResponse.json({ error: "当前阶段只支持购买课程复习包。" }, { status: 400 });
  }

  const result = await createPackageOrder({
    userId: user.id,
    packageId: body.product_id,
    paymentType: body.payment_type,
  });

  if (!result) {
    return NextResponse.json({ error: "课程包不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json(result, { status: 201 });
}
