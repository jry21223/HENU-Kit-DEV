import { OrderStatus, RecordStatus, UserRole } from "@prisma/client";
import { randomBytes } from "node:crypto";
import { prisma } from "@/lib/db";
import { centsToYuanString, yuanToCents } from "@/lib/payments/money";
import { canStartWechatNativePayment } from "@/lib/payments/order-permissions";
import { createWechatNativePayment } from "@/lib/payments/wechat/native";

export type PublicOrder = {
  id: string;
  userId: string;
  amount: string;
  amountTotal: number;
  currency: string;
  paymentProvider: string;
  outTradeNo?: string;
  status: "pending" | "paying" | "paid" | "failed" | "closed" | "expired" | "cancelled" | "refunded";
  productType: string;
  productId: string;
  productTitle: string;
  createdAt: string;
  expiresAt?: string;
  paidAt?: string;
};

const orderStatusMap: Record<OrderStatus, PublicOrder["status"]> = {
  PENDING: "pending",
  PAYING: "paying",
  PAID: "paid",
  FAILED: "failed",
  CLOSED: "closed",
  EXPIRED: "expired",
  CANCELLED: "cancelled",
  REFUNDED: "refunded",
};

type OrderLike = {
  id: string;
  userId: string;
  amount: unknown;
  amountTotal: number;
  currency: string;
  paymentProvider: string;
  outTradeNo: string | null;
  status: OrderStatus;
  productType: string;
  productId: string;
  createdAt: Date;
  expiresAt: Date | null;
  paidAt: Date | null;
};

function toPublicOrder(order: OrderLike, productTitle = order.productId): PublicOrder {
  const amountTotal = order.amountTotal || yuanToCents(order.amount) || 0;

  return {
    id: order.id,
    userId: order.userId,
    amount: amountTotal > 0 ? centsToYuanString(amountTotal) : Number(order.amount).toFixed(2),
    amountTotal,
    currency: order.currency,
    paymentProvider: order.paymentProvider,
    outTradeNo: order.outTradeNo ?? undefined,
    status: orderStatusMap[order.status],
    productType: order.productType,
    productId: order.productId,
    productTitle,
    createdAt: order.createdAt.toISOString(),
    expiresAt: order.expiresAt?.toISOString(),
    paidAt: order.paidAt?.toISOString(),
  };
}

function createOutTradeNo() {
  const timestamp = new Date().toISOString().replace(/\D/g, "").slice(0, 14);
  const suffix = randomBytes(6).toString("hex");
  return `FR${timestamp}${suffix}`;
}

async function hasActivePackageEntitlement(userId: string, packageId: string) {
  const entitlement = await prisma.entitlement.findFirst({
    where: {
      userId,
      resourceType: "package",
      resourceId: packageId,
      OR: [{ expiresAt: null }, { expiresAt: { gt: new Date() } }],
    },
    select: { id: true },
  });

  return Boolean(entitlement);
}

export async function createPackageOrder(input: { userId: string; packageId: string }) {
  const pkg = await prisma.coursePackage.findFirst({
    where: {
      id: input.packageId,
      status: RecordStatus.PUBLISHED,
    },
    select: {
      id: true,
      title: true,
      price: true,
    },
  });

  if (!pkg) {
    return { kind: "not_found" as const };
  }

  if (await hasActivePackageEntitlement(input.userId, pkg.id)) {
    return { kind: "already_owned" as const, packageId: pkg.id };
  }

  const amountTotal = yuanToCents(pkg.price);
  if (!amountTotal || amountTotal <= 0) {
    return { kind: "invalid_price" as const };
  }

  const order = await prisma.order.create({
    data: {
      userId: input.userId,
      amount: pkg.price,
      amountTotal,
      currency: "CNY",
      paymentProvider: "wechat_native",
      outTradeNo: createOutTradeNo(),
      status: OrderStatus.PENDING,
      productType: "package",
      productId: pkg.id,
    },
  });

  return { kind: "created" as const, order: toPublicOrder(order, pkg.title) };
}

export async function getOrderForUser(orderId: string, userId: string) {
  const order = await prisma.order.findFirst({
    where: { id: orderId, userId },
  });
  if (!order) {
    return null;
  }

  const productTitle = await getOrderProductTitle(order.productType, order.productId);
  return toPublicOrder(order, productTitle);
}

export async function listOrdersForUser(userId: string) {
  const orders = await prisma.order.findMany({
    where: { userId },
    orderBy: { createdAt: "desc" },
  });

  const productTitles = new Map<string, string>();
  for (const order of orders) {
    productTitles.set(
      `${order.productType}:${order.productId}`,
      await getOrderProductTitle(order.productType, order.productId),
    );
  }

  return orders.map((order) =>
    toPublicOrder(order, productTitles.get(`${order.productType}:${order.productId}`)),
  );
}

async function getOrderProductTitle(productType: string, productId: string) {
  if (productType === "package") {
    const pkg = await prisma.coursePackage.findUnique({
      where: { id: productId },
      select: { title: true },
    });
    return pkg?.title ?? productId;
  }

  if (productType === "material") {
    const material = await prisma.material.findUnique({
      where: { id: productId },
      select: { title: true },
    });
    return material?.title ?? productId;
  }

  return productId;
}

async function hasOrderEntitlement(order: Pick<OrderLike, "userId" | "productType" | "productId">) {
  const resourceType = order.productType === "package" ? "package" : order.productType;

  const entitlement = await prisma.entitlement.findFirst({
    where: {
      userId: order.userId,
      resourceType,
      resourceId: order.productId,
      OR: [{ expiresAt: null }, { expiresAt: { gt: new Date() } }],
    },
    select: { id: true },
  });

  return Boolean(entitlement);
}

export async function getOrderStatusForViewer(
  orderId: string,
  viewer: { id: string; role: UserRole },
) {
  const order = await prisma.order.findFirst({
    where: viewer.role === UserRole.ADMIN ? { id: orderId } : { id: orderId, userId: viewer.id },
  });

  if (!order) {
    return null;
  }

  return {
    orderId: order.id,
    status: orderStatusMap[order.status],
    paidAt: order.paidAt?.toISOString() ?? null,
    entitlementGranted: await hasOrderEntitlement(order),
  };
}

export async function startWechatNativePayment(input: { orderId: string; userId: string }) {
  const order = await prisma.order.findFirst({
    where: { id: input.orderId, userId: input.userId },
  });

  if (!order) {
    return { ok: false as const, status: 404, error: "Order not found" };
  }

  const amountTotal = order.amountTotal || yuanToCents(order.amount) || 0;
  const permission = canStartWechatNativePayment(
    { userId: order.userId, status: order.status, amountTotal },
    input.userId,
  );

  if (!permission.allowed) {
    return { ok: false as const, status: permission.status, error: permission.reason };
  }

  const productTitle = await getOrderProductTitle(order.productType, order.productId);
  const outTradeNo = order.outTradeNo ?? createOutTradeNo();
  const payment = await createWechatNativePayment({
    orderId: order.id,
    outTradeNo,
    description: productTitle,
    amountTotal,
  });

  const updated = await prisma.order.update({
    where: { id: order.id },
    data: {
      status: OrderStatus.PAYING,
      paymentProvider: "wechat_native",
      outTradeNo,
      amountTotal,
      codeUrl: payment.codeUrl,
      expiresAt: payment.expiresAt,
    },
  });

  return {
    ok: true as const,
    orderId: updated.id,
    codeUrl: payment.codeUrl,
    expiresAt: payment.expiresAt.toISOString(),
    status: orderStatusMap[updated.status],
  };
}
