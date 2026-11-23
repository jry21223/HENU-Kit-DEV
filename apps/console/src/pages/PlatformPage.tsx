import {
  DeleteOutlined,
  EditOutlined,
  LeftOutlined,
  LogoutOutlined,
  PlusOutlined,
  ReloadOutlined,
  RightOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  AutoComplete,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Drawer,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ModuleStateView } from "@/components/ModuleStateView";
import { PendingRecoveryAlert } from "@/components/PendingRecoveryAlert";
import { useConsole } from "@/lib/console-context";
import {
  fetchPlatformRoles,
  updatePlatformInboxItem,
  type ConsoleRoleCatalogEntry,
} from "@/lib/console-extensions";
import {
  fetchPlatformOperations,
  resolvePlatformOperation,
  revokePlatformSession,
  searchPlatformOperationAccounts,
  updatePlatformAccess,
  type PlatformAccessGrantInput,
  type PlatformOperationWriteResult,
  type PlatformOperationsAuditEvent,
  type PlatformOperationsPageRequest,
  type PlatformOperationsSnapshot,
} from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import {
  clearPendingPlatformOperation,
  readPendingPlatformOperation,
  writePendingPlatformOperation,
  type PendingPlatformOperation,
} from "@/lib/pending-operations";
import {
  emptyPlatformCursorHistory,
  firstPlatformPages,
  platformCursorFields,
  type InboxStatusFilter,
  type PlatformCursorHistory,
  type PlatformPageSection,
} from "@/lib/platform-view-context";

const COMMON_ROLES = [
  { value: "operations-operator", label: "operations-operator（平台运营）" },
  { value: "operations-reviewer", label: "operations-reviewer（审核专员）" },
  { value: "food-reviewer", label: "food-reviewer（美食审核）" },
  { value: "notice-operator", label: "notice-operator（校园通知运营）" },
  { value: "support-agent", label: "support-agent（客服工单专员）" },
  { value: "quizcraft-editor", label: "题库维护专员" },
];

const COMMON_PRODUCTS = [
  { value: "food", label: "美食榜（food）" },
  { value: "notice", label: "校园通知（notice）" },
  { value: "library", label: "资料库（library）" },
  { value: "quizcraft", label: "刷题练习" },
  { value: "account-portfolio", label: "账户与工单（account-portfolio）" },
];

const REASON_LABELS: Record<string, string> = {
  GRANTED: "权限授予",
  NO_GRANT: "未分配对应权限",
  PERMISSION_OR_SCOPE_MISSING: "缺少权限或授权范围",
  PERMISSION_DENIED: "缺少操作权限",
  SCOPE_MISMATCH: "超出授权作用范围",
  ROLE_GRANT_MATCHED: "角色授权校验通过",
  SESSION_INVALID: "登录会话无效",
  SESSION_REVOKED: "会话已撤销",
  SESSION_EXPIRED: "会话已过期",
  PARENT_SESSION_REVOKED: "上级会话已撤销",
  PARENT_SESSION_EXPIRED: "上级会话已过期",
  ACCOUNT_NOT_ACTIVE: "账户未激活",
  ACCESS_UPDATE_SUCCEEDED: "访问设置更新成功",
  SESSION_REVOKE_SUCCEEDED: "会话撤销成功",
  PERMISSION_GRANTED: "权限授予",
};

function formatAuditReason(reasonCode: string, decision: string): string {
  const normalized = reasonCode.trim().toUpperCase();
  if (REASON_LABELS[normalized]) {
    return REASON_LABELS[normalized];
  }
  return decision === "allowed" ? "权限校验通过" : "权限校验未通过";
}

const MAIL_LABELS: Record<string, string> = {
  pending: "排队中",
  processing: "发送中",
  retry_due: "待重试",
  accepted: "已接收",
  delivered: "已送达",
  failed: "发送失败",
  dead_letters: "投递死信",
};

const PRODUCT_LABELS: Record<string, string> = {
  quizcraft: "刷题练习",
  food: "美食榜",
  notice: "校园通知",
  library: "资料库",
  "account-portfolio": "账户与工单",
  portal: "门户网站",
  platform: "平台与安全",
  career: "求职雷达",
};

const INBOX_RESOURCE_LABELS: Record<string, string> = {
  feedback: "题目纠错反馈",
  anomaly_ticket: "异常举报",
  version_review: "通知版本审核",
  submission: "投稿审核",
  ticket: "客服工单",
  membership_order: "会员订单",
};

function statusTag(status: "active" | "suspended" | "deleted") {
  if (status === "active") return <Tag color="success">正常</Tag>;
  if (status === "suspended") return <Tag color="warning">已停用</Tag>;
  return <Tag color="error">已删除</Tag>;
}

function inboxStatusTag(status: string) {
  const map: Record<string, { label: string; color: string }> = {
    open: { label: "待处理", color: "error" },
    in_progress: { label: "处理中", color: "processing" },
    blocked: { label: "受阻", color: "warning" },
    resolved: { label: "已解决", color: "success" },
    cancelled: { label: "已归档", color: "default" },
    archived: { label: "已归档", color: "default" },
  };
  const info = map[status] ?? { label: status, color: "default" };
  return <Tag color={info.color}>{info.label}</Tag>;
}

