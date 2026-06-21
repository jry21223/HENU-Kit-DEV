import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { getOrderStatusForViewer } from "@/services/order-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "Please log in first." }, { status: 401 });
  }

  const { id } = await context.params;
  const status = await getOrderStatusForViewer(id, { id: user.id, role: user.role });

  if (!status) {
    return NextResponse.json({ error: "Order not found." }, { status: 404 });
  }

  return NextResponse.json(status);
}
