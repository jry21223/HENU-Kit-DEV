import {
  CheckOutlined,
  CloseOutlined,
  EditOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Col,
  Drawer,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Switch,
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
  executeFoodCommand,
  fetchFoodWorkspace,
  resolveFoodOperation,
  type FoodAnomalyTicket,
  type FoodCampus,
  type FoodCommand,
  type FoodCommandKind,
  type FoodCommandPayload,
  type FoodOperationLookupResult,
  type FoodPost,
  type FoodPostTier,
  type FoodSubmission,
  type FoodTierAdjustment,
  type FoodWorkspace,
} from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import {
  clearPendingFoodOperation,
  readPendingFoodOperation,
  writePendingFoodOperation,
  type PendingFoodOperation,
} from "@/lib/pending-operations";

const CAMPUS_OPTIONS = [
  { label: "全部校区", value: "" },
  { label: "明伦校区", value: "minglun" },
  { label: "金明校区", value: "jinming" },
  { label: "龙子湖校区", value: "longzihu" },
];

const POST_TIER_OPTIONS: Array<{ label: string; value: FoodPostTier; color: string }> = [
  { label: "夯", value: "hang", color: "volcano" },
  { label: "顶级", value: "top", color: "gold" },
  { label: "人上人", value: "elite", color: "blue" },
  { label: "NPC", value: "npc", color: "default" },
  { label: "拉完了", value: "bad", color: "red" },
];

function campusLabel(campus: FoodCampus | null): string {
  return campus === "minglun"
    ? "明伦校区"
    : campus === "jinming"
      ? "金明校区"
      : campus === "longzihu"
        ? "龙子湖校区"
        : "未分配";
}

function postTierInfo(tier: FoodPostTier) {
  return (
    POST_TIER_OPTIONS.find((item) => item.value === tier) ?? {
      label: tier,
      value: tier,
      color: "default",
    }
  );
}

function anomalyKindLabel(kind: FoodAnomalyTicket["kind"]): string {
  return {
    duplicate: "重复投稿",
    spam: "垃圾信息",
    quality: "质量问题",
    location: "位置有误",
  }[kind] ?? kind;
}

function severityBadge(severity: FoodAnomalyTicket["severity"]) {
  if (severity === "high") return <Tag color="error">高</Tag>;
  if (severity === "medium") return <Tag color="warning">中</Tag>;
  return <Tag color="default">低</Tag>;
}

function adjustTierLabel(tier: FoodTierAdjustment["current_tier"]): string {
  return {
    featured: "精选",
    recommended: "推荐",
    standard: "标准",
    watch: "观察",
  }[tier] ?? tier;
}

const SUCCESS_TEXT: Record<FoodCommandKind, string> = {
  submission_approve: "投稿已审核通过。",
  submission_reject: "投稿已拒绝。",
  submission_edit: "投稿信息已保存。",
  post_edit: "点评修改已生效。",
  anomaly_resolve: "异常举报已标记为已处理。",
  anomaly_dismiss: "异常举报已驳回。",
  tier_adjustment_confirm: "调档已确认生效。",
  tier_adjustment_reject: "调档申请已拒绝。",
};

type ConfirmAction = {
  kind: FoodCommandKind;
  resourceId: string;
  version: number;
  title: string;
  subject: string;
  confirmLabel: string;
  danger?: boolean;
  extraPayload?: Partial<FoodCommandPayload>;
};

