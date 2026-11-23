import {
  CheckOutlined,
  CloseOutlined,
  EyeOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  SendOutlined,
} from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Divider,
  Drawer,
  Form,
  Input,
  Modal,
  Segmented,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ModuleStateView } from "@/components/ModuleStateView";
import { PendingRecoveryAlert } from "@/components/PendingRecoveryAlert";
import { useConsole } from "@/lib/console-context";
import {
  fetchNoticeSources,
  type ConsoleNoticeSourceDetail,
} from "@/lib/console-extensions";
import {
  createNoticeSource,
  createNoticeVersion,
  distributeNoticeVersion,
  fetchNoticeSnapshot,
  resolveNoticeOperation,
  reviewNoticeVersion,
  type NoticeAudience,
  type NoticeSnapshot,
  type NoticeVersion,
  type NoticeWriteResult,
} from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import {
  clearPendingNoticeOperation,
  readPendingNoticeOperation,
  writePendingNoticeOperation,
  type PendingNoticeOperation,
} from "@/lib/pending-operations";

const UUID_RE = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i;
const VALID_FILTERS = ["pending_review", "approved", "distributed", "rejected", "all"] as const;
type NoticeFilter = (typeof VALID_FILTERS)[number];

function stateTag(state: NoticeVersion["state"]) {
  switch (state) {
    case "pending_review":
      return <Tag color="processing">待审核</Tag>;
    case "approved":
      return <Tag color="success">已通过</Tag>;
    case "rejected":
      return <Tag color="error">已拒绝</Tag>;
    case "distributed":
      return <Tag color="cyan">已提交分发</Tag>;
    default:
      return <Tag>{state}</Tag>;
  }
}

function distributionStatusText(status?: NoticeVersion["distribution_status"]) {
  switch (status) {
    case "delivered":
      return "最近一次分发已送达";
    case "failed":
      return "最近一次分发失败";
    case "processing":
      return "分发任务正在发送中";
    case "queued":
      return "已进入分发队列，尚未确认送达";
    default:
      return "等待同步分发状态";
  }
}

function channelLabel(value: "in_app" | "email"): string {
  return value === "in_app" ? "站内消息" : "校园邮件";
}

function audienceLabel(value: NoticeAudience): string {
  if (value.kind === "all_students") return "全体学生";
  if (value.kind === "college") return `指定学院：${value.value ?? ""}`;
  if (value.kind === "role") return `指定角色：${value.value ?? ""}`;
  return "未指定受众";
}

function confirmedOperation(
  command: PendingNoticeOperation,
  result: Record<string, unknown>,
): boolean {
  if (command.operation === "source_create") {
    return (
      typeof result.id === "string" &&
      UUID_RE.test(result.id) &&
      result.code === command.resource_id &&
      typeof result.name === "string" &&
      result.name.trim() !== ""
    );
  }
  if (command.operation === "version_create") {
    return (
      typeof result.id === "string" &&
      UUID_RE.test(result.id) &&
      result.source_id === command.resource_id &&
      result.state === "pending_review"
    );
  }
  if (command.operation === "review") {
    return (
      result.version_id === command.resource_id &&
      ["approved", "rejected"].includes(String(result.state))
    );
  }
  return result.version_id === command.resource_id && result.status === "queued";
}

