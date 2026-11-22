import { CheckCircleOutlined, SearchOutlined, WalletOutlined } from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Empty,
  Input,
  InputNumber,
  Modal,
  Row,
  Space,
  Spin,
  Statistic,
  Table,
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
  fetchAccountPointsDetail,
  type ConsolePointsAccountDetail,
} from "@/lib/console-extensions";
import {
  adjustAccountPoints,
  lookupConsoleAccount,
  resolveAccountOperation,
  type AccountPointAdjustmentWriteResult,
  type ConsoleAccountPointAdjustmentResult,
  type ConsoleLookedUpAccount,
  type ConsolePointLedgerEntry,
} from "@/lib/console-gateway";
import { formatNumber, localDateTime, localDateTimeSeconds } from "@/lib/format";
import {
  clearPendingPointOperation,
  readPendingPointOperation,
  writePendingPointOperation,
  type PendingPointOperation,
} from "@/lib/pending-operations";

const QUICK_AMOUNTS = [10, 50, 100, -10, -50];

export function AccountPointsPage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId, permissions } = useConsole();

  const canAdjust = permissions.includes("account.points.adjust");
  const [targetEmail, setTargetEmail] = useState("");
  const [lookupState, setLookupState] = useState<
    "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable"
  >("idle");
  const [account, setAccount] = useState<ConsoleLookedUpAccount>();
  const [pointsDetail, setPointsDetail] = useState<ConsolePointsAccountDetail>();
  const [pointsDetailState, setPointsDetailState] = useState<
    "idle" | "loading" | "ready" | "unavailable"
  >("idle");
  const [amount, setAmount] = useState<number | null>(null);
  const [reason, setReason] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingPointOperation | undefined>();
  const [receipt, setReceipt] = useState<{
    resourceId: string;
    data: ConsoleAccountPointAdjustmentResult;
  }>();

  const mountedRef = useRef(true);
  const lookupSeqRef = useRef(0);
  const detailSeqRef = useRef(0);

  const loadPointsDetail = useCallback(async (userId: string) => {
    const seq = ++detailSeqRef.current;
    setPointsDetailState("loading");
    const res = await fetchAccountPointsDetail(userId, 20);
    if (!mountedRef.current || seq !== detailSeqRef.current) return;
    if (res.state === "authenticated") {
      setPointsDetail(res.data);
      setPointsDetailState("ready");
    } else {
      setPointsDetail(undefined);
      setPointsDetailState("unavailable");
    }
  }, []);

  const queryPending = async (
    command: PendingPointOperation,
  ): Promise<AccountPointAdjustmentWriteResult | { state: "unknown" }> => {
    const res = await resolveAccountOperation("point_adjustment", command.idempotency_key);
    if (res.state !== "succeeded") return { state: res.state };
    const value = res.result.result;
    return value && "balance" in value && "entry" in value
      ? { state: "succeeded", result: value }
      : { state: "unavailable" };
  };

  const settle = useCallback(
    (
      command: PendingPointOperation,
      result: AccountPointAdjustmentWriteResult | { state: "unknown" },
    ) => {
      if (result.state === "succeeded" && result.result.entry.amount === command.expected_amount) {
        setReceipt({ resourceId: command.resource_id, data: result.result });
        setPending(undefined);
        clearPendingPointOperation();
        setAmount(null);
        setReason("");
        message.success("积分调整已入账。");
        void loadPointsDetail(command.resource_id);
      } else if (result.state === "conflict") {
        setPending(undefined);
        clearPendingPointOperation();
        message.error("扣减失败：对方积分余额不足或数据刚刚变动，本次未扣减。");
      } else {
        message.warning(
          result.state === "signed_out"
            ? "登录已过期，请重新登录后再核对原操作结果。"
            : "积分调整结果暂未确认，已保留记录；请点击上方横幅核对，请勿重复提交。",
        );
      }
    },
    [message],
  );

  const resolvePending = useCallback(
    async (target?: PendingPointOperation) => {
      const op = target ?? pending;
      if (!op || busy || !operatorId || op.operator_id !== operatorId) return;
      setBusy(true);
      const res = await queryPending(op);
      if (!mountedRef.current) return;
      settle(op, res);
      if (mountedRef.current) setBusy(false);
    },
    [pending, busy, operatorId, settle],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId && canAdjust) {
      const saved = readPendingPointOperation(operatorId);
      setPending(saved);
      if (saved) void resolvePending(saved);
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId, canAdjust]);

  const handleLookup = async () => {
    const email = targetEmail.trim();
    if (!email) return;
    const seq = ++lookupSeqRef.current;
    setLookupState("loading");
    setAccount(undefined);
    setPointsDetail(undefined);
    setPointsDetailState("idle");
    setConfirmOpen(false);
    const res = await lookupConsoleAccount(email);
    if (!mountedRef.current || seq !== lookupSeqRef.current) return;
    if (res.state === "authenticated") {
      if (res.account) {
        setAccount(res.account);
        setLookupState("ready");
        void loadPointsDetail(res.account.id);
      } else {
        setLookupState("not_found");
      }
    } else if (res.state === "invalid") {
      setLookupState("invalid");
    } else {
      setLookupState("unavailable");
    }
  };

  const validAmount =
    typeof amount === "number" && Number.isSafeInteger(amount) && amount !== 0 ? amount : undefined;

  const handleConfirmSubmit = async () => {
    if (!account || validAmount === undefined || !reason.trim() || !operatorId || busy || pending) {
      return;
    }
    const key = `idem_account_points_${crypto.randomUUID()}`;
    const command: PendingPointOperation = {
      version: 1,
      operator_id: operatorId,
      operation: "point_adjustment",
      resource_id: account.id,
      expected_amount: validAmount,
      idempotency_key: key,
    };
    if (!writePendingPointOperation(command)) {
      message.error("浏览器暂不允许会话存储，操作未提交。");
      return;
    }
    setConfirmOpen(false);
    setPending(command);
    setBusy(true);
    setReceipt(undefined);
    let res: AccountPointAdjustmentWriteResult | { state: "unknown" } = await adjustAccountPoints(
      { user_id: account.id, amount: validAmount, reason: reason.trim() },
      key,
    );
    if (!mountedRef.current) return;
    if (
      res.state !== "conflict" &&
      res.state !== "signed_out" &&
      (res.state !== "succeeded" || res.result.entry.amount !== command.expected_amount)
    ) {
      res = await queryPending(command);
      if (!mountedRef.current) return;
    }
    settle(command, res);
    if (mountedRef.current) setBusy(false);
  };

  if (authState !== "authenticated" || !canAdjust) {
    return (
      <section aria-labelledby="account-points-heading">
        <div className="console-page-header">
          <h1 id="account-points-heading" className="console-page-title">
            积分调整
          </h1>
        </div>
        <ModuleStateView
          state={authState === "loading" ? "loading" : authState === "signed_out" ? "signed_out" : "denied"}
          deniedTitle="没有积分调整权限"
        />
      </section>
    );
  }

  const activeReceipt = receipt && account && receipt.resourceId === account.id ? receipt.data : undefined;

  const ledgerColumns: ColumnsType<ConsolePointLedgerEntry> = [
    {
      title: "变动",
      dataIndex: "amount",
      width: 90,
      render: (val: number) => (
        <Typography.Text strong style={{ color: val > 0 ? "#16a34a" : "#dc2626" }}>
          {val > 0 ? `+${formatNumber(val)}` : formatNumber(val)}
        </Typography.Text>
      ),
    },
    {
      title: "记账原因",
      dataIndex: "reason",
      ellipsis: true,
    },
    {
      title: "时间",
      dataIndex: "created_at",
      width: 140,
      render: (val: string) => localDateTime(val),
    },
  ];

  return (
    <section aria-labelledby="account-points-heading">
      <div className="console-page-header">
        <div>
          <h1 id="account-points-heading" className="console-page-title">
            积分调整
          </h1>
          <p className="console-page-subtitle">
            先按完整学校邮箱查找并核对用户，再手工增加或扣减积分，系统将自动通知该用户。
          </p>
        </div>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="上一笔积分调整尚未确认最终入账状态。请点击右侧核对原结果，绝不会重复增减积分。"
        busy={busy}
        onResolve={() => void resolvePending()}
      />

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card title="1. 查找用户并填写变动">
            <Space orientation="vertical" size={16} style={{ width: "100%" }}>
              <div>
                <Typography.Text style={{ display: "block", marginBottom: 6 }}>
                  学生完整邮箱
                </Typography.Text>
                <Space.Compact style={{ width: "100%" }}>
                  <Input
                    type="email"
                    placeholder="student@henu.edu.cn"
                    value={targetEmail}
                    onChange={(e) => {
                      setTargetEmail(e.target.value);
                      setAccount(undefined);
                      setLookupState("idle");
                    }}
                    onPressEnter={() => void handleLookup()}
                  />
                  <Button
                    type="primary"
                    icon={<SearchOutlined />}
                    loading={lookupState === "loading"}
                    disabled={!targetEmail.trim()}
                    onClick={() => void handleLookup()}
                  >
                    查找
                  </Button>
                </Space.Compact>
              </div>

              {lookupState === "not_found" && (
                <Alert
                  type="warning"
                  showIcon
                  title="未找到该邮箱对应的账户"
                  description="请核对邮箱拼写是否完整（目前仅支持 @henu.edu.cn 校园邮箱）。"
                />
              )}
              {lookupState === "invalid" && (
                <Alert
                  type="error"
                  showIcon
                  title="邮箱格式不正确，请输入完整的 @henu.edu.cn 校园邮箱。"
                />
              )}
              {lookupState === "unavailable" && (
                <Alert type="error" showIcon title="账户查找暂时不可用，请稍后再试。" />
              )}

              {account && (
                <Alert
                  type="success"
                  showIcon
                  title={
                    <span>
                      已核对用户：<strong>{accountName(account.display_name)}</strong>（
                      {account.email}）
                    </span>
                  }
                  description={`账户状态：${accountStatusLabel(account.status)}`}
                />
              )}

              <div>
                <Typography.Text style={{ display: "block", marginBottom: 6 }}>
                  积分增减值（正数增加，负数扣减）
                </Typography.Text>
                <InputNumber
                  style={{ width: "100%" }}
                  precision={0}
                  placeholder="例如：50 或 -20"
                  disabled={!account || busy || Boolean(pending)}
                  value={amount}
                  onChange={(val) => setAmount(val)}
                />
                <Space size={6} wrap style={{ marginTop: 8 }}>
                  {QUICK_AMOUNTS.map((n) => (
                    <Button
                      key={n}
                      size="small"
                      disabled={!account || busy || Boolean(pending)}
                      onClick={() => setAmount(n)}
                    >
                      {n > 0 ? `+${n}` : n}
                    </Button>
                  ))}
                </Space>
              </div>

              <div>
                <Typography.Text style={{ display: "block", marginBottom: 6 }}>
                  调整原因
                </Typography.Text>
                <Input.TextArea
                  rows={3}
                  maxLength={1000}
                  showCount
                  style={{ marginBottom: 14 }}
                  disabled={!account || busy || Boolean(pending)}
                  placeholder="例如：纠错采纳奖励 / 活动发放 / 误发冲正"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              </div>

              <Button
                type="primary"
                icon={<WalletOutlined />}
                loading={busy}
                disabled={!account || validAmount === undefined || !reason.trim() || Boolean(pending)}
                onClick={() => setConfirmOpen(true)}
              >
                提交积分调整
              </Button>
            </Space>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card title="2. 当前余额、近期流水与记账回执" data-account-lookup-state={lookupState}>
            {!account ? (
              <Empty
                description="在「查找用户并填写变动」中查询并核对用户后，这里将展示当前积分余额、近期流水以及调整后的记账回执。"
                style={{ padding: "48px 0" }}
              />
            ) : (
              <Space orientation="vertical" size={16} style={{ width: "100%" }}>
                {activeReceipt && (
                  <Alert
                    type="success"
                    showIcon
                    icon={<CheckCircleOutlined />}
                    title={`积分调整已成功入账（${activeReceipt.entry.amount > 0 ? "+" : ""}${formatNumber(activeReceipt.entry.amount)} 分）`}
                    description={`记账原因：${activeReceipt.entry.reason} · 入账时间：${localDateTimeSeconds(activeReceipt.entry.created_at)}`}
                  />
                )}

                <Descriptions bordered size="small" column={1}>
                  <Descriptions.Item label="用户姓名">
                    {accountName(account.display_name)}
                  </Descriptions.Item>
                  <Descriptions.Item label="学校邮箱">{account.email}</Descriptions.Item>
                  <Descriptions.Item label="账户状态">
                    <Tag color={account.status === "active" ? "success" : "warning"}>
                      {accountStatusLabel(account.status)}
                    </Tag>
                  </Descriptions.Item>
                </Descriptions>

                <Row gutter={16}>
                  <Col span={12}>
                    <Statistic
                      title="当前真实余额"
                      value={
                        activeReceipt
                          ? formatNumber(activeReceipt.balance)
                          : pointsDetailState === "ready" && pointsDetail
                            ? formatNumber(pointsDetail.balance)
                            : pointsDetailState === "loading"
                              ? "读取中…"
                              : "暂不可读"
                      }
                      suffix={activeReceipt || pointsDetailState === "ready" ? "分" : undefined}
                    />
                  </Col>
                  {activeReceipt && (
                    <Col span={12}>
                      <Statistic
                        title="本次变动"
                        value={`${activeReceipt.entry.amount > 0 ? "+" : ""}${formatNumber(activeReceipt.entry.amount)}`}
                        styles={{
                          content: {
                            color: activeReceipt.entry.amount > 0 ? "#16a34a" : "#dc2626",
                          },
                        }}
                      />
                    </Col>
                  )}
                </Row>

                <Divider style={{ margin: "4px 0" }} />

                <div>
                  <Typography.Text strong style={{ display: "block", marginBottom: 8 }}>
                    近期积分流水（最多 20 条）
                  </Typography.Text>
                  {pointsDetailState === "loading" ? (
                    <div style={{ textAlign: "center", padding: "24px 0" }}>
                      <Spin />
                    </div>
                  ) : pointsDetailState === "ready" && pointsDetail ? (
                    <Table
                      rowKey="id"
                      size="small"
                      columns={ledgerColumns}
                      dataSource={pointsDetail.entries}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 5, showSizeChanger: false }}
                      locale={{ emptyText: "该账户暂无历史积分变动记录" }}
                    />
                  ) : (
                    <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                      暂未读取到历史流水；核对姓名与邮箱无误后仍可提交积分调整。
                    </Typography.Text>
                  )}
                </div>
              </Space>
            )}
          </Card>
        </Col>
      </Row>

      <Modal
        open={confirmOpen}
        title="确认执行这笔积分调整？"
        okText="确认入账"
        cancelText="取消"
        okButtonProps={{ danger: (validAmount ?? 0) < 0 }}
        confirmLoading={busy}
        onOk={() => void handleConfirmSubmit()}
        onCancel={() => setConfirmOpen(false)}
      >
        {account && validAmount !== undefined && (
          <Space orientation="vertical" size={10} style={{ width: "100%", marginTop: 8 }}>
            <Typography.Text>
              即将向 <strong>{accountName(account.display_name)}</strong>（{account.email}）
              <strong>
                {validAmount > 0
                  ? `增加 ${formatNumber(validAmount)}`
                  : `扣减 ${formatNumber(Math.abs(validAmount))}`}{" "}
                积分
              </strong>
              。
            </Typography.Text>
            <Typography.Text type="secondary">调整原因：{reason.trim()}</Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              积分流水一经入账不可撤回（如填错需做反向调整），系统将同时通知该用户。
            </Typography.Text>
          </Space>
        )}
      </Modal>
    </section>
  );
}
