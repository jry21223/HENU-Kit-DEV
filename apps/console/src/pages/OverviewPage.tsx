import {
  ArrowRightOutlined,
  BellOutlined,
  BookOutlined,
  CheckCircleOutlined,
  CompassOutlined,
  CrownOutlined,
  ExperimentOutlined,
  MessageOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ShopOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import {
  Alert,
  Button,
  Card,
  Col,
  Collapse,
  Descriptions,
  Row,
  Space,
  Statistic,
  Tag,
  Typography,
} from "antd";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ModuleStateView } from "@/components/ModuleStateView";
import { moduleSummaries, type ModuleSummary } from "@/data/modules";
import { hasConsoleAccess, useConsole } from "@/lib/console-context";
import {
  fetchAccountTicketFacts,
  fetchFoodFacts,
  fetchNoticeSnapshot,
  fetchPlatformOperations,
  type ConsoleAccountTicketFacts,
  type FoodFacts,
} from "@/lib/console-gateway";
import { readCatalogReadiness, type CatalogReadiness } from "@/lib/console-extensions";
import { localDateTime } from "@/lib/format";

type ReadState = "loading" | "ready" | "denied" | "unavailable";

function boundedRead<T extends { state: string }>(
  controllers: Set<AbortController>,
  read: (signal: AbortSignal) => Promise<T>,
): Promise<T | { state: "unavailable" }> {
  const controller = new AbortController();
  controllers.add(controller);
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result: T | { state: "unavailable" }) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      controller.signal.removeEventListener("abort", onAbort);
      controllers.delete(controller);
      resolve(result);
    };
    const onAbort = () => finish({ state: "unavailable" });
    const timer = setTimeout(() => controller.abort(), 2000);
    controller.signal.addEventListener("abort", onAbort, { once: true });
    void read(controller.signal).then(finish, () => finish({ state: "unavailable" }));
  });
}

function diagnosticTagColor(status: ModuleSummary["status"]): string {
  switch (status) {
    case "ok":
      return "success";
    case "loading":
      return "default";
    case "empty":
      return "processing";
    case "partial":
    case "stale":
      return "warning";
    case "unavailable":
      return "error";
    default:
      return "default";
  }
}

function diagnosticLabel(summary: ModuleSummary): string {
  if (summary.unavailableReason === "operator_disabled") return "已停用";
  if (summary.unavailableReason === "not_onboarded") return "未接入";
  return {
    loading: "检查中",
    ok: "运行正常",
    empty: "暂无数据",
    partial: "部分可用",
    stale: "使用缓存",
    unavailable: "暂不可用",
    denied: "无权限",
  }[summary.status];
}