export function FoodPage() {
  const { message } = AntApp.useApp();
  const { authState, operatorId, permissions } = useConsole();
  const [searchParams, setSearchParams] = useSearchParams();

  const initialCampus = (() => {
    const val = searchParams.get("campus");
    return val === "minglun" || val === "jinming" || val === "longzihu" ? val : "";
  })();

  const [campusFilter, setCampusFilter] = useState<FoodCampus | "">(initialCampus);
  const [keyword, setKeyword] = useState("");
  const [activeTab, setActiveTab] = useState("submissions");
  const [state, setState] = useState<"loading" | "ready" | "denied" | "unavailable">("loading");
  const [workspace, setWorkspace] = useState<FoodWorkspace>();
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<PendingFoodOperation | undefined>();

  // 弹窗与抽屉状态
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);
  const [confirmReason, setConfirmReason] = useState("");
  const [editingSubmission, setEditingSubmission] = useState<FoodSubmission | null>(null);
  const [editingPost, setEditingPost] = useState<FoodPost | null>(null);
  const [submissionForm] = Form.useForm();
  const [postForm] = Form.useForm();

  const mountedRef = useRef(true);
  const requestSeqRef = useRef(0);

  const canReview = permissions.includes("food.review");
  const canHandleAnomaly = permissions.includes("food.anomaly");
  const canAdjustTier = permissions.includes("food.tier_adjust");

  const canQueryPending = useMemo(() => {
    if (!pending || authState !== "authenticated" || pending.operator_id !== operatorId) {
      return false;
    }
    if (pending.operation.startsWith("anomaly_")) return canHandleAnomaly;
    if (pending.operation.startsWith("tier_adjustment_")) return canAdjustTier;
    return canReview;
  }, [pending, authState, operatorId, canHandleAnomaly, canAdjustTier, canReview]);

  const persistPending = useCallback((value?: PendingFoodOperation) => {
    if (value && !writePendingFoodOperation(value)) return false;
    if (!value) clearPendingFoodOperation();
    setPending(value);
    return true;
  }, []);

  const refresh = useCallback(
    async (campus: FoodCampus | "" = campusFilter) => {
      if (authState !== "authenticated" || !operatorId) {
        setState(authState === "denied" ? "denied" : "unavailable");
        return;
      }
      const seq = ++requestSeqRef.current;
      setState("loading");
      const result = await fetchFoodWorkspace(campus || undefined);
      if (!mountedRef.current || seq !== requestSeqRef.current) return;
      if (result.state === "authenticated") {
        setWorkspace(result.workspace);
        setState("ready");
      } else {
        setState(result.state === "denied" ? "denied" : "unavailable");
      }
    },
    [authState, operatorId, campusFilter],
  );

  const pollOperation = useCallback(
    async (operation: PendingFoodOperation): Promise<FoodOperationLookupResult | undefined> => {
      for (let attempt = 0; attempt < 3; attempt++) {
        if (attempt > 0) await new Promise((r) => setTimeout(r, attempt * 250));
        if (!mountedRef.current) return undefined;
        const res = await resolveFoodOperation(operation.operation, operation.idempotency_key);
        if (!mountedRef.current) return undefined;
        if (res.state === "unknown" || res.state === "unavailable") continue;
        return res;
      }
      return { state: "unavailable" };
    },
    [],
  );

  const applyOutcome = useCallback(
    async (
      result: FoodOperationLookupResult,
      operation: PendingFoodOperation,
      fromLookup: boolean,
    ) => {
      if (
        fromLookup &&
        (result.state === "denied" ||
          result.state === "signed_out" ||
          result.state === "invalid" ||
          result.state === "conflict")
      ) {
        message.warning(
          result.state === "denied"
            ? "当前账户无法查询原操作结果，请联系管理员核对。"
            : result.state === "signed_out"
              ? "请重新登录后再查询原操作结果。"
              : "原操作结果暂无法确认，请稍后再查。",
        );
        return;
      }
      if (
        fromLookup &&
        result.state === "failed" &&
        result.result.operation === operation.operation &&
        result.result.resource_id === operation.resource_id
      ) {
        persistPending();
        message.warning("内容已被其他人更新，原操作未生效，请刷新核对后重新操作。");
        await refresh();
        return;
      }
      if (
        result.state === "succeeded" &&
        result.result?.operation === operation.operation &&
        result.result.resource_id === operation.resource_id
      ) {
        persistPending();
        message.success(SUCCESS_TEXT[operation.operation]);
        await refresh();
      } else if (result.state === "conflict") {
        persistPending();
        message.warning("数据刚刚被其他人更新，已为你刷新最新列表，请核对后重试。");
        await refresh();
      } else if (result.state === "denied") {
        persistPending();
        message.error("当前账户缺少该项操作权限。");
      } else if (result.state === "invalid") {
        persistPending();
        message.error("提交内容不符合要求，请检查后重试。");
      } else if (result.state === "signed_out") {
        persistPending();
        message.error("登录状态已过期，请重新登录。");
      } else {
        message.warning("操作结果尚未确认，已保留记录；请点击上方横幅查询，请勿重复提交。");
      }
    },
    [message, persistPending, refresh],
  );

  const resolvePending = useCallback(
    async (target?: PendingFoodOperation) => {
      const op = target ?? pending;
      if (!op || busy || !operatorId || op.operator_id !== operatorId) return;
      setBusy(true);
      const res = await pollOperation(op);
      if (!mountedRef.current) return;
      if (res) await applyOutcome(res, op, true);
      if (mountedRef.current) setBusy(false);
    },
    [pending, busy, operatorId, pollOperation, applyOutcome],
  );

  useEffect(() => {
    mountedRef.current = true;
    if (authState === "authenticated" && operatorId) {
      const saved = readPendingFoodOperation(operatorId);
      setPending(saved);
      void refresh();
      if (saved) void resolvePending(saved);
    }
    return () => {
      mountedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authState, operatorId]);

  const handleCampusChange = (value: FoodCampus | "") => {
    setCampusFilter(value);
    const next = new URLSearchParams(searchParams);
    if (value) next.set("campus", value);
    else next.delete("campus");
    setSearchParams(next, { replace: true });
    void refresh(value);
  };

  const runCommand = async (kind: FoodCommandKind, input: FoodCommand): Promise<boolean> => {
    if (busy || pending || authState !== "authenticated" || !operatorId) return false;
    const key = `idem_food_${kind}_${crypto.randomUUID()}`;
    const operation: PendingFoodOperation = {
      version: 1,
      operator_id: operatorId,
      operation: kind,
      idempotency_key: key,
      resource_id: input.resource_id,
    };
    if (!persistPending(operation)) {
      message.error("浏览器暂不允许会话存储，无法安全提交，请检查隐私设置后重试。");
      return false;
    }
    setBusy(true);
    let fromLookup = false;
    let res: FoodOperationLookupResult | undefined = await executeFoodCommand(input, key);
    if (!mountedRef.current) return false;
    if (res.state === "unknown") {
      fromLookup = true;
      res = await pollOperation(operation);
    }
    if (!res || !mountedRef.current) return false;
    const succeeded = res.state === "succeeded";
    await applyOutcome(res, operation, fromLookup);
    if (mountedRef.current) setBusy(false);
    return succeeded;
  };

  const handleConfirmSubmit = async () => {
    if (!confirmAction) return;
    const note = confirmReason.trim();
    if (note.length < 2) {
      message.warning("请至少填写 2 个字的操作理由。");
      return;
    }
    const ok = await runCommand(confirmAction.kind, {
      kind: confirmAction.kind,
      resource_id: confirmAction.resourceId,
      expected_version: confirmAction.version,
      payload: { note, ...confirmAction.extraPayload },
    });
    if (ok) {
      setConfirmAction(null);
      setConfirmReason("");
    }
  };

  const openEditSubmission = (item: FoodSubmission) => {
    setEditingSubmission(item);
    submissionForm.setFieldsValue({
      venue_name: item.venue_name,
      item_name: item.item_name,
      description: item.description,
      campus: item.campus ?? "",
      note: "",
    });
  };

  const handleSaveSubmission = async () => {
    if (!editingSubmission) return;
    const values = await submissionForm.validateFields();
    const payload: FoodCommandPayload = { note: String(values.note).trim() };
    const venueName = String(values.venue_name).trim();
    const itemName = String(values.item_name).trim();
    const description = String(values.description ?? "");
    const campus = values.campus as FoodCampus | "";
    if (venueName !== editingSubmission.venue_name) payload.venue_name = venueName;
    if (itemName !== editingSubmission.item_name) payload.item_name = itemName;
    if (description !== editingSubmission.description) payload.description = description;
    if (campus && campus !== (editingSubmission.campus ?? "")) payload.campus = campus;

    if (
      !("venue_name" in payload) &&
      !("item_name" in payload) &&
      !("description" in payload) &&
      !("campus" in payload)
    ) {
      message.info("内容未发生变更，无需保存。");
      return;
    }
    const ok = await runCommand("submission_edit", {
      kind: "submission_edit",
      resource_id: editingSubmission.id,
      expected_version: editingSubmission.version,
      payload,
    });
    if (ok) setEditingSubmission(null);
  };

  const openEditPost = (item: FoodPost) => {
    setEditingPost(item);
    postForm.setFieldsValue({
      venue_name: item.venue_name,
      campus: item.campus,
      tier: item.tier,
      review_text: item.review_text,
      price_reference: item.price_reference,
      hours_reference: item.hours_reference,
      hidden: item.hidden,
      note: "",
    });
  };

  const handleSavePost = async () => {
    if (!editingPost) return;
    const values = await postForm.validateFields();
    const payload: FoodCommandPayload = { note: String(values.note).trim() };
    const venueName = String(values.venue_name).trim();
    const reviewText = String(values.review_text).trim();
    const priceRef = String(values.price_reference ?? "");
    const hoursRef = String(values.hours_reference ?? "");
    if (venueName !== editingPost.venue_name) payload.venue_name = venueName;
    if (values.campus !== editingPost.campus) payload.campus = values.campus;
    if (values.tier !== editingPost.tier) payload.tier = values.tier;
    if (reviewText !== editingPost.review_text) payload.review_text = reviewText;
    if (priceRef !== editingPost.price_reference) payload.price_reference = priceRef;
    if (hoursRef !== editingPost.hours_reference) payload.hours_reference = hoursRef;
    if (Boolean(values.hidden) !== editingPost.hidden) payload.hidden = Boolean(values.hidden);

    if (Object.keys(payload).length <= 1) {
      message.info("内容未发生变更，无需保存。");
      return;
    }
    const ok = await runCommand("post_edit", {
      kind: "post_edit",
      resource_id: editingPost.id,
      expected_version: editingPost.version,
      payload,
    });
    if (ok) setEditingPost(null);
  };

  const q = keyword.trim().toLocaleLowerCase();

  const filteredSubmissions = useMemo(
    () =>
      (workspace?.submissions ?? []).filter(
        (item) =>
          !q ||
          item.venue_name.toLocaleLowerCase().includes(q) ||
          item.item_name.toLocaleLowerCase().includes(q) ||
          item.description.toLocaleLowerCase().includes(q),
      ),
    [workspace, q],
  );

  const filteredPosts = useMemo(
    () =>
      (workspace?.posts ?? []).filter(
        (item) =>
          !q ||
          item.venue_name.toLocaleLowerCase().includes(q) ||
          item.review_text.toLocaleLowerCase().includes(q) ||
          item.author_display_name.toLocaleLowerCase().includes(q),
      ),
    [workspace, q],
  );

  const filteredAnomalies = useMemo(
    () =>
      (workspace?.anomaly_tickets ?? []).filter(
        (item) =>
          !q ||
          item.venue_name.toLocaleLowerCase().includes(q) ||
          item.details.toLocaleLowerCase().includes(q),
      ),
    [workspace, q],
  );

  const filteredTiers = useMemo(
    () =>
      (workspace?.tier_adjustments ?? []).filter(
        (item) =>
          !q ||
          item.venue_name.toLocaleLowerCase().includes(q) ||
          item.reason.toLocaleLowerCase().includes(q),
      ),
    [workspace, q],
  );

  const submissionColumns: ColumnsType<FoodSubmission> = [
    {
      title: "店铺与菜品",
      key: "venue",
      render: (_, item) => (
        <div>
          <Typography.Text strong>{item.venue_name}</Typography.Text>
          {item.item_name ? (
            <Typography.Text type="secondary"> · {item.item_name}</Typography.Text>
          ) : null}
          <div style={{ marginTop: 4, fontSize: 13, color: "#475569" }}>
            {item.description || "无补充描述"}
          </div>
        </div>
      ),
    },
    {
      title: "校区",
      dataIndex: "campus",
      width: 110,
      render: (campus: FoodCampus | null) => <Tag>{campusLabel(campus)}</Tag>,
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 96,
      render: (status: string) =>
        status === "pending" ? <Tag color="processing">待审核</Tag> : <Tag>{status}</Tag>,
    },
    {
      title: "操作",
      key: "actions",
      width: 240,
      render: (_, item) =>
        canReview && item.status === "pending" ? (
          <Space size={6} wrap>
            <Button
              type="primary"
              size="small"
              icon={<CheckOutlined />}
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "submission_approve",
                  resourceId: item.id,
                  version: item.version,
                  title: "批准投稿上架",
                  subject: `${item.venue_name} · ${item.item_name}`,
                  confirmLabel: "确认批准",
                });
              }}
            >
              批准
            </Button>
            <Button
              size="small"
              icon={<EditOutlined />}
              disabled={busy || Boolean(pending)}
              onClick={() => openEditSubmission(item)}
            >
              编辑
            </Button>
            <Button
              danger
              size="small"
              icon={<CloseOutlined />}
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "submission_reject",
                  resourceId: item.id,
                  version: item.version,
                  title: "拒绝该条投稿",
                  subject: `${item.venue_name} · ${item.item_name}`,
                  confirmLabel: "确认拒绝",
                  danger: true,
                });
              }}
            >
              拒绝
            </Button>
          </Space>
        ) : (
          <Typography.Text type="secondary">{canReview ? "已处理" : "仅查看"}</Typography.Text>
        ),
    },
  ];

  const postColumns: ColumnsType<FoodPost> = [
    {
      title: "店铺与点评",
      key: "post",
      render: (_, item) => (
        <div>
          <Space size={6} wrap>
            <Typography.Text strong>{item.venue_name}</Typography.Text>
            <Tag color={postTierInfo(item.tier).color}>{postTierInfo(item.tier).label}</Tag>
            <Tag>{campusLabel(item.campus)}</Tag>
            {item.hidden && <Tag color="default">已隐藏</Tag>}
          </Space>
          <div style={{ marginTop: 4, fontSize: 13, color: "#475569" }}>{item.review_text}</div>
          <div style={{ marginTop: 4, fontSize: 12, color: "#94a3b8" }}>
            作者：{item.author_display_name}
            {item.price_reference ? ` · 人均/价格：${item.price_reference}` : ""}
            {item.hours_reference ? ` · 营业时间：${item.hours_reference}` : ""}
          </div>
        </div>
      ),
    },
    {
      title: "公开状态",
      dataIndex: "hidden",
      width: 100,
      render: (hidden: boolean) =>
        hidden ? <Tag color="warning">已隐藏</Tag> : <Tag color="success">公开展示</Tag>,
    },
    {
      title: "操作",
      key: "actions",
      width: 190,
      render: (_, item) =>
        canReview ? (
          <Space size={6}>
            <Button
              size="small"
              icon={<EditOutlined />}
              disabled={busy || Boolean(pending)}
              onClick={() => openEditPost(item)}
            >
              编辑
            </Button>
            <Button
              size="small"
              danger={!item.hidden}
              icon={item.hidden ? <EyeOutlined /> : <EyeInvisibleOutlined />}
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "post_edit",
                  resourceId: item.id,
                  version: item.version,
                  title: item.hidden ? "恢复公开展示" : "隐藏该条点评",
                  subject: `${item.venue_name}（作者：${item.author_display_name}）`,
                  confirmLabel: item.hidden ? "确认恢复" : "确认隐藏",
                  danger: !item.hidden,
                  extraPayload: { hidden: !item.hidden },
                });
              }}
            >
              {item.hidden ? "恢复" : "隐藏"}
            </Button>
          </Space>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const anomalyColumns: ColumnsType<FoodAnomalyTicket> = [
    {
      title: "涉及店铺与问题",
      key: "anomaly",
      render: (_, item) => (
        <div>
          <Space size={6}>
            <Typography.Text strong>{item.venue_name}</Typography.Text>
            <Tag>{anomalyKindLabel(item.kind)}</Tag>
            {severityBadge(item.severity)}
          </Space>
          <div style={{ marginTop: 4, fontSize: 13, color: "#475569" }}>{item.details}</div>
        </div>
      ),
    },
    {
      title: "操作",
      key: "actions",
      width: 200,
      render: (_, item) =>
        canHandleAnomaly && item.status === "open" ? (
          <Space size={6}>
            <Button
              type="primary"
              size="small"
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "anomaly_resolve",
                  resourceId: item.id,
                  version: item.version,
                  title: "标记异常已处理",
                  subject: `${item.venue_name} · ${anomalyKindLabel(item.kind)}`,
                  confirmLabel: "确认已处理",
                });
              }}
            >
              标记已处理
            </Button>
            <Button
              size="small"
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "anomaly_dismiss",
                  resourceId: item.id,
                  version: item.version,
                  title: "驳回异常举报",
                  subject: `${item.venue_name} · ${anomalyKindLabel(item.kind)}`,
                  confirmLabel: "确认驳回",
                });
              }}
            >
              驳回
            </Button>
          </Space>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const tierColumns: ColumnsType<FoodTierAdjustment> = [
    {
      title: "店铺",
      dataIndex: "venue_name",
      render: (name: string) => <Typography.Text strong>{name}</Typography.Text>,
    },
    {
      title: "档位变化",
      key: "change",
      width: 200,
      render: (_, item) => (
        <Space size={6}>
          <Tag>{adjustTierLabel(item.current_tier)}</Tag>
          <span>→</span>
          <Tag color="blue">{adjustTierLabel(item.proposed_tier)}</Tag>
        </Space>
      ),
    },
    {
      title: "调整理由",
      dataIndex: "reason",
    },
    {
      title: "操作",
      key: "actions",
      width: 190,
      render: (_, item) =>
        canAdjustTier && item.status === "pending" ? (
          <Space size={6}>
            <Button
              type="primary"
              size="small"
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "tier_adjustment_confirm",
                  resourceId: item.id,
                  version: item.version,
                  title: "确认档位调整",
                  subject: `${item.venue_name}（${adjustTierLabel(item.current_tier)} → ${adjustTierLabel(item.proposed_tier)}）`,
                  confirmLabel: "确认调档",
                });
              }}
            >
              确认调档
            </Button>
            <Button
              size="small"
              disabled={busy || Boolean(pending)}
              onClick={() => {
                setConfirmReason("");
                setConfirmAction({
                  kind: "tier_adjustment_reject",
                  resourceId: item.id,
                  version: item.version,
                  title: "拒绝档位调整",
                  subject: item.venue_name,
                  confirmLabel: "确认拒绝",
                });
              }}
            >
              拒绝
            </Button>
          </Space>
        ) : (
          <Typography.Text type="secondary">仅查看</Typography.Text>
        ),
    },
  ];

  const visiblePosts = workspace?.posts.filter((p) => !p.hidden).length ?? 0;
  const hiddenPosts = workspace?.posts.filter((p) => p.hidden).length ?? 0;

  return (
    <section aria-labelledby="food-heading">
      <div className="console-page-header">
        <div>
          <h1 id="food-heading" className="console-page-title">
            美食榜治理
          </h1>
          <p className="console-page-subtitle">
            审核新投稿、维护已发布点评、处理异常举报与档位调整。校区筛选作用于投稿与点评队列。
          </p>
        </div>
        <Space wrap>
          <Select
            aria-label="校区筛选"
            style={{ width: 140 }}
            value={campusFilter}
            options={CAMPUS_OPTIONS}
            onChange={handleCampusChange}
          />
          <Button icon={<ReloadOutlined />} loading={state === "loading"} onClick={() => void refresh()}>
            刷新
          </Button>
        </Space>
      </div>

      <PendingRecoveryAlert
        visible={Boolean(pending)}
        description="刚才的美食操作在网络返回前中断，系统已保留操作编号。点击右侧按钮核对最新状态，不会重复执行。"
        busy={busy}
        disabled={!canQueryPending}
        onResolve={() => void resolvePending()}
      />

      {state !== "ready" ? (
        <ModuleStateView
          state={state}
          loadingText="正在读取美食治理队列…"
          deniedTitle="没有美食榜治理权限"
          onRetry={() => void refresh()}
        />
      ) : workspace ? (
        <>
          {workspace.stale && (
            <Alert
              type="warning"
              showIcon
              title="当前展示的是最近一次缓存快照"
              description={workspace.status_message || "实时数据暂时不可用，请稍后刷新核对。"}
              style={{ marginBottom: 16 }}
            />
          )}

          <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
            <Col xs={12} sm={6}>
              <Card size="small">
                <Statistic title="待审投稿" value={workspace.submissions.length} />
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small">
                <Statistic
                  title="已发布点评"
                  value={workspace.posts.length}
                  suffix={
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      （公开 {visiblePosts} / 隐藏 {hiddenPosts}）
                    </Typography.Text>
                  }
                />
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small">
                <Statistic title="待处理异常" value={workspace.anomaly_tickets.length} />
              </Card>
            </Col>
            <Col xs={12} sm={6}>
              <Card size="small">
                <Statistic title="待确认调档" value={workspace.tier_adjustments.length} />
              </Card>
            </Col>
          </Row>

          <Card>
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 12,
                marginBottom: 12,
              }}
            >
              <Input
                allowClear
                prefix={<SearchOutlined />}
                placeholder="按店名、菜品、点评内容或作者检索…"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                style={{ maxWidth: 340 }}
              />
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                数据更新时间：{localDateTime(workspace.as_of)}（每类最多载入 200 条）
              </Typography.Text>
            </div>

            <Tabs
              activeKey={activeTab}
              onChange={setActiveTab}
              items={[
                {
                  key: "submissions",
                  label: `待审投稿 (${filteredSubmissions.length})`,
                  children: (
                    <Table
                      rowKey="id"
                      columns={submissionColumns}
                      dataSource={filteredSubmissions}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 12, showSizeChanger: false }}
                      locale={{ emptyText: "当前筛选下没有待审核投稿" }}
                    />
                  ),
                },
                {
                  key: "posts",
                  label: `已发布点评 (${filteredPosts.length})`,
                  children: (
                    <Table
                      rowKey="id"
                      columns={postColumns}
                      dataSource={filteredPosts}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 12, showSizeChanger: false }}
                      locale={{ emptyText: "当前筛选下没有已发布点评" }}
                    />
                  ),
                },
                {
                  key: "anomalies",
                  label: `异常举报 (${filteredAnomalies.length})`,
                  children: (
                    <Table
                      rowKey="id"
                      columns={anomalyColumns}
                      dataSource={filteredAnomalies}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 12, showSizeChanger: false }}
                      locale={{ emptyText: "当前没有待处理的异常举报" }}
                    />
                  ),
                },
                {
                  key: "tiers",
                  label: `调档确认 (${filteredTiers.length})`,
                  children: (
                    <Table
                      rowKey="id"
                      columns={tierColumns}
                      dataSource={filteredTiers}
                      scroll={{ x: "max-content" }}
                      pagination={{ pageSize: 12, showSizeChanger: false }}
                      locale={{ emptyText: "当前没有待确认的调档申请" }}
                    />
                  ),
                },
              ]}
            />
          </Card>
        </>
      ) : null}

      {/* 统一二次确认弹窗 */}
      <Modal
        open={Boolean(confirmAction)}
        title={confirmAction?.title}
        okText={confirmAction?.confirmLabel ?? "确认"}
        cancelText="取消"
        okButtonProps={{
          danger: confirmAction?.danger,
          loading: busy,
          disabled: confirmReason.trim().length < 2,
        }}
        onOk={() => void handleConfirmSubmit()}
        onCancel={() => {
          setConfirmAction(null);
          setConfirmReason("");
        }}
      >
        <Space orientation="vertical" size={12} style={{ width: "100%", marginTop: 8 }}>
          <Typography.Text>
            操作对象：<strong>{confirmAction?.subject}</strong>
          </Typography.Text>
          <div>
            <Typography.Text type="secondary" style={{ display: "block", marginBottom: 6 }}>
              请填写操作理由（至少 2 个字，系统会记录审核日志）
            </Typography.Text>
            <Input.TextArea
              rows={3}
              maxLength={1000}
              showCount
              style={{ marginBottom: 24 }}
              placeholder="例如：信息核实无误允许上架 / 重复投稿 / 含不实信息"
              value={confirmReason}
              onChange={(e) => setConfirmReason(e.target.value)}
            />
          </div>
        </Space>
      </Modal>

      {/* 编辑待审投稿抽屉 */}
      <Drawer
        open={Boolean(editingSubmission)}
        title="编辑投稿信息"
        size={480}
        onClose={() => setEditingSubmission(null)}
        extra={
          <Space>
            <Button onClick={() => setEditingSubmission(null)}>取消</Button>
            <Button type="primary" loading={busy} onClick={() => void handleSaveSubmission()}>
              保存修改
            </Button>
          </Space>
        }
      >
        <Form form={submissionForm} layout="vertical">
          <Form.Item
            name="venue_name"
            label="店名"
            rules={[{ required: true, whitespace: true, message: "请输入店名" }]}
          >
            <Input maxLength={160} />
          </Form.Item>
          <Form.Item
            name="item_name"
            label="推荐菜品"
            rules={[{ required: true, whitespace: true, message: "请输入菜名" }]}
          >
            <Input maxLength={160} />
          </Form.Item>
          <Form.Item name="campus" label="所属校区">
            <Select options={[{ label: "未分配", value: "" }, ...CAMPUS_OPTIONS.slice(1)]} />
          </Form.Item>
          <Form.Item name="description" label="描述说明">
            <Input.TextArea rows={4} maxLength={2000} showCount />
          </Form.Item>
          <Form.Item
            name="note"
            label="本次修改理由"
            rules={[
              { required: true, whitespace: true, message: "请填写修改理由" },
              { min: 2, message: "理由至少 2 个字" },
            ]}
          >
            <Input.TextArea rows={2} maxLength={1000} placeholder="说明修正了哪些信息" />
          </Form.Item>
        </Form>
      </Drawer>

      {/* 编辑已发布点评抽屉 */}
      <Drawer
        open={Boolean(editingPost)}
        title="编辑已发布点评"
        size={520}
        onClose={() => setEditingPost(null)}
        extra={
          <Space>
            <Button onClick={() => setEditingPost(null)}>取消</Button>
            <Button type="primary" loading={busy} onClick={() => void handleSavePost()}>
              保存修改
            </Button>
          </Space>
        }
      >
        <Form form={postForm} layout="vertical">
          <Form.Item
            name="venue_name"
            label="店名"
            rules={[{ required: true, whitespace: true, message: "请输入店名" }]}
          >
            <Input maxLength={160} />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="campus" label="所属校区" rules={[{ required: true }]}>
                <Select options={CAMPUS_OPTIONS.slice(1)} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="tier" label="榜单档位" rules={[{ required: true }]}>
                <Select
                  options={POST_TIER_OPTIONS.map((t) => ({ label: t.label, value: t.value }))}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item
            name="review_text"
            label="点评正文"
            rules={[
              { required: true, whitespace: true, message: "请输入点评内容" },
              { min: 2, message: "点评至少 2 个字" },
            ]}
          >
            <Input.TextArea rows={5} maxLength={2000} showCount />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="price_reference" label="价格参考">
                <Input maxLength={200} placeholder="例如：人均 15 元" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="hours_reference" label="营业时间">
                <Input maxLength={200} placeholder="例如：10:30-21:00" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="hidden" label="从公开榜单隐藏" valuePropName="checked">
            <Switch checkedChildren="已隐藏" unCheckedChildren="公开" />
          </Form.Item>
          <Form.Item
            name="note"
            label="本次修改理由"
            rules={[
              { required: true, whitespace: true, message: "请填写修改理由" },
              { min: 2, message: "理由至少 2 个字" },
            ]}
          >
            <Input.TextArea rows={2} maxLength={1000} placeholder="说明修正原因以便审计核查" />
          </Form.Item>
        </Form>
      </Drawer>
    </section>
  );
}
