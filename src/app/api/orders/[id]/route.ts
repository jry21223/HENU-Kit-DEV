import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { getOrderForUser } from "@/services/order-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const { id } = await context.params;
  const order = await getOrderForUser(id, user.id);

  if (!order) {
    return NextResponse.json({ error: "订单不存在。" }, { status: 404 });
  }

  return NextResponse.json({ order });
}