export function OverviewPage() {
  const navigate = useNavigate();
  const {
    permissions,
    scopes,
    overview,
    overviewState,
    authorizedNav,
    refreshOverview,
  } = useConsole();

  const [foodState, setFoodState] = useState<ReadState>("loading");
  const [food, setFood] = useState<Pick<FoodFacts, "counts" | "as_of">>();
  const [ticketState, setTicketState] = useState<ReadState>("loading");
  const [tickets, setTickets] = useState<Pick<ConsoleAccountTicketFacts, "counts" | "as_of">>();
  const [platformState, setPlatformState] = useState<ReadState>("loading");
  const [platform, setPlatform] = useState<{
    unresolved: number;
    returned: number;
    hasMore: boolean;
    deadLetters: number;
    updatedAt: string;
  }>();
  const [noticeState, setNoticeState] = useState<ReadState | "not_configured">("loading");
  const [notice, setNotice] = useState<{
    pending: number;
    approved: number;
    queued: number;
    delivered: number;
    returned: number;
    updatedAt: string;
  }>();
  const [libraryState, setLibraryState] = useState<ReadState>("loading");
  const [library, setLibrary] = useState<CatalogReadiness>();

  const mountedRef = useRef(true);
  const controllersRef = useRef(new Set<AbortController>());
  const loadSeqRef = useRef(0);

  const canFood = hasConsoleAccess(permissions, scopes, "food.read", "food");
  const canTickets = hasConsoleAccess(permissions, scopes, "account.tickets.read", "account-portfolio");
  const canPlatform = hasConsoleAccess(permissions, scopes, "platform.operations.read");
  const canNotice = hasConsoleAccess(permissions, scopes, "notice.read", "notice");
  const canLibrary = hasConsoleAccess(permissions, scopes, "library.read", "library");
  const canCareer = hasConsoleAccess(permissions, scopes, "career.read", "career");
  const canCampus = hasConsoleAccess(permissions, scopes, "campus.read", "campus");
  const canPractice = hasConsoleAccess(permissions, scopes, "quizcraft.workshop.read", "quizcraft");
  const canMembership = hasConsoleAccess(permissions, scopes, "account.membership.write", "account-portfolio");
  const canPoints = hasConsoleAccess(permissions, scopes, "account.points.adjust", "account-portfolio");

  const loadFacts = useCallback(async () => {
    for (const c of controllersRef.current) c.abort();
    controllersRef.current.clear();
    const seq = ++loadSeqRef.current;

    setFoodState(canFood ? "loading" : "denied");
    setTicketState(canTickets ? "loading" : "denied");
    setPlatformState(canPlatform ? "loading" : "denied");
    setNoticeState(canNotice ? "loading" : "denied");
    setLibraryState(canLibrary ? "loading" : "denied");

    const overallTimer = setTimeout(() => {
      for (const c of controllersRef.current) c.abort();
    }, 3000);

    await Promise.all([
      (async () => {
        if (!canFood) return;
        const res = await boundedRead(controllersRef.current, (signal) =>
          fetchFoodFacts({ signal }),
        );
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        if (res.state === "authenticated") {
          setFood({ counts: res.facts.counts, as_of: res.facts.as_of });
          setFoodState("ready");
        } else {
          setFoodState(res.state === "denied" ? "denied" : "unavailable");
        }
      })(),
      (async () => {
        if (!canTickets) return;
        const res = await boundedRead(controllersRef.current, (signal) =>
          fetchAccountTicketFacts("unresolved", { signal }),
        );
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        if (res.state === "authenticated") {
          setTickets({ counts: res.facts.counts, as_of: res.facts.as_of });
          setTicketState("ready");
        } else {
          setTicketState(res.state === "denied" ? "denied" : "unavailable");
        }
      })(),
      (async () => {
        if (!canPlatform) return;
        const res = await boundedRead(controllersRef.current, (signal) =>
          fetchPlatformOperations(undefined, { signal }),
        );
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        if (res.state === "authenticated") {
          const snap = res.operations;
          setPlatform({
            unresolved: snap.inbox_items.filter((i) =>
              ["open", "in_progress", "blocked"].includes(i.status),
            ).length,
            returned: snap.inbox_items.length,
            hasMore: snap.pagination.inbox_items.next_page !== null,
            deadLetters: snap.mail.dead_letters,
            updatedAt: snap.generated_at,
          });
          setPlatformState("ready");
        } else {
          setPlatformState(res.state === "denied" ? "denied" : "unavailable");
        }
      })(),
      (async () => {
        if (!canNotice) return;
        const res = await boundedRead(controllersRef.current, (signal) =>
          fetchNoticeSnapshot({ signal }),
        );
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        if (res.state === "authenticated") {
          const snap = res.snapshot;
          setNotice({
            pending: snap.items.filter((i) => i.state === "pending_review").length,
            approved: snap.items.filter((i) => i.state === "approved").length,
            queued: snap.items.filter((i) => i.distribution_status === "queued").length,
            delivered: snap.items.filter((i) => i.distribution_status === "delivered").length,
            returned: snap.items.length,
            updatedAt: snap.generated_at,
          });
          setNoticeState("ready");
        } else if (res.state === "not_configured") {
          setNoticeState("not_configured");
        } else {
          setNoticeState(res.state === "denied" ? "denied" : "unavailable");
        }
      })(),
      (async () => {
        if (!canLibrary) return;
        const res = await boundedRead(controllersRef.current, (signal) =>
          readCatalogReadiness(signal),
        );
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        if (res.state === "authenticated") {
          setLibrary(res.data);
          setLibraryState("ready");
        } else {
          setLibraryState(res.state === "denied" ? "denied" : "unavailable");
        }
      })(),
    ]);

    clearTimeout(overallTimer);
  }, [canFood, canTickets, canPlatform, canNotice, canLibrary]);

  useEffect(() => {
    mountedRef.current = true;
    void loadFacts();
    return () => {
      mountedRef.current = false;
      for (const c of controllersRef.current) c.abort();
      controllersRef.current.clear();
    };
  }, [loadFacts]);

  const handleRefreshAll = () => {
    void refreshOverview();
    void loadFacts();
  };

  const busy =
    overviewState === "loading" ||
    [foodState, ticketState, platformState, noticeState, libraryState].includes("loading");

  const summaries = useMemo<ModuleSummary[]>(() => {
    if (overviewState === "loading") {
      return moduleSummaries.map((item) => ({ ...item, status: "loading", metrics: [] }));
    }
    if (overviewState !== "ready" || !overview) {
      return moduleSummaries.map((item) => ({
        ...item,
        status: "unavailable",
        metrics: [],
        statusMessage: "摘要诊断暂不可用，不影响下方业务工作台使用。",
      }));
    }
    const grantCheck: Partial<Record<ModuleSummary["id"], boolean>> = {
      platform: canPlatform,
      notice: canNotice,
      library: canLibrary,
      food: canFood,
    };
    return moduleSummaries.map((pres) => {
      const allowed = grantCheck[pres.id];
      if (allowed === false) {
        return {
          ...pres,
          status: "denied",
          metrics: [],
          statusMessage: "当前账户无该模块查看权限。",
        };
      }
      const live = overview.modules.find((m) => m.id === pres.id);
      if (!live) {
        return { ...pres, status: "unavailable", metrics: [], statusMessage: "暂无返回数据。" };
      }
      return {
        ...pres,
        status: live.status,
        metrics: live.metrics,
        statusMessage: live.status_message,
        unavailableReason: live.unavailable_reason,
        asOf: live.as_of,
        lastSuccessAt: live.last_success_at,
        requestId: live.request_id,
      };
    });
  }, [overview, overviewState, canPlatform, canNotice, canLibrary, canFood]);

  if (overviewState === "denied") {
    const otherEntries = authorizedNav.filter((item) => item.permission !== "console.overview.read");
    return (
      <section aria-labelledby="overview-heading">
        <div className="console-page-header">
          <div>
            <h1 id="overview-heading" className="console-page-title">
              工作台总览
            </h1>
          </div>
        </div>
        <ModuleStateView
          state="denied"
          deniedTitle="当前账户没有总览查看权限"
          deniedDescription="你仍可以直接通过导航菜单或下方入口进入已获授权的业务工作台。"
          extra={
            otherEntries.length > 0 ? (
              <Space wrap>
                {otherEntries.map((item) => (
                  <Button key={item.path} onClick={() => navigate(item.path)}>
                    进入{item.label}
                  </Button>
                ))}
              </Space>
            ) : undefined
          }
        />
      </section>
    );
  }

  const foodPendingTotal =
    food
      ? food.counts.pending_submissions +
        food.counts.open_anomalies +
        food.counts.pending_tier_adjustments
      : 0;

  const allClear =
    foodState === "ready" &&
    ticketState === "ready" &&
    foodPendingTotal === 0 &&
    (tickets?.counts.unresolved ?? 0) === 0;

  const portalSummary = summaries.find((s) => s.id === "portal");

  return (
    <section aria-labelledby="overview-heading">
      <div className="console-page-header">
        <div>
          <h1 id="overview-heading" className="console-page-title">
            工作台总览
          </h1>
          <p className="console-page-subtitle">
            先处理待审核和未解决事项，再按需进入各业务模块工作台。
          </p>
        </div>
        <Button icon={<ReloadOutlined spin={busy} />} disabled={busy} onClick={handleRefreshAll}>
          刷新数据
        </Button>
      </div>

      {allClear && (
        <Alert
          type="success"
          showIcon
          icon={<CheckCircleOutlined />}
          title="当前没有紧急待办"
          description="美食审核队列与客服工单均已清空，可以安心处理日常维护工作。"
          style={{ marginBottom: 20 }}
        />
      )}

      {/* 核心待办区 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} md={8}>
          <Card
            hoverable={canFood}
            style={{ height: "100%" }}
            data-overview-task="food"
            data-state={foodState}
            title={
              <Space>
                <ShopOutlined />
                <span>美食内容审核</span>
              </Space>
            }
            extra={
              canFood ? (
                <Button
                  type={food && food.counts.pending_submissions > 0 ? "primary" : "default"}
                  size="small"
                  icon={<ArrowRightOutlined />}
                  iconPlacement="end"
                  onClick={() => navigate("/food")}
                >
                  去处理
                </Button>
              ) : null
            }
          >
            {foodState === "ready" && food ? (
              <Space orientation="vertical" size={8} style={{ width: "100%" }}>
                <Statistic
                  title="待审核投稿"
                  value={food.counts.pending_submissions}
                  suffix="条"
                  styles={{
                    content: {
                      color: food.counts.pending_submissions > 0 ? "#b45309" : "#15803d",
                    },
                  }}
                />
                <Space size={12} wrap>
                  <Typography.Text type="secondary">
                    异常举报 {food.counts.open_anomalies}
                  </Typography.Text>
                  <Typography.Text type="secondary">
                    待确认调档 {food.counts.pending_tier_adjustments}
                  </Typography.Text>
                </Space>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  更新于 {localDateTime(food.as_of)}
                </Typography.Text>
              </Space>
            ) : (
              <Typography.Text type="secondary">
                {foodState === "loading"
                  ? "正在加载待办数…"
                  : foodState === "denied"
                    ? "无权查看美食待办"
                    : "暂时无法读取待办数，可进入工作台核对"}
              </Typography.Text>
            )}
          </Card>
        </Col>

        <Col xs={24} md={8}>
          <Card
            hoverable={canTickets}
            style={{ height: "100%" }}
            data-overview-task="tickets"
            data-state={ticketState}
            title={
              <Space>
                <MessageOutlined />
                <span>客服工单</span>
              </Space>
            }
            extra={
              canTickets ? (
                <Button
                  type={tickets && tickets.counts.unresolved > 0 ? "primary" : "default"}
                  size="small"
                  icon={<ArrowRightOutlined />}
                  iconPlacement="end"
                  onClick={() => navigate("/account?status=unresolved")}
                >
                  查看未解决
                </Button>
              ) : null
            }
          >
            {ticketState === "ready" && tickets ? (
              <Space orientation="vertical" size={8} style={{ width: "100%" }}>
                <Statistic
                  title="未解决工单"
                  value={tickets.counts.unresolved}
                  suffix="条"
                  styles={{
                    content: {
                      color: tickets.counts.unresolved > 0 ? "#dc2626" : "#15803d",
                    },
                  }}
                />
                <Space size={12} wrap>
                  <Typography.Text type="secondary">
                    待处理 {tickets.counts.open}
                  </Typography.Text>
                  <Typography.Text type="secondary">
                    处理中 {tickets.counts.in_progress}
                  </Typography.Text>
                </Space>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  更新于 {localDateTime(tickets.as_of)}
                </Typography.Text>
              </Space>
            ) : (
              <Typography.Text type="secondary">
                {ticketState === "loading"
                  ? "正在加载工单数…"
                  : ticketState === "denied"
                    ? "无权查看工单"
                    : "暂时无法读取工单数，可进入工作台核对"}
              </Typography.Text>
            )}
          </Card>
        </Col>

        <Col xs={24} md={8}>
          <Card
            hoverable={canNotice}
            style={{ height: "100%" }}
            data-overview-notice
            data-state={noticeState}
            title={
              <Space>
                <BellOutlined />
                <span>校园通知</span>
              </Space>
            }
            extra={
              canNotice ? (
                <Button
                  type={notice && notice.pending > 0 ? "primary" : "default"}
                  size="small"
                  icon={<ArrowRightOutlined />}
                  iconPlacement="end"
                  onClick={() => navigate("/notices")}
                >
                  进入审核
                </Button>
              ) : null
            }
          >
            {noticeState === "ready" && notice ? (
              <Space orientation="vertical" size={8} style={{ width: "100%" }}>
                <Statistic
                  title="近 50 条版本待审核"
                  value={notice.pending}
                  suffix="条"
                  styles={{
                    content: {
                      color: notice.pending > 0 ? "#b45309" : "#15803d",
                    },
                  }}
                />
                <Space size={12} wrap>
                  <Typography.Text type="secondary">已通过 {notice.approved}</Typography.Text>
                  <Typography.Text type="secondary">排队分发 {notice.queued}</Typography.Text>
                  <Typography.Text type="secondary">已送达 {notice.delivered}</Typography.Text>
                </Space>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  更新于 {localDateTime(notice.updatedAt)}
                </Typography.Text>
              </Space>
            ) : (
              <Typography.Text type="secondary">
                {noticeState === "loading"
                  ? "正在读取通知状态…"
                  : noticeState === "not_configured"
                    ? "通知服务暂未接入此环境"
                    : noticeState === "denied"
                      ? "无权查看校园通知"
                      : "暂时无法读取通知列表，可进入工作台核对"}
              </Typography.Text>
            )}
          </Card>
        </Col>
      </Row>

      {/* 业务模块工作台 */}
      <Typography.Title level={5} style={{ marginBottom: 12 }}>
        业务模块入口
      </Typography.Title>
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} md={12}>
          <Card
            size="small"
            style={{ height: "100%" }}
            data-overview-platform
            data-state={platformState}
            title={
              <Space>
                <SafetyCertificateOutlined />
                <span>平台与安全</span>
              </Space>
            }
            extra={
              canPlatform ? (
                <Button size="small" onClick={() => navigate("/operations")}>
                  进入工作台
                </Button>
              ) : (
                <Tag>无权限</Tag>
              )
            }
          >
            {platformState === "ready" && platform ? (
              <Space orientation="vertical" size={4}>
                <Typography.Text>
                  当前页待办收件箱未结项 <strong>{platform.unresolved}</strong> 条
                  {platform.hasMore ? "（还有更多页）" : ""} · 邮件死信{" "}
                  <strong>{platform.deadLetters}</strong> 条
                </Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  账户权限、登录会话、邮件投递与授权审计 · 载入于 {localDateTime(platform.updatedAt)}
                </Typography.Text>
              </Space>
            ) : (
              <Typography.Text type="secondary">
                {platformState === "loading"
                  ? "正在加载平台状态…"
                  : platformState === "denied"
                    ? "当前账户没有平台安全管理权限。"
                    : "管理用户角色授权、会话下线、邮件与审计日志。"}
              </Typography.Text>
            )}
          </Card>
        </Col>

        <Col xs={24} md={12}>
          <Card
            size="small"
            style={{ height: "100%" }}
            title={
              <Space>
                <ExperimentOutlined />
                <span>题库工坊（刷题练习）</span>
              </Space>
            }
            extra={
              canPractice ? (
                <Button size="small" onClick={() => navigate("/practice")}>
                  进入工坊
                </Button>
              ) : (
                <Tag>按权限开放</Tag>
              )
            }
          >
            <Typography.Text type="secondary">
              管理刷题题库、导入草稿版本、逐题校验、发布上架与安全回滚。
            </Typography.Text>
          </Card>
        </Col>

        <Col xs={24} md={12}>
          <Card
            size="small"
            style={{ height: "100%" }}
            data-overview-library
            title={
              <Space>
                <BookOutlined />
                <span>资料库管理</span>
              </Space>
            }
            extra={
              <Space size={8}>
                <Button
                  size="small"
                  href="https://henukit.cn/library"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  公开目录
                </Button>
                {canLibrary && (
                  <Button size="small" onClick={() => navigate("/library")}>
                    进入资料库管理
                  </Button>
                )}
              </Space>
            }
          >
            <Typography.Text type="secondary">
              {libraryState === "loading"
                ? "正在检查资料管理能力…"
                : libraryState === "ready" && library?.management === "not_ready"
                  ? "在线管理接口尚未就绪，目前通过发布流水线更新公开资料目录。"
                  : libraryState === "ready" && library
                    ? "管理课程、资料、审核和纠错，查看公开版本与下载量。"
                    : "查看资料管理能力状态或直接访问学生端公开资料目录。"}
            </Typography.Text>
          </Card>
        </Col>

        {canCareer && <Col xs={24} md={12}>
          <Card size="small" title="求职雷达监控" extra={<Button size="small" onClick={() => navigate("/career")}>查看任务</Button>}>
            <Typography.Text type="secondary">查看扫描、简历解析和邮件简报投递进度。</Typography.Text>
          </Card>
        </Col>}
        {canCampus && <Col xs={24} md={12}>
          <Card size="small" title="校园互助治理" extra={<Button size="small" onClick={() => navigate("/campus")}>查看单子</Button>}>
            <Typography.Text type="secondary">治理公开单子和留言，查看联系方式交换统计。</Typography.Text>
          </Card>
        </Col>}

        <Col xs={24} md={12}>
          <Card
            size="small"
            style={{ height: "100%" }}
            title={
              <Space>
                <CrownOutlined />
                <span>会员权益与积分</span>
              </Space>
            }
            extra={
              <Space size={8}>
                {canMembership && (
                  <Button size="small" onClick={() => navigate("/account/memberships")}>
                    会员权益
                  </Button>
                )}
                {canPoints && (
                  <Button
                    size="small"
                    icon={<WalletOutlined />}
                    onClick={() => navigate("/account/points")}
                  >
                    积分调整
                  </Button>
                )}
              </Space>
            }
          >
            <Typography.Text type="secondary">
              按指定学生账户核对信息后发放/撤销终身会员或手工调整积分。
            </Typography.Text>
          </Card>
        </Col>

        <Col xs={24}>
          <Card
            size="small"
            title={
              <Space>
                <CompassOutlined />
                <span>公共主站（henukit.cn）</span>
              </Space>
            }
            extra={
              <Button
                size="small"
                href="https://henukit.cn"
                target="_blank"
                rel="noopener noreferrer"
              >
                打开主站
              </Button>
            }
          >
            <Typography.Text type="secondary">
              学生端门户由仓库配置驱动发布；可展开下方「系统连接与权限诊断」查看主站实时探活指标。
            </Typography.Text>
          </Card>
        </Col>
      </Row>

      {/* 技术诊断收拢进折叠区，不打扰日常运营 */}
      <Collapse
        items={[
          {
            key: "diagnostics",
            label: "系统连接与权限诊断（排查问题时展开）",
            children: (
              <Space orientation="vertical" size={16} style={{ width: "100%" }}>
                {overviewState === "unavailable" && (
                  <Alert
                    type="info"
                    showIcon
                    title="聚合摘要接口暂时不可用"
                    description="上方各业务待办均为独立直连读取，可正常进入各工作台操作。"
                  />
                )}
                <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }}>
                  {summaries.map((item) => (
                    <Descriptions.Item
                      key={item.id}
                      label={
                        <Space size={6}>
                          <span>{item.name}</span>
                          <Tag color={diagnosticTagColor(item.status)}>
                            {diagnosticLabel(item)}
                          </Tag>
                        </Space>
                      }
                    >
                      <div>{item.statusMessage}</div>
                      {item.asOf && (
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          更新于 {localDateTime(item.asOf)}
                        </Typography.Text>
                      )}
                      {item.requestId && (
                        <div style={{ marginTop: 4 }}>
                          <span className="console-code-chip">{item.requestId}</span>
                        </div>
                      )}
                    </Descriptions.Item>
                  ))}
                </Descriptions>

                {portalSummary && portalSummary.metrics.length > 0 && (
                  <Descriptions
                    title="主站探活指标"
                    bordered
                    size="small"
                    column={{ xs: 1, sm: 3 }}
                  >
                    {portalSummary.metrics.map((metric) => (
                      <Descriptions.Item key={metric.label} label={metric.label}>
                        <strong>{metric.value}</strong>
                        {metric.hint ? (
                          <Typography.Text type="secondary"> · {metric.hint}</Typography.Text>
                        ) : null}
                      </Descriptions.Item>
                    ))}
                  </Descriptions>
                )}

                <div>
                  <Typography.Text type="secondary" style={{ display: "block", marginBottom: 8 }}>
                    当前账户已获授权的权限码（{permissions.length} 项）
                  </Typography.Text>
                  <Space size={[6, 6]} wrap>
                    {permissions.map((perm) => (
                      <span key={perm} className="console-code-chip">
                        {perm}
                      </span>
                    ))}
                  </Space>
                </div>
              </Space>
            ),
          },
        ]}
      />
    </section>
  );
}
