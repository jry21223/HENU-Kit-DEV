import {
  CloseCircleOutlined,
  CrownOutlined,
  LeftOutlined,
  ReloadOutlined,
  RightOutlined,
  RollbackOutlined,
  SearchOutlined,
  SyncOutlined,
} from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  Modal,
  Segmented,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { PendingRecoveryAlert } from "@/components/PendingRecoveryAlert";
import { accountName, accountStatusLabel } from "@/lib/account-labels";
import { useConsole } from "@/lib/console-context";
import {
  closeAccountMembershipOrder,
  fetchAccountMembershipOrderRefund,
  fetchAccountMembershipOrders,
  refundAccountMembershipOrder,
  type ConsoleMembershipOrderItem,
} from "@/lib/console-extensions";
import {
  fetchAccountMembership,
  grantAccountMembership,
  resolveAccountOperation,
  revokeAccountMembership,
  searchAccountMemberships,
  type AccountMembershipWriteResult,
  type ConsoleAccountMembership,
  type ConsoleMembershipAccountSummary,
  type ConsoleMembershipMutationRequest,
} from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import {
  clearPendingMembershipOperation,
  readPendingMembershipOperation,
  writePendingMembershipOperation,
  type PendingMembershipOperation,
} from "@/lib/pending-operations";

function membershipTag(membership: ConsoleAccountMembership | null) {
  if (!membership) return <Tag>未初始化</Tag>;
  if (membership.plan === "lifetime" && membership.lifetime) {
    return <Tag color="gold">终身会员</Tag>;
  }
  return <Tag color="default">普通用户</Tag>;
}

function orderStatusTag(status: string) {
  switch (status) {
    case "paid":
      return <Tag color="success">已支付</Tag>;
    case "created":
    case "pending_payment":
      return <Tag color="processing">待支付</Tag>;
    case "closed":
      return <Tag color="default">已关闭</Tag>;
    case "refunded":
      return <Tag color="warning">已退款</Tag>;
    default:
      return <Tag>{status}</Tag>;
  }
}

function refundStatusLabel(status: string): string {
  return (
    {
      processing: "退款处理中",
      succeeded: "已退款到账",
      closed: "退款已关闭",
      abnormal: "退款异常（需人工核查）",
    }[status] ?? status
  );
}

function formatCents(cents: number): string {
  return `¥${(cents / 100).toFixed(2)}`;
}

function validOutcome(command: PendingMembershipOperation, value: ConsoleAccountMembership) {
  return (
    value.version === command.expected_version + 1 &&
    value.lifetime === (command.operation === "membership_grant") &&
    value.plan === (command.operation === "membership_grant" ? "lifetime" : "free")
  );
}