export function NoticePage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId, permissions } = useConsole();
  const [searchParams, setSearchParams] = useSearchParams();

  const initialFilter =
    VALID_FILTERS.find((v) => v === searchParams.get("state")) ?? "pending_review";
  const initialSelected = (() => {
    const val = searchParams.get("selected");
    return val && UUID_RE.test(val) ? val : "";
  })();

  const [stateFilter, setStateFilter] = useState<NoticeFilter>(initialFilter);
  const [keyword, setKeyword] = useState("");
  const [selectedId, setSelectedId] = useState<string>(initialSelected);
  const [state, setState] = useState<
    "loading" | "ready" | "stale" | "denied" | "not_configured" | "unavailable"
  >("loading");
  const [snapshot, setSnapshot] = useState<NoticeSnapshot>();
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingNoticeOperation | undefined>();

  // 弹窗/抽屉
  const [sourceModalOpen, setSourceModalOpen] = useState(false);
  const [versionDrawerOpen, setVersionDrawerOpen] = useState(false);
  const [sourceForm] = Form.useForm();
  const [versionForm] = Form.useForm();

  // 审核与分发控制
  const [reviewReason, setReviewReason] = useState("");
  const [channel, setChannel] = useState<"in_app" | "email">("in_app");
  const [audienceKind, setAudienceKind] = useState<"all_students" | "college" | "role">(
    "all_students",
  );
  const [audienceValue, setAudienceValue] = useState("");
  const [confirmDist, setConfirmDist] = useState<{
    item: NoticeVersion;
    channel: "in_app" | "email";
    audience: { kind: "all_students" | "college" | "role"; value?: string };
  } | null>(null);

  const [createdSources, setCreatedSources] = useState<ConsoleNoticeSourceDetail[]>([]);
  const [directorySources, setDirectorySources] = useState<ConsoleNoticeSourceDetail[]>([]);
  const [activeTab, setActiveTab] = useState<"versions" | "sources">("versions");

  const mountedRef = useRef(true);
  const readSeqRef = useRef(0);

  const canManage = permissions.includes("notice.manage");
  const canReview = permissions.includes("notice.review");
  const canDistribute = permissions.includes("notice.distribute");

  const syncQuery = useCallback(
    (nextFilter: NoticeFilter, nextSelected: string) => {
      const params = new URLSearchParams(searchParams);
      params.set("state", nextFilter);
      if (nextSelected) params.set("selected", nextSelected);
      else params.delete("selected");
      setSearchParams(params, { replace: true });
    },
    [searchParams, setSearchParams],
  );

  const refresh = useCallback(async () => {
    if (authState !== "authenticated") {
      setState(authState === "denied" ? "denied" : "unavailable");
      return;
    }
    const seq = ++readSeqRef.current;
    setState((prev) => (prev === "ready" ? "ready" : "loading"));
    const [res, sourcesRes] = await Promise.all([fetchNoticeSnapshot(), fetchNoticeSources()]);
    if (!mountedRef.current || seq !== readSeqRef.current) return;
    if (sourcesRes.state === "authenticated") {
      setDirectorySources(sourcesRes.data);
    }
    if (res.state === "authenticated") {
      setSnapshot(res.snapshot);
      setState("ready");
    } else if (res.state === "unavailable" && snapshot) {
      setState("stale");
    } else {
      setSnapshot(undefined);
      setState(
        res.state === "denied"
          ? "denied"
          : res.state === "not_configured"
            ? "not_configured"
            : "unavailable",
      );
    }
  }, [authState, snapshot]);

  const clearPending = useCallback(() => {
    setPending(undefined);
    clearPendingNoticeOperation();
  }, []);

  const resolvePending = useCallback(
    async (target?: PendingNoticeOperation) => {
      const op = target ?? pending;
      if (!op || busy) return;
      setBusy(true);
      const res = await resolveNoticeOperation(op.operation, op.idempotency_key);
      if (!mountedRef.current) return;
      if (res.state === "succeeded" && confirmedOperation(op, res.result)) {
        clearPending();
        message.success(
          op.operation === "distribution"
            ? "分发任务已确认进入队列。"
            : op.operation === "review"
              ? `审核已完成（${res.result.state === "approved" ? "已批准" : "已拒绝"}）。`
              : "之前的操作已确认完成。",
        );
        await refresh();
      } else {
        message.warning(
          res.state === "denied"
            ? "当前账户无法查询原操作结果，请联系管理员。"
            : "结果暂未确认，已保留记录；请稍后再查，请勿重复提交。",
        );
      }
      if (mountedRef.current) setBusy(false);
    },
    [pending, busy, clearPending, message, refresh],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId) {
      const saved = readPendingNoticeOperation(operatorId);
      setPending(saved);
      void refresh();
      if (saved) void resolvePending(saved);
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId]);

  // 优先使用后端真实来源目录（GET /api/v1/notices/sources），再补齐本次新建与历史版本来源
  const mergedSources = useMemo<ConsoleNoticeSourceDetail[]>(() => {
    const map = new Map<string, ConsoleNoticeSourceDetail>();
    for (const s of directorySources) map.set(s.id, s);
    for (const s of createdSources) {
      if (!map.has(s.id)) map.set(s.id, s);
    }
    for (const item of snapshot?.items ?? []) {
      if (item.source?.id && !map.has(item.source.id)) {
        map.set(item.source.id, {
          id: item.source.id,
          code: item.source.code,
          name: item.source.name,
        });
      }
    }
    return Array.from(map.values());
  }, [directorySources, createdSources, snapshot]);

  const knownSources = useMemo(
    () =>
      mergedSources.map((s) => ({
        value: s.id,
        label: `${s.name}（${s.code}）`,
      })),
    [mergedSources],
  );

  const executeNoticeOp = async (
    operation: PendingNoticeOperation["operation"],
    resourceId: string,
    call: (key: string) => Promise<NoticeWriteResult>,
    successMsg: string,
    distribution?: PendingNoticeOperation["distribution"],
    onSuccess?: (res: Record<string, unknown>) => void,
  ): Promise<boolean> => {
    if (!operatorId || busy || pending) return false;
    const key = `idem_notice_${operation}_${crypto.randomUUID()}`;
    const targetLabel = {
      source_create: "登记通知来源",
      version_create: "创建通知版本",
      review: "通知审核",
      distribution: "通知分发",
    }[operation];
    const command: PendingNoticeOperation = {
      version: 1,
      operator_id: operatorId,
      operation,
      idempotency_key: key,
      resource_id: resourceId,
      target_label: targetLabel,
      ...(distribution ? { distribution } : {}),
    };
    if (!writePendingNoticeOperation(command)) {
      message.error("浏览器暂不允许会话存储，无法安全提交，请检查浏览器设置后重试。");
      return false;
    }
    setPending(command);
    setBusy(true);

    let res = await call(key);
    if (!mountedRef.current) return false;
    if (res.state === "conflict" || res.state === "invalid" || res.state === "denied") {
      clearPending();
      setBusy(false);
      message.error(
        res.state === "conflict"
          ? "该通知刚刚被其他人更新，已为你刷新最新内容。"
          : res.state === "denied"
            ? "当前账户缺少执行该操作的权限。"
            : "提交内容不符合格式要求，请检查后重试。",
      );
      await refresh();
      return false;
    }
    if (res.state !== "succeeded" || !confirmedOperation(command, res.result)) {
      res = await resolveNoticeOperation(command.operation, command.idempotency_key);
      if (!mountedRef.current) return false;
    }
    if (res.state === "succeeded" && confirmedOperation(command, res.result)) {
      clearPending();
      onSuccess?.(res.result);
      message.success(successMsg);
      setBusy(false);
      await refresh();
      return true;
    }
    setBusy(false);
    message.warning("操作结果暂未确认，已保留记录；请点击上方横幅核对，请勿重复提交。");
    return false;
  };

  const handleCreateSource = async () => {
    const values = await sourceForm.validateFields();
    const ok = await executeNoticeOp(
      "source_create",
      String(values.code).trim(),
      (key) =>
        createNoticeSource(
          {
            code: String(values.code).trim(),
            name: String(values.name).trim(),
            canonical_url: String(values.canonical_url).trim(),
          },
          key,
        ),
      "通知来源已登记成功。",
      undefined,
      (res) => {
        if (typeof res.id === "string") {
          setCreatedSources((prev) => [
            {
              id: String(res.id),
              code: String(values.code).trim(),
              name: String(values.name).trim(),
              canonical_url: String(values.canonical_url).trim(),
            },
            ...prev,
          ]);
          versionForm.setFieldValue("source_id", res.id);
        }
      },
    );
    if (ok) {
      sourceForm.resetFields();
      setSourceModalOpen(false);
    }
  };

  const handleCreateVersion = async () => {
    const values = await versionForm.validateFields();
    const sourceId = String(values.source_id).trim();
    if (!UUID_RE.test(sourceId)) {
      message.error("来源 ID 须为有效的 UUID，请从下拉列表选择或粘贴正确的来源 ID。");
      return;
    }
    const publishedAt = values.source_published_at
      ? new Date(String(values.source_published_at)).toISOString()
      : undefined;
    const ok = await executeNoticeOp(
      "version_create",
      sourceId,
      (key) =>
        createNoticeVersion(
          sourceId,
          {
            title: String(values.title).trim(),
            body: String(values.body),
            source_url: String(values.source_url).trim(),
            ...(publishedAt ? { source_published_at: publishedAt } : {}),
          },
          key,
        ),
      "通知版本已创建，已进入待审核队列。",
    );
    if (ok) {
      versionForm.resetFields();
      setVersionDrawerOpen(false);
      setStateFilter("pending_review");
    }
  };

  const handleReview = async (item: NoticeVersion, decision: "approved" | "rejected") => {
    const reason = reviewReason.trim();
    if (decision === "rejected" && !reason) {
      message.warning("驳回通知必须填写驳回理由。");
      return;
    }
    const ok = await executeNoticeOp(
      "review",
      item.id,
      (key) =>
        reviewNoticeVersion(
          item.id,
          { decision, note: reason || undefined, expected_revision: item.revision },
          key,
        ),
      decision === "approved" ? "通知已审核通过，现在可配置分发。" : "已驳回该通知。",
    );
    if (ok) setReviewReason("");
  };

  const handleConfirmDistribute = async () => {
    if (!confirmDist) return;
    const { item, channel: distChannel, audience } = confirmDist;
    setConfirmDist(null);
    const payload = { channel: distChannel, audience, expected_revision: item.revision };
    await executeNoticeOp(
      "distribution",
      item.id,
      (key) => distributeNoticeVersion(item.id, payload, key),
      "分发任务已提交队列，系统将按渠道投递。",
      payload,
    );
  };

  const filteredItems = useMemo(() => {
    const q = keyword.trim().toLocaleLowerCase();
    return (snapshot?.items ?? []).filter((item) => {
      if (stateFilter !== "all" && item.state !== stateFilter) return false;
      if (!q) return true;
      return (
        item.title.toLocaleLowerCase().includes(q) ||
        item.source.name.toLocaleLowerCase().includes(q) ||
        item.body.toLocaleLowerCase().includes(q)
      );
    });
  }, [snapshot, stateFilter, keyword]);

  const selectedVersion = useMemo(
    () => snapshot?.items.find((item) => item.id === selectedId),
    [snapshot, selectedId],
  );

  const columns: ColumnsType<NoticeVersion> = [
    {
      title: "通知标题",
      key: "title",
      render: (_, item) => (
        <div>
          <Typography.Text strong>{item.title}</Typography.Text>
          <div style={{ marginTop: 2, fontSize: 12, color: "#64748b" }}>
            {item.source.name} · 第 {item.version} 版
          </div>
        </div>
      ),
    },
    {
      title: "状态",
      dataIndex: "state",
      width: 110,
      render: (val: NoticeVersion["state"]) => stateTag(val),
    },
    {
      title: "分发进度",
      key: "dist",
      width: 220,
      render: (_, item) =>
        item.state === "distributed" ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {item.distribution_count} 次分发 · {distributionStatusText(item.distribution_status)}
          </Typography.Text>
        ) : (
          <Typography.Text type="secondary">—</Typography.Text>
        ),
    },
    {
      title: "操作",
      key: "actions",
      width: 120,
      render: (_, item) => (
        <Button
          size="small"
          icon={<EyeOutlined />}
          onClick={() => {
            setSelectedId(item.id);
            setReviewReason("");
            syncQuery(stateFilter, item.id);
          }}
        >
          查看 / 处理
        </Button>
      ),
    },
  ];

  return (
    <section aria-labelledby="notice-heading">
      <div className="console-page-header">
        <div>
          <h1 id="notice-heading" className="console-page-title">
            校园通知
          </h1>
          <p className="console-page-subtitle">
            管理通知来源、审核发布版本并按站内消息或邮件定向分发。正文发布后不可篡改，所有操作留痕。
          </p>
        </div>
        <Space wrap>
          {canManage && state === "ready" && (
            <>
              <Button icon={<PlusOutlined />} onClick={() => setSourceModalOpen(true)}>
                登记来源
              </Button>
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => setVersionDrawerOpen(true)}
              >
                发布新通知
              </Button>
            </>
          )}
          <Button icon={<ReloadOutlined />} loading={state === "loading"} onClick={() => void refresh()}>
            刷新
          </Button>
        </Space>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="刚才的通知操作尚未收到最终确认。请点击右侧查询原结果，避免重复发送或重复分发。"
        busy={busy}
        onResolve={() => void resolvePending()}
      />

      {state !== "ready" && state !== "stale" ? (
        <ModuleStateView
          state={state}
          loadingText="正在读取通知列表…"
          deniedTitle="没有校园通知运营权限"
          notConfiguredTitle="校园通知工作台尚未接入"
          notConfiguredDescription="当前环境尚未连接通知服务，请联系管理员配置；此状态不影响其他业务。"
          onRetry={() => void refresh()}
        />
      ) : (
        <>
          {state === "stale" && (
            <Alert
              type="warning"
              showIcon
              title="刷新失败，以下为上次读取的内容"
              description="当前仅可核对，重新刷新成功前无法执行审核或分发。"
              style={{ marginBottom: 16 }}
            />
          )}

          <Card>
            <Tabs
              activeKey={activeTab}
              onChange={(key) => setActiveTab(key as "versions" | "sources")}
              items={[
                {
                  key: "versions",
                  label: `通知审核与分发 (${snapshot?.items.length ?? 0})`,
                  children: (
                    <>
                      <div
                        style={{
                          display: "flex",
                          flexWrap: "wrap",
                          alignItems: "center",
                          justifyContent: "space-between",
                          gap: 12,
                          marginBottom: 16,
                        }}
                      >
                        <Segmented
                          value={stateFilter}
                          onChange={(val) => {
                            const next = val as NoticeFilter;
                            setStateFilter(next);
                            setSelectedId("");
                            syncQuery(next, "");
                          }}
                          options={[
                            { label: "待审核", value: "pending_review" },
                            { label: "已通过", value: "approved" },
                            { label: "已分发", value: "distributed" },
                            { label: "已拒绝", value: "rejected" },
                            { label: "全部", value: "all" },
                          ]}
                        />
                        <Input
                          allowClear
                          prefix={<SearchOutlined />}
                          placeholder="搜通知标题、来源或正文关键词…"
                          value={keyword}
                          onChange={(e) => setKeyword(e.target.value)}
                          style={{ maxWidth: 280 }}
                        />
                      </div>

                      {snapshot && (
                        <Typography.Text
                          type="secondary"
                          style={{ display: "block", marginBottom: 12, fontSize: 12 }}
                        >
                          展示最近 50 个通知版本 · 当前筛选命中 {filteredItems.length} 条 · 更新于{" "}
                          {localDateTime(snapshot.generated_at)}
                        </Typography.Text>
                      )}

                      <Table
                        rowKey="id"
                        columns={columns}
                        dataSource={filteredItems}
                        scroll={{ x: "max-content" }}
                        pagination={{ pageSize: 12, showSizeChanger: false }}
                        locale={{ emptyText: "当前筛选下暂无通知" }}
                      />
                    </>
                  ),
                },
                {
                  key: "sources",
                  label: `通知来源管理 (${mergedSources.length})`,
                  children: (
                    <Table
                      rowKey="id"
                      dataSource={mergedSources}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 12, showSizeChanger: false }}
                      locale={{ emptyText: "尚未登记通知来源，请点击右上角「登记来源」" }}
                      columns={[
                        {
                          title: "来源名称",
                          key: "name",
                          render: (_, src: ConsoleNoticeSourceDetail) => (
                            <div>
                              <Typography.Text strong>{src.name}</Typography.Text>
                              <div>
                                <span className="console-code-chip">{src.code}</span>
                              </div>
                            </div>
                          ),
                        },
                        {
                          title: "来源主页",
                          dataIndex: "canonical_url",
                          render: (url?: string) =>
                            url ? (
                              <a href={url} target="_blank" rel="noreferrer">
                                {url}
                              </a>
                            ) : (
                              <Typography.Text type="secondary">—</Typography.Text>
                            ),
                        },
                        {
                          title: "创建时间",
                          dataIndex: "created_at",
                          width: 160,
                          render: (val?: string) => (val ? localDateTime(val) : "—"),
                        },
                        {
                          title: "操作",
                          key: "actions",
                          width: 140,
                          render: (_, src: ConsoleNoticeSourceDetail) =>
                            canManage ? (
                              <Button
                                size="small"
                                type="primary"
                                onClick={() => {
                                  versionForm.setFieldValue("source_id", src.id);
                                  setVersionDrawerOpen(true);
                                }}
                              >
                                发布新通知
                              </Button>
                            ) : (
                              <Typography.Text type="secondary">仅查看</Typography.Text>
                            ),
                        },
                      ]}
                    />
                  ),
                },
              ]}
            />
          </Card>
        </>
      )}

      {/* 通知详情、审核与分发抽屉 */}
      <Drawer
        open={Boolean(selectedVersion)}
        title={selectedVersion?.title ?? "通知详情"}
        size={600}
        onClose={() => {
          setSelectedId("");
          syncQuery(stateFilter, "");
        }}
      >
        {selectedVersion && (
          <Space orientation="vertical" size={16} style={{ width: "100%" }}>
            <Descriptions bordered size="small" column={2}>
              <Descriptions.Item label="通知来源">{selectedVersion.source.name}</Descriptions.Item>
              <Descriptions.Item label="当前状态">{stateTag(selectedVersion.state)}</Descriptions.Item>
              <Descriptions.Item label="正文版本">第 {selectedVersion.version} 版</Descriptions.Item>
              <Descriptions.Item label="原文链接">
                <a href={selectedVersion.source_url} target="_blank" rel="noreferrer">
                  <Space size={4}>
                    <LinkOutlined />
                    查看原文
                  </Space>
                </a>
              </Descriptions.Item>
            </Descriptions>

            <Card size="small" title="通知正文">
              <Typography.Paragraph
                style={{
                  whiteSpace: "pre-wrap",
                  marginBottom: 0,
                  lineHeight: 1.75,
                  wordBreak: "break-word",
                }}
              >
                {selectedVersion.body}
              </Typography.Paragraph>
            </Card>

            {state === "ready" && selectedVersion.state === "pending_review" && canReview && (
              <Card size="small" title="审核处理">
                <Space orientation="vertical" size={12} style={{ width: "100%" }}>
                  <Input.TextArea
                    rows={3}
                    maxLength={1000}
                    placeholder="填写审核说明（拒绝时必填；通过可留空）"
                    value={reviewReason}
                    onChange={(e) => setReviewReason(e.target.value)}
                  />
                  <Space>
                    <Button
                      type="primary"
                      icon={<CheckOutlined />}
                      loading={busy}
                      disabled={Boolean(pending)}
                      onClick={() => void handleReview(selectedVersion, "approved")}
                    >
                      批准通过
                    </Button>
                    <Button
                      danger
                      icon={<CloseOutlined />}
                      loading={busy}
                      disabled={Boolean(pending) || !reviewReason.trim()}
                      onClick={() => void handleReview(selectedVersion, "rejected")}
                    >
                      拒绝
                    </Button>
                  </Space>
                </Space>
              </Card>
            )}

            {state === "ready" && selectedVersion.state === "approved" && canDistribute && (
              <Card size="small" title="定向分发">
                <Space orientation="vertical" size={12} style={{ width: "100%" }}>
                  <Space wrap style={{ width: "100%" }}>
                    <div>
                      <Typography.Text type="secondary" style={{ display: "block", marginBottom: 4, fontSize: 12 }}>
                        发送渠道
                      </Typography.Text>
                      <Select
                        style={{ width: 140 }}
                        value={channel}
                        onChange={(v) => setChannel(v)}
                        options={[
                          { label: "站内消息", value: "in_app" },
                          { label: "校园邮件", value: "email" },
                        ]}
                      />
                    </div>
                    <div>
                      <Typography.Text type="secondary" style={{ display: "block", marginBottom: 4, fontSize: 12 }}>
                        推送受众
                      </Typography.Text>
                      <Select
                        style={{ width: 150 }}
                        value={audienceKind}
                        onChange={(v) => setAudienceKind(v)}
                        options={[
                          { label: "全体学生", value: "all_students" },
                          { label: "指定学院", value: "college" },
                          { label: "指定角色", value: "role" },
                        ]}
                      />
                    </div>
                    {audienceKind !== "all_students" && (
                      <div style={{ flex: 1, minWidth: 180 }}>
                        <Typography.Text type="secondary" style={{ display: "block", marginBottom: 4, fontSize: 12 }}>
                          受众标识
                        </Typography.Text>
                        <Input
                          placeholder="例如：software-college"
                          value={audienceValue}
                          onChange={(e) => setAudienceValue(e.target.value)}
                        />
                      </div>
                    )}
                  </Space>
                  <Button
                    type="primary"
                    icon={<SendOutlined />}
                    disabled={
                      busy ||
                      Boolean(pending) ||
                      (audienceKind !== "all_students" && !audienceValue.trim())
                    }
                    onClick={() =>
                      setConfirmDist({
                        item: selectedVersion,
                        channel,
                        audience:
                          audienceKind === "all_students"
                            ? { kind: "all_students" }
                            : { kind: audienceKind, value: audienceValue.trim() },
                      })
                    }
                  >
                    创建分发任务
                  </Button>
                </Space>
              </Card>
            )}

            {selectedVersion.state === "distributed" && (
              <Alert
                type="info"
                showIcon
                title={`已提交 ${selectedVersion.distribution_count} 次分发`}
                description={distributionStatusText(selectedVersion.distribution_status)}
              />
            )}
          </Space>
        )}
      </Drawer>

      {/* 分发前二次确认弹窗 */}
      <Modal
        open={Boolean(confirmDist)}
        title="确认创建通知分发任务"
        okText="确认分发"
        cancelText="取消"
        confirmLoading={busy}
        onOk={() => void handleConfirmDistribute()}
        onCancel={() => setConfirmDist(null)}
      >
        {confirmDist && (
          <Space orientation="vertical" size={12} style={{ width: "100%", marginTop: 8 }}>
            <Alert
              type="warning"
              showIcon
              title="分发任务提交后将向目标受众发送通知，请核对渠道与范围。"
            />
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="通知标题">{confirmDist.item.title}</Descriptions.Item>
              <Descriptions.Item label="发送渠道">
                {channelLabel(confirmDist.channel)}
              </Descriptions.Item>
              <Descriptions.Item label="推送受众">
                {audienceLabel(confirmDist.audience)}
              </Descriptions.Item>
            </Descriptions>
          </Space>
        )}
      </Modal>

      {/* 登记来源弹窗 */}
      <Modal
        open={sourceModalOpen}
        title="登记通知来源"
        okText="创建来源"
        cancelText="取消"
        confirmLoading={busy}
        onOk={() => void handleCreateSource()}
        onCancel={() => setSourceModalOpen(false)}
      >
        <Form form={sourceForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item
            name="code"
            label="来源代码"
            rules={[
              { required: true, message: "请输入来源代码" },
              { pattern: /^[a-z0-9][a-z0-9-]{1,62}$/, message: "仅限小写字母、数字和连字符（2–63 位）" },
            ]}
          >
            <Input placeholder="例如：henu-jwc" />
          </Form.Item>
          <Form.Item
            name="name"
            label="来源名称"
            rules={[{ required: true, whitespace: true, message: "请输入来源名称" }]}
          >
            <Input maxLength={120} placeholder="例如：河南大学教务处" />
          </Form.Item>
          <Form.Item
            name="canonical_url"
            label="来源主页（HTTPS）"
            rules={[
              { required: true, message: "请输入来源主页网址" },
              { pattern: /^https:\/\/.+/, message: "必须以 https:// 开头" },
            ]}
          >
            <Input placeholder="https://jwc.henu.edu.cn" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 发布新通知版本抽屉 */}
      <Drawer
        open={versionDrawerOpen}
        title="创建通知版本"
        size={520}
        onClose={() => setVersionDrawerOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setVersionDrawerOpen(false)}>取消</Button>
            <Button type="primary" loading={busy} onClick={() => void handleCreateVersion()}>
              创建并送审
            </Button>
          </Space>
        }
      >
        <Form form={versionForm} layout="vertical">
          {knownSources.length > 0 ? (
            <Form.Item
              name="source_id"
              label="通知来源"
              extra="直接选择已登记的通知来源；若需新来源请先点击「登记来源」。"
              rules={[{ required: true, message: "请选择通知来源" }]}
            >
              <Select
                showSearch={{ optionFilterProp: "label" }}
                placeholder="请选择已登记的通知来源"
                options={knownSources}
              />
            </Form.Item>
          ) : (
            <Form.Item
              name="source_id"
              label="通知来源 ID（UUID）"
              extra="尚未加载到来源目录，请先点击「登记来源」或粘贴来源 UUID。"
              rules={[{ required: true, whitespace: true, message: "请选择或填写来源 ID" }]}
            >
              <Input placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
            </Form.Item>
          )}
          <Divider style={{ margin: "12px 0" }} />
          <Form.Item
            name="title"
            label="通知标题"
            rules={[{ required: true, whitespace: true, message: "请输入标题" }]}
          >
            <Input maxLength={200} placeholder="简洁明确的通知标题" />
          </Form.Item>
          <Form.Item
            name="body"
            label="通知正文"
            rules={[{ required: true, whitespace: true, message: "请输入正文" }]}
          >
            <Input.TextArea rows={6} maxLength={100000} placeholder="正文创建后不可修改" />
          </Form.Item>
          <Form.Item
            name="source_url"
            label="原文链接（HTTPS）"
            rules={[
              { required: true, message: "请输入原文链接" },
              { pattern: /^https:\/\/.+/, message: "必须以 https:// 开头" },
            ]}
          >
            <Input placeholder="https://..." />
          </Form.Item>
          <Form.Item name="source_published_at" label="原文发布时间（选填）">
            <Input type="datetime-local" />
          </Form.Item>
        </Form>
      </Drawer>
    </section>
  );
}
