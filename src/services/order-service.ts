import { OrderStatus, RecordStatus } from "@prisma/client";
import { prisma } from "@/lib/db";
import {
  buildEasypayPaymentParams,
  buildEasypayPaymentUrl,
  formatMoney,
  getEasypayConfig,
  type EasypayParams,
} from "@/lib/easypay";

export type PublicOrder = {
  id: string;
  userId: string;
  amount: string;
  status: "pending" | "paid" | "failed" | "cancelled" | "refunded";
  productType: string;
  productId: string;
  productTitle: string;
  createdAt: string;
  paidAt?: string;
};

const orderStatusMap: Record<OrderStatus, PublicOrder["status"]> = {
  PENDING: "pending",
  PAID: "paid",
  FAILED: "failed",
  CANCELLED: "cancelled",
  REFUNDED: "refunded",
};

function toPublicOrder(
  order: {
    id: string;
    userId: string;
    amount: unknown;
    status: OrderStatus;
    productType: string;
    productId: string;
    createdAt: Date;
    paidAt: Date | null;
  },
  productTitle = order.productId,
): PublicOrder {
  return {
    id: order.id,
    userId: order.userId,
    amount: Number(order.amount).toFixed(2),
    status: orderStatusMap[order.status],
    productType: order.productType,
    productId: order.productId,
    productTitle,
    createdAt: order.createdAt.toISOString(),
    paidAt: order.paidAt?.toISOString(),
  };
}

export async function createPackageOrder(input: {
  userId: string;
  packageId: string;
  paymentType?: string;
}) {
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
    return null;
  }

  const order = await prisma.order.create({
    data: {
      userId: input.userId,
      amount: pkg.price,
      status: OrderStatus.PENDING,
      productType: "package",
      productId: pkg.id,
    },
  });

  const config = getEasypayConfig();
  const paymentParams = buildEasypayPaymentParams({
    orderId: order.id,
    name: pkg.title,
    money: Number(order.amount).toFixed(2),
    config,
    paymentType: input.paymentType,
  });

  return {
    order: toPublicOrder(order, pkg.title),
    payment: {
      provider: "easypay",
      configured: config.configured,
      gatewayUrl: config.gatewayUrl,
      paymentUrl: buildEasypayPaymentUrl(paymentParams, config.gatewayUrl),
      params: paymentParams,
    },
  };
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

async function grantOrderEntitlement(order: {
  userId: string;
  productType: string;
  productId: string;
}) {
  const resourceType = order.productType === "package" ? "package" : order.productType;

  if (!["package", "material"].includes(resourceType)) {
    throw new Error("Unsupported order product type");
  }

  await prisma.entitlement.upsert({
    where: {
      userId_resourceType_resourceId: {
        userId: order.userId,
        resourceType,
        resourceId: order.productId,
      },
    },
    update: {
      source: "easypay",
      expiresAt: null,
    },
    create: {
      userId: order.userId,
      resourceType,
      resourceId: order.productId,
      source: "easypay",
      expiresAt: null,
    },
  });
}

export async function settleEasypayOrder(params: EasypayParams) {
  const outTradeNo = String(params.out_trade_no ?? "");
  const tradeStatus = String(params.trade_status ?? "");
  const paidMoney = formatMoney(params.money);

  if (!outTradeNo || tradeStatus !== "TRADE_SUCCESS" || !paidMoney) {
    return { ok: false, status: 400, message: "支付通知参数无效。" };
  }

  const order = await prisma.order.findUnique({
    where: { id: outTradeNo },
  });

  if (!order) {
    return { ok: false, status: 404, message: "订单不存在。" };
  }

  const orderMoney = Number(order.amount).toFixed(2);
  if (orderMoney !== paidMoney) {
    return { ok: false, status: 400, message: "订单金额不匹配。" };
  }

  if (order.status === OrderStatus.PAID) {
    await grantOrderEntitlement(order);
    return { ok: true, status: 200, message: "订单已支付。" };
  }

  if (order.status !== OrderStatus.PENDING) {
    return { ok: false, status: 409, message: "订单状态不允许支付。" };
  }

  await prisma.$transaction(async (tx) => {
    await tx.order.update({
      where: { id: order.id },
      data: {
        status: OrderStatus.PAID,
        paidAt: new Date(),
      },
    });
    const resourceType = order.productType === "package" ? "package" : order.productType;
    if (!["package", "material"].includes(resourceType)) {
      throw new Error("Unsupported order product type");
    }
    await tx.entitlement.upsert({
      where: {
        userId_resourceType_resourceId: {
          userId: order.userId,
          resourceType,
          resourceId: order.productId,
        },
      },
      update: {
        source: "easypay",
        expiresAt: null,
      },
      create: {
        userId: order.userId,
        resourceType,
        resourceId: order.productId,
        source: "easypay",
        expiresAt: null,
      },
    });
  });

  return { ok: true, status: 200, message: "订单支付成功。" };
}
