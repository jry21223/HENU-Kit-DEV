import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { grantEntitlement } from "@/services/package-service";

type EntitlementBody = {
  user_id?: string;
  email?: string;
  resource_type?: "package" | "material";
  resource_id?: string;
  source?: string;
  expires_at?: string | null;
};

export async function POST(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const body = (await request.json().catch(() => null)) as EntitlementBody | null;
  if (!body?.resource_type || !body.resource_id || (!body.user_id && !body.email)) {
    return NextResponse.json({ error: "缺少授权必填字段。" }, { status: 400 });
  }
  if (!["package", "material"].includes(body.resource_type)) {
    return NextResponse.json({ error: "授权资源类型无效。" }, { status: 400 });
  }

  const expiresAt = body.expires_at ? new Date(body.expires_at) : null;
  if (body.expires_at && Number.isNaN(expiresAt?.getTime())) {
    return NextResponse.json({ error: "过期时间无效。" }, { status: 400 });
  }

  const entitlement = await grantEntitlement({
    userId: body.user_id,
    email: body.email,
    resourceType: body.resource_type,
    resourceId: body.resource_id,
    source: body.source ?? "manual",
    expiresAt,
  });

  if (!entitlement) {
    return NextResponse.json({ error: "用户不存在。" }, { status: 404 });
  }

  return NextResponse.json({
    entitlement: {
      id: entitlement.id,
      user_id: entitlement.userId,
      resource_type: entitlement.resourceType,
      resource_id: entitlement.resourceId,
      source: entitlement.source,
      expires_at: entitlement.expiresAt,
    },
  }, { status: 201 });
}
