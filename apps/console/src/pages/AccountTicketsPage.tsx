import {
  CheckCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  SendOutlined,
} from "@ant-design/icons";
import {
  App as AntApp,
  Button,
  Card,
  Col,
  Divider,
  Empty,
  Input,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ModuleStateView } from "@/components/ModuleStateView";
import { PendingRecoveryAlert } from "@/components/PendingRecoveryAlert";
import { useConsole } from "@/lib/console-context";
import {
  fetchAccountTicket,
  fetchAccountTicketFacts,
  replyToAccountTicket,
  resolveAccountTicketOperation,
  transitionAccountTicket,
  type AccountTicketWriteResult,
  type ConsoleAccountTicket,
  type ConsoleAccountTicketDetail,
  type ConsoleAccountTicketFacts,
  type ConsoleOperatorReplyRequest,
  type ConsoleTicketFactReference,
  type ConsoleTicketFactsFilter,
  type ConsoleTicketTransitionRequest,
} from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import {
  clearPendingAccountTicketOperation,
  readPendingAccountTicketOperation,
  writePendingAccountTicketOperation,
  type PendingAccountTicketOperation,
} from "@/lib/pending-operations";

const VALID_FILTERS: ConsoleTicketFactsFilter[] = [
  "all",
  "unresolved",
  "open",
  "in_progress",
  "resolved",
];

function ticketStatusTag(status: ConsoleAccountTicket["status"]) {
  if (status === "open") return <Tag color="error">待处理</Tag>;
  if (status === "in_progress") return <Tag color="processing">处理中</Tag>;
  return <Tag color="success">已解决</Tag>;
}

function formatShortTicketRef(reference: string): string {
  const match = /^([A-Z]+-)([0-9a-f]{8})-[0-9a-f-]{27}$/i.exec(reference.trim());
  if (match) {
    return `${match[1]}${match[2]}…`;
  }
  if (reference.length > 20) {
    return `${reference.slice(0, 14)}…`;
  }
  return reference;
}