export function AccountMembershipsPage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId, permissions } = useConsole();

  const canWrite = permissions.includes("account.membership.write");
  const canReadOrders = permissions.includes("account.orders.read");
  const canCloseOrders = permissions.includes("account.orders.close");
  const canRefundOrders = permissions.includes("account.orders.refund");
  const canAccessPage = canWrite || canReadOrders;

  const [activeTab, setActiveTab] = useState<"entitlements" | "orders">(
    canWrite ? "entitlements" : "orders",
  );

  // Tab 1: 会员权益管理
  const [searchInput, setSearchInput] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [page, setPage] = useState(1);
  const [nextPage, setNextPage] = useState<number | null>(null);
  const [accounts, setAccounts] = useState<ConsoleMembershipAccountSummary[]>([]);
  const [listState, setListState] = useState<
    "loading" | "ready" | "empty" | "denied" | "signed_out" | "invalid" | "unavailable"
  >("loading");

  const [selected, setSelected] = useState<ConsoleMembershipAccountSummary | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [reason, setReason] = useState("");
  const [confirmOpen, setConfirmOpen] = useState<"grant" | "revoke" | null>(null);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingMembershipOperation | undefined>();

  // Tab 2: 会员订单与退款
  const [orderStatusFilter, setOrderStatusFilter] = useState("all");
  const [ordersState, setOrdersState] = useState<
    "loading" | "ready" | "denied" | "signed_out" | "unavailable"
  >("loading");
  const [orders, setOrders] = useState<ConsoleMembershipOrderItem[]>([]);
  const [orderActionModal, setOrderActionModal] = useState<{
    kind: "close" | "refund";
    order: ConsoleMembershipOrderItem;
  } | null>(null);
  const [orderActionReason, setOrderActionReason] = useState("");
  const [orderActionBusy, setOrderActionBusy] = useState(false);
  const [syncingRefundId, setSyncingRefundId] = useState<string | null>(null);

  const mountedRef = useRef(true);
  const listSeqRef = useRef(0);
  const ordersSeqRef = useRef(0);
  const confirmedMapRef = useRef(new Map<string, ConsoleAccountMembership>());

  const loadAccounts = useCallback(
    async (targetPage = 1, query = activeQuery) => {
      if (authState !== "authenticated" || !operatorId || !canWrite) {
        setListState(
          authState === "loading" ? "loading" : authState === "signed_out" ? "signed_out" : "denied",
        );
        return;
      }
      const seq = ++listSeqRef.current;
      setListState("loading");
      const res = await searchAccountMemberships({ query, page: targetPage });
      if (!mountedRef.current || seq !== listSeqRef.current) return;
      if (res.state === "authenticated") {
        const merged = res.page.accounts.map((item) => {
          const confirmed = confirmedMapRef.current.get(item.id);
          return item.membership && confirmed && confirmed.version > item.membership.version
            ? { ...item, membership: confirmed }
            : item;
        });
        setAccounts(merged);
        setPage(targetPage);
        setNextPage(res.page.next_page);
        setListState(merged.length ? "ready" : "empty");
      } else {
        setAccounts([]);
        setNextPage(null);
        setListState(
          res.state === "signed_out"
            ? "signed_out"
            : res.state === "denied"
              ? "denied"
              : res.state === "invalid"
                ? "invalid"
                : "unavailable",
        );
      }
    },
    [authState, operatorId, canWrite, activeQuery],
  );

  const loadOrders = useCallback(
    async (status = orderStatusFilter) => {
      if (authState !== "authenticated" || !canReadOrders) {
        setOrdersState(authState === "loading" ? "loading" : "denied");
        return;
      }
      const seq = ++ordersSeqRef.current;
      setOrdersState("loading");
      const res = await fetchAccountMembershipOrders({ status, limit: 50 });
      if (!mountedRef.current || seq !== ordersSeqRef.current) return;
      if (res.state === "authenticated") {
        setOrders(res.data);
        setOrdersState("ready");
      } else {
        setOrders([]);
        setOrdersState(
          res.state === "signed_out"
            ? "signed_out"
            : res.state === "denied"
              ? "denied"
              : "unavailable",
        );
      }
    },
    [authState, canReadOrders, orderStatusFilter],
  );

  const queryPending = async (
    command: PendingMembershipOperation,
  ): Promise<AccountMembershipWriteResult | { state: "unknown" }> => {
    const res = await resolveAccountOperation(command.operation, command.idempotency_key);
    if (res.state !== "succeeded") return { state: res.state };
    const value = res.result.result;
    return value && "membership" in value
      ? { state: "succeeded", membership: value.membership }
      : { state: "unavailable" };
  };

  const settleResult = useCallback(
    async (
      command: PendingMembershipOperation,
      result: AccountMembershipWriteResult | { state: "unknown" },
    ) => {
      if (result.state === "succeeded" && validOutcome(command, result.membership)) {
        confirmedMapRef.current.set(command.resource_id, result.membership);
        setPending(undefined);
        clearPendingMembershipOperation();
        setAccounts((prev) =>
          prev.map((item) =>
            item.id === command.resource_id ? { ...item, membership: result.membership } : item,
          ),
        );
        setSelected((prev) =>
          prev && prev.id === command.resource_id
            ? { ...prev, membership: result.membership }
            : prev,
        );
        setReason("");
        message.success(
          command.operation === "membership_grant" ? "已发放终身会员权益。" : "已撤销终身会员权益。",
        );
      } else if (result.state === "conflict") {
        setPending(undefined);
        clearPendingMembershipOperation();
        message.warning("该用户的会员状态刚刚变化，已为你刷新最新信息，请核对后重新确认。");
        if (selected?.id === command.resource_id) {
          setDetailLoading(true);
          const refreshed = await fetchAccountMembership(command.resource_id);
          if (mountedRef.current) {
            setDetailLoading(false);
            if (refreshed.state === "authenticated") {
              setSelected((prev) =>
                prev && prev.id === command.resource_id
                  ? { ...prev, membership: refreshed.membership }
                  : prev,
              );
            }
          }
        }
      } else {
        message.warning(
          result.state === "signed_out"
            ? "登录已过期，请重新登录后再核对原操作结果。"
            : "操作结果暂未确认，已保留记录；请点击上方横幅查询，切勿重复操作。",
        );
      }
    },
    [message, selected],
  );

  const resolvePending = useCallback(
    async (target?: PendingMembershipOperation) => {
      const op = target ?? pending;
      if (!op || busy || !operatorId || op.operator_id !== operatorId) return;
      setBusy(true);
      const res = await queryPending(op);
      if (!mountedRef.current) return;
      await settleResult(op, res);
      if (mountedRef.current) setBusy(false);
    },
    [pending, busy, operatorId, settleResult],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId) {
      if (canWrite) {
        const saved = readPendingMembershipOperation(operatorId);
        setPending(saved);
        void loadAccounts(1, "");
        if (saved) void resolvePending(saved);
      }
      if (canReadOrders) {
        void loadOrders("all");
      }
    } else {
      setListState(authState === "loading" ? "loading" : "denied");
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId, canWrite, canReadOrders]);

  const handleSearch = () => {
    const trimmed = searchInput.trim();
    setActiveQuery(trimmed);
    setSelected(null);
    void loadAccounts(1, trimmed);
  };

  const handleConfirmMutation = async () => {
    if (!selected?.membership || !confirmOpen || !operatorId || busy || pending) return;
    const trimmedReason = reason.trim();
    if (!trimmedReason) return;
    const operation = confirmOpen === "grant" ? "membership_grant" : "membership_revoke";
    const key = `idem_account_membership_${confirmOpen}_${crypto.randomUUID()}`;
    const command: PendingMembershipOperation = {
      version: 1,
      operator_id: operatorId,
      operation,
      resource_id: selected.id,
      expected_version: selected.membership.version,
      idempotency_key: key,
    };
    const input: ConsoleMembershipMutationRequest = {
      reason: trimmedReason,
      expected_version: selected.membership.version,
    };
    if (!writePendingMembershipOperation(command)) {
      message.error("浏览器暂不允许会话存储，操作未提交。");
      return;
    }
    setConfirmOpen(null);
    setPending(command);
    setBusy(true);
    let res: AccountMembershipWriteResult | { state: "unknown" } =
      operation === "membership_grant"
        ? await grantAccountMembership(selected.id, input, key)
        : await revokeAccountMembership(selected.id, input, key);
    if (!mountedRef.current) return;
    if (
      res.state !== "conflict" &&
      res.state !== "signed_out" &&
      (res.state !== "succeeded" || !validOutcome(command, res.membership))
    ) {
      res = await queryPending(command);
      if (!mountedRef.current) return;
    }
    await settleResult(command, res);
    if (mountedRef.current) setBusy(false);
  };

  const handleConfirmOrderAction = async () => {
    if (!orderActionModal || !orderActionReason.trim() || orderActionBusy) return;
    const { kind, order } = orderActionModal;
    setOrderActionBusy(true);
    const key = `idem_account_order_${kind}_${crypto.randomUUID()}`;
    const payload = { expected_version: order.version, reason: orderActionReason.trim() };
    const res =
      kind === "close"
        ? await closeAccountMembershipOrder(order.id, payload, key)
        : await refundAccountMembershipOrder(order.id, payload, key);
    if (!mountedRef.current) return;
    setOrderActionBusy(false);
    if (res.state === "succeeded") {
      const dataObj = res.data as {
        order: ConsoleMembershipOrderItem;
        refund?: { status?: string };
      };
      const refundStatus = dataObj.refund?.status ?? "";
      message.success(
        kind === "close"
          ? "未支付订单已关闭。"
          : `退款请求已受理${refundStatus ? `（状态：${refundStatusLabel(refundStatus)}）` : ""}。`,
      );
      setOrderActionModal(null);
      setOrderActionReason("");
      void loadOrders(orderStatusFilter);
      if (kind === "refund" && canWrite) {
        void loadAccounts(page, activeQuery);
      }
    } else if (res.state === "conflict") {
      message.warning("该订单状态刚刚发生变化，已为你刷新最新订单列表。");
      setOrderActionModal(null);
      void loadOrders(orderStatusFilter);
    } else {
      message.error(res.message || "订单操作未能完成，请稍后重试。");
    }
  };

  const handleRefreshRefund = async (order: ConsoleMembershipOrderItem) => {
    if (!order.latest_refund?.id || syncingRefundId) return;
    setSyncingRefundId(order.id);
    const res = await fetchAccountMembershipOrderRefund(order.id, order.latest_refund.id);
    if (!mountedRef.current) return;
    setSyncingRefundId(null);
    if (res.state === "authenticated") {
      message.success(`最新退款状态：${refundStatusLabel(res.data.refund.status)}`);
      void loadOrders(orderStatusFilter);
    } else {
      message.error("暂时无法同步退款状态，请稍后重试。");
    }
  };

  const accountColumns: ColumnsType<ConsoleMembershipAccountSummary> = [
    {
      title: "用户",
      key: "user",
      render: (_, item) => (
        <div>
          <Typography.Text strong>{accountName(item.display_name)}</Typography.Text>
          <div style={{ fontSize: 12, color: "#64748b", wordBreak: "break-all" }}>
            {item.email}
          </div>
        </div>
      ),
    },
    {
      title: "账户状态",
      dataIndex: "status",
      width: 110,
      render: (status: string) => (
        <Tag color={status === "active" ? "success" : status === "suspended" ? "warning" : "error"}>
          {accountStatusLabel(status)}
        </Tag>
      ),
    },
    {
      title: "当前会员权益",
      key: "membership",
      width: 130,
      render: (_, item) => membershipTag(item.membership),
    },
    {
      title: "操作",
      key: "actions",
      width: 120,
      render: (_, item) => (
        <Button
          size="small"
          icon={<CrownOutlined />}
          onClick={() => {
            setSelected(item);
            setReason("");
          }}
        >
          管理权益
        </Button>
      ),
    },
  ];

  const orderColumns: ColumnsType<ConsoleMembershipOrderItem> = [
    {
      title: "订单编号",
      key: "id",
      width: 150,
      render: (_, item) => <span className="console-code-chip">{item.id.slice(0, 8)}…</span>,
    },
    {
      title: "下单用户",
      key: "user",
      render: (_, item) => (
        <div>
          <Typography.Text strong>
            {item.display_name ? accountName(item.display_name) : item.user_id?.slice(0, 8) || "未知用户"}
          </Typography.Text>
          {item.email && <div style={{ fontSize: 12, color: "#64748b" }}>{item.email}</div>}
        </div>
      ),
    },
    {
      title: "方案与金额",
      key: "amount",
      width: 140,
      render: (_, item) => (
        <div>
          <div>{item.plan === "lifetime" ? "终身会员" : item.plan}</div>
          <Typography.Text strong>{formatCents(item.amount_cents)}</Typography.Text>
        </div>
      ),
    },
    {
      title: "订单状态",
      key: "status",
      width: 150,
      render: (_, item) => (
        <Space orientation="vertical" size={2}>
          {orderStatusTag(item.status)}
          {item.latest_refund && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {refundStatusLabel(item.latest_refund.status)}
            </Typography.Text>
          )}
        </Space>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "created_at",
      width: 155,
      render: (val: string) => localDateTime(val),
    },
    {
      title: "操作",
      key: "actions",
      width: 190,
      render: (_, item) => (
        <Space size={6} wrap>
          {canCloseOrders && (item.status === "created" || item.status === "pending_payment") && (
            <Button
              size="small"
              icon={<CloseCircleOutlined />}
              onClick={() => {
                setOrderActionReason("");
                setOrderActionModal({ kind: "close", order: item });
              }}
            >
              关闭订单
            </Button>
          )}
          {canRefundOrders && item.status === "paid" && (
            <Button
              danger
              size="small"
              icon={<RollbackOutlined />}
              onClick={() => {
                setOrderActionReason("");
                setOrderActionModal({ kind: "refund", order: item });
              }}
            >
              发起退款
            </Button>
          )}
          {item.latest_refund && item.latest_refund.status === "processing" && (
            <Button
              size="small"
              icon={<SyncOutlined spin={syncingRefundId === item.id} />}
              onClick={() => void handleRefreshRefund(item)}
            >
              同步退款
            </Button>
          )}
          {!(canCloseOrders && (item.status === "created" || item.status === "pending_payment")) &&
            !(canRefundOrders && item.status === "paid") &&
            !(item.latest_refund && item.latest_refund.status === "processing") && (
              <Typography.Text type="secondary">无需操作</Typography.Text>
            )}
        </Space>
      ),
    },
  ];

  const tabItems = [
    ...(canWrite
      ? [
          {
            key: "entitlements",
            label: "会员权益管理",
            children: (
              <>
                <Space wrap style={{ marginBottom: 16 }}>
                  <Input
                    allowClear
                    prefix={<SearchOutlined />}
                    placeholder="输入显示名称或完整邮箱…"
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                    onPressEnter={handleSearch}
                    style={{ width: 280 }}
                  />
                  <Button type="primary" loading={listState === "loading"} onClick={handleSearch}>
                    搜索用户
                  </Button>
                  {activeQuery && (
                    <Button
                      onClick={() => {
                        setSearchInput("");
                        setActiveQuery("");
                        void loadAccounts(1, "");
                      }}
                    >
                      清除筛选
                    </Button>
                  )}
                </Space>

                {listState === "unavailable" || listState === "invalid" ? (
                  <ModuleStateView
                    state={listState}
                    invalidTitle="搜索条件无效"
                    invalidDescription="请检查姓名或邮箱格式后重试。"
                    onRetry={() => void loadAccounts(page, activeQuery)}
                  />
                ) : (
                  <>
                    <Table
                      rowKey="id"
                      loading={listState === "loading"}
                      columns={accountColumns}
                      dataSource={accounts}
                      scroll={{ x: "max-content" }}
                      pagination={false}
                      locale={{ emptyText: "没有找到匹配的用户，可清空搜索词查看列表" }}
                    />
                    <div
                      style={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        marginTop: 16,
                      }}
                    >
                      <Button
                        icon={<LeftOutlined />}
                        disabled={page <= 1 || listState === "loading"}
                        onClick={() => void loadAccounts(page - 1, activeQuery)}
                      >
                        上一页
                      </Button>
                      <Typography.Text type="secondary">第 {page} 页（每页最多 20 人）</Typography.Text>
                      <Button
                        icon={<RightOutlined />}
                        iconPlacement="end"
                        disabled={nextPage === null || listState === "loading"}
                        onClick={() => nextPage && void loadAccounts(nextPage, activeQuery)}
                      >
                        下一页
                      </Button>
                    </div>
                  </>
                )}
              </>
            ),
          },
        ]
      : []),
    ...(canReadOrders
      ? [
          {
            key: "orders",
            label: `会员订单与退款 (${orders.length})`,
            children: (
              <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                <div
                  style={{
                    display: "flex",
                    flexWrap: "wrap",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 12,
                  }}
                >
                  <Segmented
                    value={orderStatusFilter}
                    onChange={(val) => {
                      const next = String(val);
                      setOrderStatusFilter(next);
                      void loadOrders(next);
                    }}
                    options={[
                      { label: "全部", value: "all" },
                      { label: "待支付", value: "pending_payment" },
                      { label: "已支付", value: "paid" },
                      { label: "已关闭", value: "closed" },
                      { label: "已退款", value: "refunded" },
                    ]}
                  />
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    loading={ordersState === "loading"}
                    onClick={() => void loadOrders(orderStatusFilter)}
                  >
                    刷新订单
                  </Button>
                </div>

                {ordersState === "unavailable" || ordersState === "denied" ? (
                  <ModuleStateView
                    state={ordersState}
                    deniedTitle="没有会员订单查看权限"
                    onRetry={() => void loadOrders(orderStatusFilter)}
                  />
                ) : (
                  <Table
                    rowKey="id"
                    loading={ordersState === "loading"}
                    columns={orderColumns}
                    dataSource={orders}
                    scroll={{ x: "max-content" }}
                    pagination={{ pageSize: 12, showSizeChanger: false }}
                    locale={{ emptyText: "当前筛选下暂无会员订单" }}
                  />
                )}
              </Space>
            ),
          },
        ]
      : []),
  ];

  return (
    <section aria-labelledby="account-membership-heading">
      <div className="console-page-header">
        <div>
          <h1 id="account-membership-heading" className="console-page-title">
            会员与订单
          </h1>
          <p className="console-page-subtitle">
            核对学生账户并发放或撤销终身会员权益，统一处理未支付关单与已支付订单退款。
          </p>
        </div>
        <Button
          icon={<ReloadOutlined />}
          onClick={() => {
            if (canWrite) void loadAccounts(page, activeQuery);
            if (canReadOrders) void loadOrders(orderStatusFilter);
          }}
        >
          刷新列表
        </Button>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="有一项会员权益变更尚未确认结果。请点击核对原操作结果，不会重复发放或撤销。"
        busy={busy}
        onResolve={() => void resolvePending()}
      />

      {!canAccessPage || authState !== "authenticated" ? (
        <ModuleStateView
          state={authState === "loading" ? "loading" : authState === "signed_out" ? "signed_out" : "denied"}
          deniedTitle="没有会员与订单管理权限"
        />
      ) : (
        <Card>
          <Tabs
            activeKey={activeTab}
            onChange={(key) => setActiveTab(key as "entitlements" | "orders")}
            items={tabItems}
          />
        </Card>
      )}

      {/* 右侧会员权益管理抽屉 */}
      <Drawer
        open={Boolean(selected)}
        title="管理会员权益"
        size={480}
        onClose={() => setSelected(null)}
      >
        {selected && (
          <Space orientation="vertical" size={16} style={{ width: "100%" }}>
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="用户姓名">{accountName(selected.display_name)}</Descriptions.Item>
              <Descriptions.Item label="学校邮箱">{selected.email}</Descriptions.Item>
              <Descriptions.Item label="账户状态">
                {accountStatusLabel(selected.status)}
              </Descriptions.Item>
              <Descriptions.Item label="当前会员">
                {detailLoading ? "刷新中…" : membershipTag(selected.membership)}
              </Descriptions.Item>
            </Descriptions>

            {!selected.membership ? (
              <Alert
                type="info"
                showIcon
                title="该用户尚未激活账户中心"
                description="用户至少登录过一次主站账户中心后才会建立权益记录，目前暂时无法发放会员。"
              />
            ) : (
              <Card
                size="small"
                title={selected.membership.plan === "free" ? "发放终身会员" : "撤销终身会员"}
              >
                <Space orientation="vertical" size={12} style={{ width: "100%" }}>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    提交后权益立即生效，系统会记录操作日志并向该用户发送一条通知。
                  </Typography.Text>
                  <Input.TextArea
                    rows={4}
                    maxLength={1000}
                    showCount
                    style={{ marginBottom: 16 }}
                    placeholder={
                      selected.membership.plan === "free"
                        ? "请填写发放终身会员的原因（如：活动奖励、内测贡献、线下补单）"
                        : "请填写撤销终身会员的原因（如：误发回收、退款撤销）"
                    }
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                  <Button
                    type="primary"
                    danger={selected.membership.plan !== "free"}
                    disabled={busy || Boolean(pending) || !reason.trim()}
                    onClick={() =>
                      setConfirmOpen(selected.membership?.plan === "free" ? "grant" : "revoke")
                    }
                  >
                    {selected.membership.plan === "free" ? "发放终身会员" : "撤销终身会员"}
                  </Button>
                </Space>
              </Card>
            )}
          </Space>
        )}
      </Drawer>

      {/* 会员权益高风险确认弹窗 */}
      <Modal
        open={Boolean(confirmOpen)}
        title={confirmOpen === "grant" ? "确认发放终身会员？" : "确认撤销终身会员？"}
        okText="确认执行"
        cancelText="取消"
        okButtonProps={{ danger: confirmOpen === "revoke" }}
        confirmLoading={busy}
        onOk={() => void handleConfirmMutation()}
        onCancel={() => setConfirmOpen(null)}
      >
        {selected && (
          <Space orientation="vertical" size={10} style={{ width: "100%", marginTop: 8 }}>
            <Typography.Text>
              即将为 <strong>{accountName(selected.display_name)}</strong>（{selected.email}）
              {confirmOpen === "grant" ? "开通终身会员权益" : "撤销终身会员权益"}。
            </Typography.Text>
            <Typography.Text type="secondary">操作原因：{reason.trim()}</Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              提交后立即生效，并向该用户发送站内通知。
            </Typography.Text>
          </Space>
        )}
      </Modal>

      {/* 订单关单/退款二次确认弹窗 */}
      <Modal
        open={Boolean(orderActionModal)}
        title={orderActionModal?.kind === "close" ? "确认关闭未支付订单？" : "确认发起订单退款？"}
        okText={orderActionModal?.kind === "close" ? "确认关单" : "确认退款"}
        cancelText="取消"
        okButtonProps={{
          danger: orderActionModal?.kind === "refund",
          disabled: orderActionReason.trim().length < 2,
        }}
        confirmLoading={orderActionBusy}
        onOk={() => void handleConfirmOrderAction()}
        onCancel={() => {
          setOrderActionModal(null);
          setOrderActionReason("");
        }}
      >
        {orderActionModal && (
          <Space orientation="vertical" size={12} style={{ width: "100%", marginTop: 8 }}>
            {orderActionModal.kind === "refund" && (
              <Alert
                type="warning"
                showIcon
                title="退款受理后将按支付渠道原路退回，并在退款成功后自动回收该笔订单对应的会员权益。"
              />
            )}
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="订单编号">
                <span className="console-code-chip">{orderActionModal.order.id}</span>
              </Descriptions.Item>
              <Descriptions.Item label="金额">
                {formatCents(orderActionModal.order.amount_cents)}
              </Descriptions.Item>
            </Descriptions>
            <div>
              <Typography.Text type="secondary" style={{ display: "block", marginBottom: 6 }}>
                请填写操作原因（至少 2 个字，将写入审计日志）
              </Typography.Text>
              <Input.TextArea
                rows={3}
                maxLength={1000}
                showCount
                style={{ marginBottom: 24 }}
                placeholder={
                  orderActionModal.kind === "close"
                    ? "例如：用户确认放弃支付 / 长时间未支付清理"
                    : "例如：学生申请退款，已核实原订单信息"
                }
                value={orderActionReason}
                onChange={(e) => setOrderActionReason(e.target.value)}
              />
            </div>
          </Space>
        )}
      </Modal>
    </section>
  );
}
