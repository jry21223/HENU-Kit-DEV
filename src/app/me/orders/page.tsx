import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getCurrentUser } from "@/lib/auth";
import { listOrdersForUser } from "@/services/order-service";

type OrdersPageProps = {
  searchParams?: Promise<{
    payment?: string;
    orderId?: string;
  }>;
};

const statusLabels = {
  pending: "待支付",
  paying: "待扫码",
  paid: "已支付",
  failed: "支付失败",
  closed: "已关闭",
  expired: "已过期",
  cancelled: "已取消",
  refunded: "已退款",
} as const;

function getPaymentMessage(payment?: string) {
  if (payment === "success") {
    return "支付结果已确认，订单状态已更新。";
  }
  if (payment === "invalid") {
    return "支付返回校验失败，未更新订单。";
  }
  if (payment === "failed") {
    return "支付未成功，订单仍待处理。";
  }
  return null;
}

export default async function OrdersPage({ searchParams }: OrdersPageProps) {
  const user = await getCurrentUser();
  const resolvedSearchParams = await searchParams;

  if (!user) {
    return (
      <PageShell
        eyebrow="我的复习"
        title="我的订单"
        description="登录后查看课程包订单和支付状态。"
      >
        <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <p className="text-sm text-muted">请先使用学生邮箱登录。</p>
          <Link
            href="/login"
            className="mt-4 inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
          >
            去登录
          </Link>
        </div>
      </PageShell>
    );
  }

  const orders = await listOrdersForUser(user.id);
  const paymentMessage = getPaymentMessage(resolvedSearchParams?.payment);

  return (
    <PageShell
      eyebrow="我的复习"
      title="我的订单"
      description="查看课程复习包订单状态。支付成功后由服务端回调发放权限。"
    >
      {paymentMessage ? (
        <div className="mb-5 rounded-lg border border-line bg-white p-4 text-sm font-semibold text-ink shadow-soft">
          {paymentMessage}
        </div>
      ) : null}

      {orders.length > 0 ? (
        <div className="grid gap-4">
          {orders.map((order) => (
            <article key={order.id} className="rounded-lg border border-line bg-white p-5 shadow-soft">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-ink">{order.productTitle}</h2>
                  <p className="mt-2 text-sm text-muted">订单号：{order.id}</p>
                  <p className="mt-1 text-sm text-muted">
                    创建时间：{new Date(order.createdAt).toLocaleString()}
                  </p>
                  {order.expiresAt ? (
                    <p className="mt-1 text-sm text-muted">
                      过期时间：{new Date(order.expiresAt).toLocaleString()}
                    </p>
                  ) : null}
                  {order.paidAt ? (
                    <p className="mt-1 text-sm text-muted">
                      支付时间：{new Date(order.paidAt).toLocaleString()}
                    </p>
                  ) : null}
                </div>
                <div className="text-left sm:text-right">
                  <p className="text-lg font-semibold text-ink">￥{order.amount}</p>
                  <p className="mt-2 rounded-md border border-line bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
                    {statusLabels[order.status]}
                  </p>
                </div>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
          暂无订单。
        </div>
      )}
    </PageShell>
  );
}
