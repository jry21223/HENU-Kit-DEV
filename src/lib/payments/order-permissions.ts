export type PayableOrderStatus = "PENDING" | "PAYING" | "PAID" | "FAILED" | "CLOSED" | "EXPIRED" | "CANCELLED" | "REFUNDED";

export type PaymentPermissionResult =
  | { allowed: true }
  | { allowed: false; status: number; reason: string };

export function canStartWechatNativePayment(
  order: { userId: string; status: PayableOrderStatus; amountTotal: number },
  userId: string,
): PaymentPermissionResult {
  if (order.userId !== userId) {
    return { allowed: false, status: 404, reason: "order_not_found" };
  }

  if (!["PENDING", "PAYING"].includes(order.status)) {
    return { allowed: false, status: 409, reason: "order_status_not_payable" };
  }

  if (!Number.isInteger(order.amountTotal) || order.amountTotal <= 0) {
    return { allowed: false, status: 400, reason: "invalid_order_amount" };
  }

  return { allowed: true };
}

export function shouldCreatePackageOrder(hasActiveEntitlement: boolean) {
  return !hasActiveEntitlement;
}