function healthTag(value: string) {
  if (value === "ready") return <Tag color="success">正常</Tag>;
  if (value === "unavailable") return <Tag color="error">不可用</Tag>;
  return <Tag>未知</Tag>;
}

function grantText(grant: PlatformAccessGrantInput): string {
  const scope =
    grant.scope.kind === "platform"
      ? "全平台"
      : grant.scope.kind === "product"
        ? `产品:${grant.scope.product_code || "未填"}`
        : `资源:${grant.scope.product_code || ""}/${grant.scope.resource_type || ""}/${grant.scope.resource_id || ""}`;
  return `${grant.role_code} (${scope})`;
}

type ConfirmAction =
  | {
      kind: "session_revoke";
      resourceId: string;
      targetLabel: string;
      changeSummary: string;
    }
  | {
      kind: "access_update";
      resourceId: string;
      targetLabel: string;
      changeSummary: string;
      expectedRevision: number;
      status: "active" | "suspended" | "deleted";
      grants: PlatformAccessGrantInput[];
    };

export function PlatformPage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId } = useConsole();

  const [state, setState] = useState<
    "loading" | "ready" | "denied" | "invalid" | "unavailable"
  >("loading");
  const [operations, setOperations] = useState<PlatformOperationsSnapshot>();
  const [pages, setPages] = useState<PlatformOperationsPageRequest>(firstPlatformPages());
  const [cursorHistory, setCursorHistory] = useState<PlatformCursorHistory>(
    emptyPlatformCursorHistory(),
  );
  const [accountQueryInput, setAccountQueryInput] = useState("");
  const [submittedAccountQuery, setSubmittedAccountQuery] = useState("");
  const [searchCursorHistory, setSearchCursorHistory] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const [searchBusy, setSearchBusy] = useState(false);
  const [inboxFilter, setInboxFilter] = useState<InboxStatusFilter>("all");
  const [activeTab, setActiveTab] = useState("accounts");
  const [roleCatalog, setRoleCatalog] = useState<ConsoleRoleCatalogEntry[]>([]);
  const [inboxBusyId, setInboxBusyId] = useState<string | null>(null);

  // 权限编辑抽屉
  const [editingAccount, setEditingAccount] = useState<
    PlatformOperationsSnapshot["accounts"][number] | null
  >(null);
  const [editStatus, setEditStatus] = useState<"active" | "suspended" | "deleted">("active");
  const [editGrants, setEditGrants] = useState<PlatformAccessGrantInput[]>([]);

  // 确认弹窗 & 幂等恢复
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingPlatformOperation | undefined>();

  const mountedRef = useRef(true);
  const readSeqRef = useRef(0);

  const canWrite =
    operations?.access_context.permissions.includes("platform.operations.write") ?? false;

  const load = useCallback(
    async (nextPages: PlatformOperationsPageRequest = firstPlatformPages(), keepSearch = false) => {
      if (authState !== "authenticated" || !operatorId) {
        setState(authState === "loading" ? "loading" : "denied");
        return;
      }
      const seq = ++readSeqRef.current;
      setState("loading");
      const [res, rolesRes] = await Promise.all([
        fetchPlatformOperations(nextPages),
        fetchPlatformRoles(),
      ]);
      if (!mountedRef.current || seq !== readSeqRef.current) return;
      if (rolesRes.state === "authenticated" && rolesRes.data.length > 0) {
        setRoleCatalog(rolesRes.data);
      }
      if (res.state === "authenticated") {
        setOperations((prev) => {
          if (keepSearch && prev && submittedAccountQuery) {
            return {
              ...res.operations,
              accounts: prev.accounts,
              pagination: { ...res.operations.pagination, accounts: prev.pagination.accounts },
            };
          }
          return res.operations;
        });
        setPages({
          accounts_page: res.operations.pagination.accounts.page,
          sessions_page: res.operations.pagination.sessions.page,
          inbox_page: res.operations.pagination.inbox_items.page,
          audit_page: res.operations.pagination.audit.page,
          snapshot_at: res.operations.generated_at,
          accounts_cursor: nextPages.accounts_cursor,
          sessions_cursor: nextPages.sessions_cursor,
          inbox_cursor: nextPages.inbox_cursor,
          audit_cursor: nextPages.audit_cursor,
        });
        if (!keepSearch) {
          setSubmittedAccountQuery("");
          setSearchCursorHistory([undefined]);
        }
        setState("ready");
      } else {
        setState(
          res.state === "denied" || res.state === "signed_out"
            ? "denied"
            : res.state === "invalid"
              ? "invalid"
              : "unavailable",
        );
      }
    },
    [authState, operatorId, submittedAccountQuery],
  );

  const handleWriteOutcome = useCallback(
    (result: PlatformOperationWriteResult, command: PendingPlatformOperation, fromLookup: boolean) => {
      if (result.state === "succeeded") {
        setPending(undefined);
        clearPendingPlatformOperation();
        message.success("操作已完成并写入审计日志。");
        setEditingAccount(null);
        setCursorHistory(emptyPlatformCursorHistory());
        void load(firstPlatformPages());
      } else if (
        result.state === "unknown" ||
        (fromLookup &&
          (result.state === "unavailable" ||
            result.state === "signed_out" ||
            result.state === "denied" ||
            result.state === "not_found"))
      ) {
        setPending(command);
        writePendingPlatformOperation(command);
        message.warning("操作结果暂未确认，已保留记录；请点击上方横幅查询，勿重复提交。");
      } else {
        setPending(undefined);
        clearPendingPlatformOperation();
        message.error(
          result.state === "conflict"
            ? "目标账户权限刚刚有变动，请刷新后重试。"
            : result.state === "denied"
              ? "当前账户没有执行该写操作的权限。"
              : result.state === "invalid"
                ? "提交的角色或权限范围格式无效，请检查后重试。"
                : "操作未完成，请稍后刷新重试。",
        );
      }
    },
    [load, message],
  );

  const resolvePending = useCallback(
    async (target?: PendingPlatformOperation) => {
      const op = target ?? pending;
      if (!op || busy) return;
      setBusy(true);
      const res = await resolvePlatformOperation(op.operation, op.idempotency_key);
      if (!mountedRef.current) return;
      handleWriteOutcome(res, op, true);
      setBusy(false);
    },
    [pending, busy, handleWriteOutcome],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId) {
      const saved = readPendingPlatformOperation(operatorId);
      setPending(saved);
      void load(firstPlatformPages());
      if (saved) void resolvePending(saved);
    } else {
      setState(authState === "loading" ? "loading" : "denied");
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId]);

  const handleLoadPage = async (section: PlatformPageSection, targetPage: number) => {
    if (!operations || targetPage < 1) return;
    const pag =
      section === "accounts_page"
        ? operations.pagination.accounts
        : section === "sessions_page"
          ? operations.pagination.sessions
          : section === "inbox_page"
            ? operations.pagination.inbox_items
            : operations.pagination.audit;
    const cursor =
      targetPage === pag.page + 1
        ? pag.next_cursor ?? undefined
        : cursorHistory[section][targetPage - 1];
    if (targetPage > 1 && !cursor) return;
    setCursorHistory((prev) => {
      const copy = { ...prev, [section]: [...prev[section]] };
      copy[section][targetPage - 1] = cursor;
      return copy;
    });
    await load(
      { ...pages, [section]: targetPage, [platformCursorFields[section]]: cursor },
      section !== "accounts_page",
    );
  };

  const handleSearchAccounts = async (targetPage = 1, cursor?: string) => {
    if (!operations || searchBusy) return;
    const q = accountQueryInput.trim();
    if (q.length < 2) {
      message.warning("请至少输入 2 个字符（显示名称或完整邮箱）。");
      return;
    }
    setSearchBusy(true);
    const res = await searchPlatformOperationAccounts({
      query: q,
      page: targetPage,
      snapshot_at: operations.generated_at,
      ...(cursor ? { cursor } : {}),
    });
    if (!mountedRef.current) return;
    setSearchBusy(false);
    if (res.state === "authenticated") {
      setSubmittedAccountQuery(q);
      setOperations((prev) =>
        prev
          ? {
              ...prev,
              accounts: res.page.accounts,
              pagination: {
                ...prev.pagination,
                accounts: {
                  page: targetPage,
                  next_page: res.page.next_page,
                  next_cursor: res.page.next_cursor,
                },
              },
            }
          : prev,
      );
      if (res.page.next_cursor) {
        setSearchCursorHistory((prev) => {
          const next = [...prev];
          next[targetPage] = res.page.next_cursor ?? undefined;
          return next;
        });
      }
    } else {
      message.error(
        res.state === "rate_limited" ? "搜索过于频繁，请稍后再试。" : "账户搜索失败，请稍后重试。",
      );
    }
  };

  const openEditDrawer = (account: PlatformOperationsSnapshot["accounts"][number]) => {
    setEditingAccount(account);
    setEditStatus(account.status);
    setEditGrants(account.grants.map((g) => ({ role_code: g.role_code, scope: { ...g.scope } })));
  };

  const requestSaveAccess = () => {
    if (!editingAccount) return;
    const changes: string[] = [];
    if (editStatus !== editingAccount.status) {
      changes.push(`账户状态改为「${editStatus === "active" ? "正常" : editStatus === "suspended" ? "已停用" : "已删除"}」`);
    }
    if (JSON.stringify(editGrants) !== JSON.stringify(editingAccount.grants)) {
      changes.push(
        `授权调整为：${editGrants.length ? editGrants.map(grantText).join("、") : "清空全部后台角色"}`,
      );
    }
    if (!changes.length) {
      message.info("没有任何变更需要保存。");
      return;
    }
    setConfirmAction({
      kind: "access_update",
      resourceId: editingAccount.id,
      targetLabel: `${editingAccount.display_name || "未设置姓名"} (${editingAccount.email || "无邮箱"})`,
      changeSummary: changes.join("；"),
      expectedRevision: editingAccount.authorization_revision,
      status: editStatus,
      grants: editGrants,
    });
  };

  const executeConfirmed = async () => {
    if (!confirmAction || !operatorId || busy || pending) return;
    const key = `idem_console_${confirmAction.kind === "session_revoke" ? "revoke" : "access"}_${crypto.randomUUID()}`;
    const command: PendingPlatformOperation = {
      version: 1,
      operator_id: operatorId,
      operation: confirmAction.kind,
      idempotency_key: key,
      resource_id: confirmAction.resourceId,
    };
    if (!writePendingPlatformOperation(command)) {
      message.error("浏览器暂不允许会话存储，操作未提交。");
      return;
    }
    const action = confirmAction;
    setConfirmAction(null);
    setPending(command);
    setBusy(true);
    const res =
      action.kind === "session_revoke"
        ? await revokePlatformSession(action.resourceId, key)
        : await updatePlatformAccess(
            action.resourceId,
            {
              expected_revision: action.expectedRevision,
              status: action.status,
              grants: action.grants,
            },
            key,
          );
    if (!mountedRef.current) return;
    handleWriteOutcome(res, command, false);
    setBusy(false);
  };

  const visibleInbox = useMemo(
    () =>
      (operations?.inbox_items ?? []).filter(
        (item) => inboxFilter === "all" || item.status === inboxFilter,
      ),
    [operations, inboxFilter],
  );

  const accountColumns: ColumnsType<PlatformOperationsSnapshot["accounts"][number]> = [
    {
      title: "用户",
      key: "user",
      render: (_, acc) => (
        <div>
          <Typography.Text strong>{acc.display_name || "未设置姓名"}</Typography.Text>
          <div style={{ fontSize: 12, color: "#64748b", wordBreak: "break-all" }}>
            {acc.email || "邮箱不可用"} · {acc.email_verified ? "邮箱已验证" : "未验证"}
          </div>
        </div>
      ),
    },
    {
      title: "账户状态",
      dataIndex: "status",
      width: 100,
      render: (val: "active" | "suspended" | "deleted") => statusTag(val),
    },
    {
      title: "后台角色授权",
      key: "grants",
      render: (_, acc) =>
        acc.grants.length > 0 ? (
          <Space size={[4, 4]} wrap>
            {acc.grants.map((g, idx) => (
              <Tag key={idx}>{grantText(g)}</Tag>
            ))}
          </Space>
        ) : (
          <Typography.Text type="secondary">无后台角色</Typography.Text>
        ),
    },
    {
      title: "操作",
      key: "actions",
      width: 120,
      render: (_, acc) =>
        canWrite ? (
          <Button
            size="small"
            icon={<EditOutlined />}
            disabled={busy || Boolean(pending)}
            onClick={() => openEditDrawer(acc)}
          >
            管理权限
          </Button>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const sessionColumns: ColumnsType<PlatformOperationsSnapshot["sessions"][number]> = [
    {
      title: "登录用户",
      key: "user",
      render: (_, s) => (
        <div>
          <Typography.Text strong>{s.display_name || "未设置姓名"}</Typography.Text>
          <div style={{ fontSize: 12, color: "#64748b" }}>{s.email || "邮箱不可用"}</div>
        </div>
      ),
    },
    {
      title: "会话类型",
      dataIndex: "kind",
      width: 110,
      render: (k: string) => <Tag>{k === "core" ? "主会话" : k === "client_exchange" ? "客户端" : k}</Tag>,
    },
    {
      title: "过期时间",
      dataIndex: "expires_at",
      width: 160,
      render: (v: string) => localDateTime(v),
    },
    {
      title: "操作",
      key: "actions",
      width: 120,
      render: (_, s) =>
        s.revoked_at ? (
          <Tag>已撤销</Tag>
        ) : canWrite ? (
          <Button
            danger
            size="small"
            icon={<LogoutOutlined />}
            disabled={busy || Boolean(pending)}
            onClick={() =>
              setConfirmAction({
                kind: "session_revoke",
                resourceId: s.id,
                targetLabel: `${s.display_name || "未设置姓名"} (${s.email || "无邮箱"})`,
                changeSummary: `强制下线该登录会话（原到期时间 ${localDateTime(s.expires_at)}）`,
              })
            }
          >
            强制下线
          </Button>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const roleOptions = useMemo(() => {
    if (roleCatalog.length > 0) {
      return roleCatalog.map((r) => ({
        value: r.code,
        label: `${r.code}（${r.display_name}${r.permissions.length > 0 ? ` · ${r.permissions.length} 项权限` : ""}）`,
      }));
    }
    return COMMON_ROLES;
  }, [roleCatalog]);

  const handleUpdateInboxStatus = async (
    item: PlatformOperationsSnapshot["inbox_items"][number],
    nextStatus: "in_progress" | "blocked" | "resolved" | "cancelled",
  ) => {
    if (!canWrite || inboxBusyId) return;
    setInboxBusyId(item.id);
    const key = `idem_platform_inbox_${item.id}_${crypto.randomUUID()}`;
    const res = await updatePlatformInboxItem(
      item.id,
      {
        expected_version: item.version,
        status: nextStatus,
        source_product_code: item.source_product_code,
        source_resource_type: item.source_resource_type,
        source_resource_id: item.source_resource_id,
      },
      key,
    );
    if (!mountedRef.current) return;
    setInboxBusyId(null);
    if (res.state === "succeeded") {
      message.success("待办状态已更新。");
      setOperations((prev) =>
        prev
          ? {
              ...prev,
              inbox_items: prev.inbox_items.map((x) =>
                x.id === item.id
                  ? {
                      ...x,
                      ...res.data,
                      status: res.data.status || nextStatus,
                      version: res.data.version ?? x.version + 1,
                    }
                  : x,
              ),
            }
          : prev,
      );
    } else if (res.state === "conflict") {
      message.warning("该待办刚刚被其他人更新，已为你刷新最新列表。");
      void load(pages, true);
    } else {
      message.error(res.state === "denied" ? "当前账户缺少更新待办的权限。" : "更新待办状态失败，请稍后重试。");
    }
  };

  const auditColumns: ColumnsType<PlatformOperationsAuditEvent> = [
    {
      title: "时间",
      dataIndex: "created_at",
      width: 150,
      render: (v: string) => localDateTime(v),
    },
    {
      title: "结果",
      dataIndex: "decision",
      width: 80,
      render: (d: string) =>
        d === "allowed" ? <Tag color="success">允许</Tag> : <Tag color="error">拒绝</Tag>,
    },
    {
      title: "操作人",
      key: "actor",
      render: (_, e) => (
        <div>
          <div>{e.display_name || "未设置姓名"}</div>
          <div style={{ fontSize: 12, color: "#64748b" }}>{e.email || "—"}</div>
        </div>
      ),
    },
    {
      title: "权限与原因",
      key: "perm",
      render: (_, e) => (
        <div>
          <span className="console-code-chip">{e.permission_code}</span>
          <div style={{ marginTop: 4, fontSize: 12, color: "#64748b" }}>
            {formatAuditReason(e.reason_code, e.decision)}
          </div>
        </div>
      ),
    },
  ];

  return (
    <section aria-labelledby="operations-heading">
      <div className="console-page-header">
        <div>
          <h1 id="operations-heading" className="console-page-title">
            平台与安全
          </h1>
          <p className="console-page-subtitle">
            管理全站用户角色授权、在线登录会话、跨产品运营待办、邮件基础设施与权限审计。
          </p>
        </div>
        <Space wrap>
          {operations && (
            <>
              <Tag color={canWrite ? "blue" : "default"}>{canWrite ? "可读写权限" : "只读权限"}</Tag>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                数据库 {healthTag(operations.dependencies.postgres)} 缓存{" "}
                {healthTag(operations.dependencies.redis)}
              </Typography.Text>
            </>
          )}
          <Button
            icon={<ReloadOutlined />}
            loading={state === "loading"}
            onClick={() => {
              setAccountQueryInput("");
              setSubmittedAccountQuery("");
              setCursorHistory(emptyPlatformCursorHistory());
              void load(firstPlatformPages());
            }}
          >
            刷新
          </Button>
        </Space>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="刚才的安全操作尚未确认结果。点击右侧查询原结果，请勿重复修改。"
        busy={busy}
        onResolve={() => void resolvePending()}
      />

      {state !== "ready" ? (
        <ModuleStateView
          state={state}
          loadingText="正在读取平台与安全数据…"
          deniedTitle="没有平台与安全管理权限"
          onRetry={() => {
            setCursorHistory(emptyPlatformCursorHistory());
            void load(firstPlatformPages());
          }}
        />
      ) : operations ? (
        <Card>
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: "accounts",
                label: `用户与权限 (${operations.accounts.length})`,
                children: (
                  <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                    <Space wrap>
                      <Input
                        allowClear
                        prefix={<SearchOutlined />}
                        placeholder="按显示名称或完整邮箱搜索账户…"
                        value={accountQueryInput}
                        onChange={(e) => setAccountQueryInput(e.target.value)}
                        onPressEnter={() => void handleSearchAccounts(1)}
                        style={{ width: 280 }}
                      />
                      <Button
                        type="primary"
                        loading={searchBusy}
                        onClick={() => void handleSearchAccounts(1)}
                      >
                        搜索账户
                      </Button>
                      {submittedAccountQuery && (
                        <Button
                          onClick={() => {
                            setAccountQueryInput("");
                            setSubmittedAccountQuery("");
                            setSearchCursorHistory([undefined]);
                            void load(firstPlatformPages());
                          }}
                        >
                          返回全部列表
                        </Button>
                      )}
                    </Space>
                    <Table
                      rowKey="id"
                      columns={accountColumns}
                      dataSource={operations.accounts}
                      scroll={{ x: "max-content" }}
                      pagination={false}
                      locale={{ emptyText: "没有匹配的账户" }}
                    />
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Button
                        icon={<LeftOutlined />}
                        disabled={operations.pagination.accounts.page <= 1}
                        onClick={() => {
                          const target = operations.pagination.accounts.page - 1;
                          if (submittedAccountQuery) {
                            void handleSearchAccounts(target, searchCursorHistory[target - 1]);
                          } else {
                            void handleLoadPage("accounts_page", target);
                          }
                        }}
                      >
                        上一页
                      </Button>
                      <Typography.Text type="secondary">
                        第 {operations.pagination.accounts.page} 页
                      </Typography.Text>
                      <Button
                        icon={<RightOutlined />}
                        iconPlacement="end"
                        disabled={operations.pagination.accounts.next_page === null}
                        onClick={() => {
                          const next = operations.pagination.accounts.next_page;
                          if (!next) return;
                          if (submittedAccountQuery) {
                            void handleSearchAccounts(
                              next,
                              operations.pagination.accounts.next_cursor ?? undefined,
                            );
                          } else {
                            void handleLoadPage("accounts_page", next);
                          }
                        }}
                      >
                        下一页
                      </Button>
                    </div>
                  </Space>
                ),
              },
              {
                key: "sessions",
                label: `登录会话 (${operations.sessions.length})`,
                children: (
                  <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                    <Table
                      rowKey="id"
                      columns={sessionColumns}
                      dataSource={operations.sessions}
                      scroll={{ x: "max-content" }}
                      pagination={false}
                      locale={{ emptyText: "当前页暂无活跃会话" }}
                    />
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Button
                        icon={<LeftOutlined />}
                        disabled={operations.pagination.sessions.page <= 1}
                        onClick={() =>
                          void handleLoadPage("sessions_page", operations.pagination.sessions.page - 1)
                        }
                      >
                        上一页
                      </Button>
                      <Typography.Text type="secondary">
                        第 {operations.pagination.sessions.page} 页
                      </Typography.Text>
                      <Button
                        icon={<RightOutlined />}
                        iconPlacement="end"
                        disabled={operations.pagination.sessions.next_page === null}
                        onClick={() =>
                          operations.pagination.sessions.next_page &&
                          void handleLoadPage("sessions_page", operations.pagination.sessions.next_page)
                        }
                      >
                        下一页
                      </Button>
                    </div>
                  </Space>
                ),
              },
              {
                key: "inbox",
                label: `跨产品待办 (${operations.inbox_items.length})`,
                children: (
                  <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                    <Space>
                      <Typography.Text type="secondary">状态筛选：</Typography.Text>
                      <Select
                        style={{ width: 150 }}
                        value={inboxFilter}
                        onChange={(v) => setInboxFilter(v)}
                        options={[
                          { label: "全部状态", value: "all" },
                          { label: "待处理", value: "open" },
                          { label: "处理中", value: "in_progress" },
                          { label: "受阻", value: "blocked" },
                          { label: "已解决", value: "resolved" },
                          { label: "已归档", value: "cancelled" },
                        ]}
                      />
                    </Space>
                    <Table
                      rowKey="id"
                      dataSource={visibleInbox}
                      scroll={{ x: "max-content" }}
                      pagination={false}
                      locale={{ emptyText: "当前页暂无符合条件的待办项" }}
                      columns={[
                        {
                          title: "来源模块",
                          key: "source",
                          render: (_, item) => (
                            <Space>
                              <Tag>
                                {PRODUCT_LABELS[item.source_product_code] ??
                                  item.source_product_code}
                              </Tag>
                              <span>
                                {INBOX_RESOURCE_LABELS[item.source_resource_type] ??
                                  item.source_resource_type}
                              </span>
                            </Space>
                          ),
                        },
                        {
                          title: "关联资源",
                          dataIndex: "source_resource_id",
                          render: (id: string) => <span className="console-code-chip">{id}</span>,
                        },
                        {
                          title: "状态",
                          dataIndex: "status",
                          width: 100,
                          render: (s: string) => inboxStatusTag(s),
                        },
                        {
                          title: "更新时间",
                          dataIndex: "updated_at",
                          width: 160,
                          render: (v: string) => localDateTime(v),
                        },
                        {
                          title: "操作",
                          key: "actions",
                          width: 220,
                          render: (_, item) =>
                            canWrite ? (
                              <Space size={6} wrap>
                                {item.status === "open" && (
                                  <Button
                                    size="small"
                                    loading={inboxBusyId === item.id}
                                    onClick={() => void handleUpdateInboxStatus(item, "in_progress")}
                                  >
                                    开始处理
                                  </Button>
                                )}
                                {item.status !== "resolved" && item.status !== "archived" && (item.status as string) !== "cancelled" && (
                                  <Button
                                    type="primary"
                                    size="small"
                                    loading={inboxBusyId === item.id}
                                    onClick={() => void handleUpdateInboxStatus(item, "resolved")}
                                  >
                                    标记解决
                                  </Button>
                                )}
                                {item.status === "resolved" && (
                                  <Button
                                    size="small"
                                    loading={inboxBusyId === item.id}
                                    onClick={() => void handleUpdateInboxStatus(item, "cancelled")}
                                  >
                                    归档
                                  </Button>
                                )}
                              </Space>
                            ) : (
                              <Typography.Text type="secondary">仅查看</Typography.Text>
                            ),
                        },
                      ]}
                      expandable={{
                        expandedRowRender: (item) => (
                          <Descriptions size="small" column={2}>
                            <Descriptions.Item label="说明">
                              {item.source_product_code === "quizcraft" &&
                              item.source_resource_type === "feedback"
                                ? "来自刷题纠错反馈，请结合题库管理核查后在此流转处理状态。"
                                : "跨产品运营待办，处理完毕后可直接标记解决或归档。"}
                            </Descriptions.Item>
                            {item.sla_due_at && (
                              <Descriptions.Item label="处理时限">
                                {localDateTime(item.sla_due_at)}
                              </Descriptions.Item>
                            )}
                          </Descriptions>
                        ),
                      }}
                    />
                  </Space>
                ),
              },
              {
                key: "mail",
                label: "邮件投递状态",
                children: (
                  <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                    <Alert
                      type="info"
                      showIcon
                      title="展示系统全量邮件投递队列汇总指标；死信数大于 0 时请及时检查 SMTP 配置。"
                    />
                    <Row gutter={[12, 12]}>
                      {Object.entries(operations.mail).map(([k, v]) => (
                        <Col xs={12} sm={8} md={6} key={k}>
                          <Card size="small">
                            <Statistic
                              title={MAIL_LABELS[k] ?? k}
                              value={v}
                              styles={{
                                content: {
                                  color:
                                    (k === "failed" || k === "dead_letters") && v > 0
                                      ? "#dc2626"
                                      : undefined,
                                },
                              }}
                            />
                          </Card>
                        </Col>
                      ))}
                    </Row>
                  </Space>
                ),
              },
              {
                key: "audit",
                label: `授权审计 (${operations.audit.length})`,
                children: (
                  <Space orientation="vertical" size={14} style={{ width: "100%" }}>
                    <Table
                      rowKey={(e) => `${e.request_id}-${e.created_at}`}
                      columns={auditColumns}
                      dataSource={operations.audit}
                      scroll={{ x: "max-content" }}
                      pagination={false}
                      locale={{ emptyText: "暂无审计事件" }}
                    />
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Button
                        icon={<LeftOutlined />}
                        disabled={operations.pagination.audit.page <= 1}
                        onClick={() =>
                          void handleLoadPage("audit_page", operations.pagination.audit.page - 1)
                        }
                      >
                        上一页
                      </Button>
                      <Typography.Text type="secondary">
                        第 {operations.pagination.audit.page} 页
                      </Typography.Text>
                      <Button
                        icon={<RightOutlined />}
                        iconPlacement="end"
                        disabled={operations.pagination.audit.next_page === null}
                        onClick={() =>
                          operations.pagination.audit.next_page &&
                          void handleLoadPage("audit_page", operations.pagination.audit.next_page)
                        }
                      >
                        下一页
                      </Button>
                    </div>
                  </Space>
                ),
              },
            ]}
          />
        </Card>
      ) : null}

      {/* 编辑账户状态与角色授权抽屉 */}
      <Drawer
        open={Boolean(editingAccount)}
        title={`管理权限：${editingAccount?.display_name || editingAccount?.email || ""}`}
        size={540}
        onClose={() => setEditingAccount(null)}
        extra={
          <Space>
            <Button onClick={() => setEditingAccount(null)}>取消</Button>
            <Button type="primary" loading={busy} onClick={requestSaveAccess}>
              保存更改
            </Button>
          </Space>
        }
      >
        {editingAccount && (
          <Space orientation="vertical" size={16} style={{ width: "100%" }}>
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="用户姓名">
                {editingAccount.display_name || "未设置姓名"}
              </Descriptions.Item>
              <Descriptions.Item label="学校邮箱">
                {editingAccount.email || "—"}
              </Descriptions.Item>
            </Descriptions>

            <div>
              <Typography.Text strong style={{ display: "block", marginBottom: 6 }}>
                账户状态
              </Typography.Text>
              <Select
                style={{ width: "100%" }}
                value={editStatus}
                onChange={(v) => setEditStatus(v)}
                options={[
                  { label: "正常（可登录与使用）", value: "active" },
                  { label: "已停用（禁止登录）", value: "suspended" },
                  { label: "已删除（逻辑停用且不再通过权限检查）", value: "deleted" },
                ]}
              />
            </div>

            <Divider style={{ margin: "4px 0" }} />

            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography.Text strong>角色与权限范围</Typography.Text>
              <Button
                size="small"
                icon={<PlusOutlined />}
                onClick={() =>
                  setEditGrants((prev) => [
                    ...prev,
                    { role_code: "operations-operator", scope: { kind: "platform" } },
                  ])
                }
              >
                添加角色
              </Button>
            </div>

            {editGrants.length === 0 && (
              <Alert
                type="info"
                showIcon
                title="当前未分配任何后台角色，该用户将仅保留普通学生身份。"
              />
            )}

            {editGrants.map((grant, idx) => (
              <Card
                key={idx}
                size="small"
                title={`授权 #${idx + 1}`}
                extra={
                  <Button
                    type="text"
                    danger
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() =>
                      setEditGrants((prev) => prev.filter((_, i) => i !== idx))
                    }
                  >
                    移除
                  </Button>
                }
              >
                <Space orientation="vertical" size={10} style={{ width: "100%" }}>
                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      角色代码{roleCatalog.length > 0 ? "（已实时同步服务端角色目录）" : "（可从预设选择或直接输入）"}
                    </Typography.Text>
                    <AutoComplete
                      style={{ width: "100%", marginTop: 4 }}
                      options={roleOptions}
                      value={grant.role_code}
                      onChange={(val) =>
                        setEditGrants((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, role_code: val } : item)),
                        )
                      }
                      placeholder="例如：operations-operator"
                    />
                  </div>

                  <div>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      生效范围
                    </Typography.Text>
                    <Select
                      style={{ width: "100%", marginTop: 4 }}
                      value={grant.scope.kind}
                      onChange={(kind) =>
                        setEditGrants((prev) =>
                          prev.map((item, i) => {
                            if (i !== idx) return item;
                            if (kind === "platform") return { ...item, scope: { kind: "platform" } };
                            if (kind === "product") {
                              return {
                                ...item,
                                scope: { kind: "product", product_code: item.scope.product_code || "food" },
                              };
                            }
                            return {
                              ...item,
                              scope: {
                                kind: "resource",
                                product_code: item.scope.product_code || "",
                                resource_type: item.scope.resource_type || "",
                                resource_id: item.scope.resource_id || "",
                              },
                            };
                          }),
                        )
                      }
                      options={[
                        { label: "全平台范围", value: "platform" },
                        { label: "单个产品模块", value: "product" },
                        { label: "单个具体资源", value: "resource" },
                      ]}
                    />
                  </div>

                  {grant.scope.kind !== "platform" && (
                    <div>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        产品模块
                      </Typography.Text>
                      <AutoComplete
                        style={{ width: "100%", marginTop: 4 }}
                        options={COMMON_PRODUCTS}
                        value={grant.scope.product_code ?? ""}
                        onChange={(val) =>
                          setEditGrants((prev) =>
                            prev.map((item, i) =>
                              i === idx
                                ? { ...item, scope: { ...item.scope, product_code: val } }
                                : item,
                            ),
                          )
                        }
                      />
                    </div>
                  )}

                  {grant.scope.kind === "resource" && (
                    <Row gutter={8}>
                      <Col span={12}>
                        <Input
                          placeholder="资源类型"
                          value={grant.scope.resource_type ?? ""}
                          onChange={(e) =>
                            setEditGrants((prev) =>
                              prev.map((item, i) =>
                                i === idx
                                  ? {
                                      ...item,
                                      scope: { ...item.scope, resource_type: e.target.value },
                                    }
                                  : item,
                              ),
                            )
                          }
                        />
                      </Col>
                      <Col span={12}>
                        <Input
                          placeholder="资源 ID"
                          value={grant.scope.resource_id ?? ""}
                          onChange={(e) =>
                            setEditGrants((prev) =>
                              prev.map((item, i) =>
                                i === idx
                                  ? {
                                      ...item,
                                      scope: { ...item.scope, resource_id: e.target.value },
                                    }
                                  : item,
                              ),
                            )
                          }
                        />
                      </Col>
                    </Row>
                  )}
                </Space>
              </Card>
            ))}
          </Space>
        )}
      </Drawer>

      {/* 高风险安全操作确认弹窗 */}
      <Modal
        open={Boolean(confirmAction)}
        title={
          confirmAction?.kind === "session_revoke" ? "确认强制下线该登录会话？" : "确认更新用户权限与状态？"
        }
        okText="确认执行"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        confirmLoading={busy}
        onOk={() => void executeConfirmed()}
        onCancel={() => setConfirmAction(null)}
      >
        {confirmAction && (
          <Space orientation="vertical" size={10} style={{ width: "100%", marginTop: 8 }}>
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="操作对象">{confirmAction.targetLabel}</Descriptions.Item>
              <Descriptions.Item label="变更内容">{confirmAction.changeSummary}</Descriptions.Item>
            </Descriptions>
            {confirmAction.kind === "access_update" && confirmAction.status === "deleted" && (
              <Alert
                type="warning"
                showIcon
                title="账户数据不会被物理清空，但该用户将无法再登录且所有授权立即失效。"
              />
            )}
          </Space>
        )}
      </Modal>
    </section>
  );
}