export function AccountTicketsPage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId, permissions } = useConsole();
  const [searchParams, setSearchParams] = useSearchParams();

  const initialStatus = (() => {
    const val = searchParams.get("status") as ConsoleTicketFactsFilter | null;
    return val && VALID_FILTERS.includes(val) ? val : "unresolved";
  })();

  const [statusFilter, setStatusFilter] = useState<ConsoleTicketFactsFilter>(initialStatus);
  const [searchQuery, setSearchQuery] = useState("");
  const [workspaceState, setWorkspaceState] = useState<
    "loading" | "ready" | "denied" | "unavailable"
  >("loading");
  const [queueFacts, setQueueFacts] = useState<ConsoleAccountTicketFacts>();
  const [queue, setQueue] = useState<ConsoleTicketFactReference[]>([]);

  const [selectedId, setSelectedId] = useState("");
  const [detailState, setDetailState] = useState<
    "idle" | "loading" | "ready" | "not_found" | "unavailable"
  >("idle");
  const [detail, setDetail] = useState<ConsoleAccountTicketDetail>();
  const [replyBody, setReplyBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingAccountTicketOperation | undefined>();

  const draftsRef = useRef(new Map<string, string>());
  const mountedRef = useRef(true);
  const queueSeqRef = useRef(0);
  const detailSeqRef = useRef(0);

  const canRead = permissions.includes("account.tickets.read");
  const canReply = permissions.includes("account.tickets.reply");
  const canTransition = permissions.includes("account.tickets.transition");
  const canWrite = !busy && !pending && Boolean(operatorId) && authState === "authenticated";

  const refreshQueue = useCallback(
    async (filter: ConsoleTicketFactsFilter = statusFilter) => {
      if (authState !== "authenticated" || !canRead) {
        setWorkspaceState(authState === "loading" ? "loading" : "denied");
        return;
      }
      const seq = ++queueSeqRef.current;
      setWorkspaceState("loading");
      const res = await fetchAccountTicketFacts(filter);
      if (!mountedRef.current || seq !== queueSeqRef.current) return;
      if (res.state === "authenticated") {
        setQueueFacts(res.facts);
        setQueue(res.facts.window.items);
        setWorkspaceState("ready");
      } else {
        setQueue([]);
        setQueueFacts(undefined);
        setWorkspaceState(res.state === "denied" ? "denied" : "unavailable");
      }
    },
    [authState, canRead, statusFilter],
  );

  const openTicket = useCallback(
    async (ticketId: string) => {
      if (authState !== "authenticated" || !canRead) return;
      const seq = ++detailSeqRef.current;
      if (selectedId) draftsRef.current.set(selectedId, replyBody);
      setSelectedId(ticketId);
      setReplyBody(draftsRef.current.get(ticketId) ?? "");
      setDetail(undefined);
      setDetailState("loading");
      const res = await fetchAccountTicket(ticketId);
      if (!mountedRef.current || seq !== detailSeqRef.current) return;
      if (res.state === "authenticated" && res.ticket.ticket.id === ticketId) {
        setDetail(res.ticket);
        setDetailState("ready");
      } else {
        setDetailState(res.state === "not_found" ? "not_found" : "unavailable");
      }
    },
    [authState, canRead, selectedId, replyBody],
  );

  const clearPending = useCallback(() => {
    setPending(undefined);
    clearPendingAccountTicketOperation();
  }, []);

  const finishResult = useCallback(
    async (
      command: PendingAccountTicketOperation,
      result: AccountTicketWriteResult | { state: "unknown" },
    ) => {
      if (result.state === "succeeded") {
        if (
          result.ticket.id !== command.resource_id ||
          result.ticket.version !== command.expected_version + 1
        ) {
          message.warning("返回结果与原工单不一致，请点击上方按钮再次核对。");
          return;
        }
        clearPending();
        if (command.operation === "ticket_reply") {
          draftsRef.current.delete(command.resource_id);
          if (selectedId === command.resource_id) setReplyBody("");
          message.success("回复已发送。");
        } else {
          message.success(
            result.ticket.status === "in_progress" ? "工单已标记为处理中。" : "工单已标记为已解决。",
          );
        }
        await refreshQueue();
        if (mountedRef.current) await openTicket(command.resource_id);
      } else if (result.state === "conflict") {
        clearPending();
        message.warning("工单刚刚有新动态，已为你保留草稿并刷新最新内容，请核对后再提交。");
        await refreshQueue();
        if (mountedRef.current && selectedId === command.resource_id) {
          await openTicket(command.resource_id);
        }
      } else if (result.state === "unknown" || result.state === "unavailable") {
        message.warning("操作结果暂未确认，已保留记录；请点击上方横幅查询，勿重复提交。");
      } else {
        message.error(
          result.state === "denied"
            ? "当前账户缺少该工单操作权限。"
            : result.state === "signed_out"
              ? "登录状态已过期，请重新登录。"
              : "暂时无法核对原操作结果，请稍后再试。",
        );
      }
    },
    [clearPending, message, openTicket, refreshQueue, selectedId],
  );

  const resolvePending = useCallback(
    async (target?: PendingAccountTicketOperation) => {
      const op = target ?? pending;
      if (!op || busy || !operatorId || op.operator_id !== operatorId) return;
      setBusy(true);
      const res = await resolveAccountTicketOperation(op.operation, op.idempotency_key);
      if (!mountedRef.current) return;
      await finishResult(op, res);
      if (mountedRef.current) setBusy(false);
    },
    [pending, busy, operatorId, finishResult],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId && canRead) {
      const saved = readPendingAccountTicketOperation(operatorId);
      setPending(saved);
      void refreshQueue(statusFilter);
      if (saved) void resolvePending(saved);
    } else {
      setWorkspaceState(authState === "loading" ? "loading" : "denied");
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId, canRead]);

  const handleStatusChange = (next: ConsoleTicketFactsFilter) => {
    setStatusFilter(next);
    setSelectedId("");
    setDetail(undefined);
    setDetailState("idle");
    const params = new URLSearchParams(searchParams);
    params.set("status", next);
    setSearchParams(params, { replace: true });
    void refreshQueue(next);
  };

  const submitCommand = async (
    operation: PendingAccountTicketOperation["operation"],
    input: ConsoleOperatorReplyRequest | ConsoleTicketTransitionRequest,
  ) => {
    const current = detail?.ticket;
    if (!current || !operatorId || !canWrite) return;
    const key = `idem_account_${operation === "ticket_reply" ? "reply" : "transition"}_${crypto.randomUUID()}`;
    const command: PendingAccountTicketOperation = {
      version: 1,
      operator_id: operatorId,
      operation,
      resource_id: current.id,
      expected_version: current.version,
      idempotency_key: key,
    };
    if (!writePendingAccountTicketOperation(command)) {
      message.error("浏览器暂不允许会话存储，无法安全提交，请检查设置后重试。");
      return;
    }
    setPending(command);
    setBusy(true);
    let res: AccountTicketWriteResult | { state: "unknown" } =
      operation === "ticket_reply"
        ? await replyToAccountTicket(current.id, input as ConsoleOperatorReplyRequest, key)
        : await transitionAccountTicket(current.id, input as ConsoleTicketTransitionRequest, key);
    if (!mountedRef.current) return;
    if (res.state !== "succeeded" && res.state !== "conflict") {
      res = await resolveAccountTicketOperation(operation, key);
      if (!mountedRef.current) return;
    }
    await finishResult(command, res);
    if (mountedRef.current) setBusy(false);
  };

  const handleSendReply = async () => {
    const body = replyBody.trim();
    if (!body || !canReply || !detail || detail.ticket.status === "resolved") return;
    await submitCommand("ticket_reply", { body, expected_version: detail.ticket.version });
  };

  const handleTransition = async (status: "in_progress" | "resolved") => {
    if (!canTransition || !detail || detail.ticket.status === "resolved") return;
    await submitCommand("ticket_transition", { status, expected_version: detail.ticket.version });
  };

  const visibleTickets = useMemo(() => {
    const q = searchQuery.trim().toLocaleLowerCase();
    return queue.filter(
      (item) =>
        !q ||
        item.title.toLocaleLowerCase().includes(q) ||
        item.reference.toLocaleLowerCase().includes(q),
    );
  }, [queue, searchQuery]);

  return (
    <section aria-labelledby="account-tickets-heading">
      <div className="console-page-header">
        <div>
          <h1 id="account-tickets-heading" className="console-page-title">
            客服工单
          </h1>
          <p className="console-page-subtitle">
            查看并回复学生提交的账户与使用问题工单，跟进处理状态直至解决。
          </p>
        </div>
        <Button
          icon={<ReloadOutlined />}
          loading={workspaceState === "loading"}
          onClick={() => void refreshQueue()}
        >
          刷新队列
        </Button>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="上一条工单回复或状态修改尚未收到确认。点击右侧查询原结果，避免重复发送回复。"
        busy={busy}
        onResolve={() => void resolvePending()}
      />

      {workspaceState === "denied" || workspaceState === "unavailable" ? (
        <ModuleStateView
          state={workspaceState}
          deniedTitle="没有客服工单查看权限"
          onRetry={() => void refreshQueue()}
        />
      ) : (
        <>
          {queueFacts && (
            <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
              <Col xs={12} sm={6}>
                <Card size="small">
                  <Statistic
                    title="未解决合计"
                    value={queueFacts.counts.unresolved}
                    styles={{
                      content: {
                        color: queueFacts.counts.unresolved > 0 ? "#dc2626" : "#15803d",
                      },
                    }}
                  />
                </Card>
              </Col>
              <Col xs={12} sm={6}>
                <Card size="small">
                  <Statistic title="待处理" value={queueFacts.counts.open} />
                </Card>
              </Col>
              <Col xs={12} sm={6}>
                <Card size="small">
                  <Statistic title="处理中" value={queueFacts.counts.in_progress} />
                </Card>
              </Col>
              <Col xs={12} sm={6}>
                <Card size="small">
                  <Statistic title="已解决" value={queueFacts.counts.resolved} />
                </Card>
              </Col>
            </Row>
          )}

          <Row gutter={[16, 16]}>
            {/* 左栏：工单队列 */}
            <Col xs={24} lg={9}>
              <Card
                title="工单队列"
                extra={
                  queueFacts ? (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      共匹配 {queueFacts.window.matched_total} 条
                    </Typography.Text>
                  ) : null
                }
              >
                <Space orientation="vertical" size={10} style={{ width: "100%", marginBottom: 12 }}>
                  <Select
                    aria-label="工单状态"
                    style={{ width: "100%" }}
                    value={statusFilter}
                    onChange={handleStatusChange}
                    options={[
                      { label: "未解决（待处理 + 处理中）", value: "unresolved" },
                      { label: "待处理", value: "open" },
                      { label: "处理中", value: "in_progress" },
                      { label: "已解决", value: "resolved" },
                      { label: "全部状态", value: "all" },
                    ]}
                  />
                  <Input
                    allowClear
                    prefix={<SearchOutlined />}
                    placeholder="按工单标题或编号筛选…"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                  />
                </Space>

                {workspaceState === "loading" ? (
                  <div style={{ textAlign: "center", padding: "36px 0" }}>
                    <Spin />
                  </div>
                ) : visibleTickets.length === 0 ? (
                  <Empty description="当前筛选下暂无工单" style={{ padding: "32px 0" }} />
                ) : (
                  <div
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 8,
                      maxHeight: 540,
                      overflowY: "auto",
                    }}
                  >
                    {visibleTickets.map((ticket) => {
                      const active = selectedId === ticket.id;
                      return (
                        <button
                          key={ticket.id}
                          type="button"
                          onClick={() => void openTicket(ticket.id)}
                          style={{
                            width: "100%",
                            textAlign: "left",
                            cursor: "pointer",
                            padding: "12px 12px",
                            borderRadius: 8,
                            border: active ? "1px solid #18181b" : "1px solid #e5e7eb",
                            background: active ? "#f8fafc" : "#ffffff",
                            font: "inherit",
                          }}
                        >
                          <div
                            style={{
                              display: "flex",
                              justifyContent: "space-between",
                              alignItems: "flex-start",
                              gap: 8,
                            }}
                          >
                            <Typography.Text strong style={{ wordBreak: "break-word" }}>
                              {ticket.title}
                            </Typography.Text>
                            {ticketStatusTag(ticket.status)}
                          </div>
                          <div
                            style={{
                              marginTop: 4,
                              fontSize: 12,
                              color: "#64748b",
                              display: "flex",
                              justifyContent: "space-between",
                              flexWrap: "wrap",
                              gap: 4,
                            }}
                          >
                            <Tooltip title={ticket.reference}>
                              <span className="console-mono">
                                {formatShortTicketRef(ticket.reference)}
                              </span>
                            </Tooltip>
                            <span>{localDateTime(ticket.updated_at)}</span>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                )}
              </Card>
            </Col>

            {/* 右栏：工单详情与对话 */}
            <Col xs={24} lg={15}>
              <Card data-account-ticket-detail-state={detailState}>
                {detailState === "idle" && (
                  <Empty
                    description="在工单队列中选择一条工单，即可查看对话记录、回复用户并更新处理状态"
                    style={{ padding: "64px 0" }}
                  />
                )}
                {detailState === "loading" && (
                  <div style={{ textAlign: "center", padding: "64px 0" }}>
                    <Spin />
                  </div>
                )}
                {detailState === "not_found" && (
                  <Empty description="该工单已不存在或无权查看，请刷新工单队列" />
                )}
                {detailState === "unavailable" && (
                  <ModuleStateView
                    state="unavailable"
                    unavailableTitle="工单详情暂时加载失败"
                    onRetry={() => selectedId && void openTicket(selectedId)}
                  />
                )}
                {detailState === "ready" && detail && (
                  <div>
                    <div
                      style={{
                        display: "flex",
                        flexWrap: "wrap",
                        justifyContent: "space-between",
                        alignItems: "flex-start",
                        gap: 12,
                      }}
                    >
                      <div>
                        <Space size={8} style={{ marginBottom: 4 }}>
                          <Tooltip title={detail.ticket.reference}>
                            <Typography.Text
                              copyable={{ text: detail.ticket.reference }}
                              className="console-code-chip"
                              style={{ margin: 0 }}
                            >
                              {formatShortTicketRef(detail.ticket.reference)}
                            </Typography.Text>
                          </Tooltip>
                          {ticketStatusTag(detail.ticket.status)}
                        </Space>
                        <Typography.Title level={4} style={{ margin: "4px 0" }}>
                          {detail.ticket.title}
                        </Typography.Title>
                        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                          提交人：{detail.ticket.display_name || "未设置姓名"} ·{" "}
                          {detail.ticket.email || "邮箱不可用"} · 更新于{" "}
                          {localDateTime(detail.ticket.updated_at)}
                        </Typography.Text>
                      </div>

                      {canTransition && detail.ticket.status !== "resolved" && (
                        <Space wrap>
                          {detail.ticket.status === "open" && (
                            <Button
                              icon={<PlayCircleOutlined />}
                              disabled={!canWrite}
                              loading={busy}
                              onClick={() => void handleTransition("in_progress")}
                            >
                              开始处理
                            </Button>
                          )}
                          <Button
                            type="primary"
                            icon={<CheckCircleOutlined />}
                            disabled={!canWrite}
                            loading={busy}
                            onClick={() => void handleTransition("resolved")}
                          >
                            标记已解决
                          </Button>
                        </Space>
                      )}
                    </div>

                    <Divider style={{ margin: "16px 0" }} />

                    {/* 对话记录 */}
                    <Space orientation="vertical" size={12} style={{ width: "100%" }}>
                      {detail.messages.map((msg) => (
                        <div
                          key={msg.id}
                          className={
                            msg.author_kind === "operator"
                              ? "console-ticket-bubble-operator"
                              : "console-ticket-bubble-user"
                          }
                        >
                          <div
                            style={{
                              display: "flex",
                              justifyContent: "space-between",
                              fontSize: 12,
                              color: "#64748b",
                              marginBottom: 6,
                            }}
                          >
                            <strong>{msg.author_kind === "operator" ? "客服回复" : "学生提问"}</strong>
                            <span>{localDateTime(msg.created_at)}</span>
                          </div>
                          <div style={{ whiteSpace: "pre-wrap", lineHeight: 1.65 }}>
                            {msg.body}
                          </div>
                        </div>
                      ))}
                    </Space>

                    <Divider style={{ margin: "18px 0" }} />

                    {canReply && detail.ticket.status !== "resolved" ? (
                      <Space orientation="vertical" size={10} style={{ width: "100%" }}>
                        <Input.TextArea
                          aria-label="客服回复"
                          rows={4}
                          maxLength={5000}
                          showCount
                          style={{ marginBottom: 18 }}
                          placeholder="输入客服回复内容（按 Cmd/Ctrl + Enter 快捷发送）…"
                          value={replyBody}
                          onChange={(e) => setReplyBody(e.target.value)}
                          onKeyDown={(e) => {
                            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                              e.preventDefault();
                              void handleSendReply();
                            }
                          }}
                        />
                        <div>
                          <Button
                            type="primary"
                            icon={<SendOutlined />}
                            loading={busy}
                            disabled={!canWrite || !replyBody.trim()}
                            onClick={() => void handleSendReply()}
                          >
                            发送回复
                          </Button>
                        </div>
                      </Space>
                    ) : detail.ticket.status === "resolved" ? (
                      <Typography.Text type="secondary">
                        该工单已解决。如果学生后续追问，工单将自动重新打开。
                      </Typography.Text>
                    ) : (
                      <Typography.Text type="secondary">
                        当前账户仅有只读权限，无法回复或修改工单状态。
                      </Typography.Text>
                    )}
                  </div>
                )}
              </Card>
            </Col>
          </Row>
        </>
      )}
    </section>
  );
}
