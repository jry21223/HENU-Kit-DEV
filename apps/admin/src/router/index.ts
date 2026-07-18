import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { adminRequest, type UIConfig } from "../lib/admin-api";

const AccessGrantsView = () => import("../views/AccessGrantsView.vue");
const AnalyticsView = () => import("../views/AnalyticsView.vue");
const BlogReviewsView = () => import("../views/BlogReviewsView.vue");
const CoursesView = () => import("../views/CoursesView.vue");
const AiDraftsView = () => import("../views/AiDraftsView.vue");
const DashboardView = () => import("../views/DashboardView.vue");
const LegacyDashboardView = () => import("../views/LegacyDashboardView.vue");
const DownloadsView = () => import("../views/DownloadsView.vue");
const DomainOperationsView = () => import("../views/DomainOperationsView.vue");
const UnifiedDomainView = () => import("../views/UnifiedDomainView.vue");
const ForumReplyReviewsView = () => import("../views/ForumReplyReviewsView.vue");
const ForumReviewsView = () => import("../views/ForumReviewsView.vue");
const LoginView = () => import("../views/LoginView.vue");
const MaterialReviewsView = () => import("../views/MaterialReviewsView.vue");
const MaterialsView = () => import("../views/MaterialsView.vue");
const MediaAssetsView = () => import("../views/MediaAssetsView.vue");
const OperationLogsView = () => import("../views/OperationLogsView.vue");
const OrdersView = () => import("../views/OrdersView.vue");
const PackagesView = () => import("../views/PackagesView.vue");
const PaymentIncidentsView = () => import("../views/PaymentIncidentsView.vue");
const PaymentReconciliationView = () => import("../views/PaymentReconciliationView.vue");
const MembershipsView = () => import("../views/MembershipsView.vue");
const PointsView = () => import("../views/PointsView.vue");
const ReportsView = () => import("../views/ReportsView.vue");
const UsersView = () => import("../views/UsersView.vue");
const WikiCreatorApplicationsView = () => import("../views/WikiCreatorApplicationsView.vue");
const WikiProposalReviewsView = () => import("../views/WikiProposalReviewsView.vue");
const WikiReviewsView = () => import("../views/WikiReviewsView.vue");

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: DashboardView },
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/dashboard", component: DashboardView },
    { path: "/legacy-dashboard", component: LegacyDashboardView },
    { path: "/notices", component: UnifiedDomainView, meta: { title: "校园通知", domain: "notice", description: "人工导入、不可变版本、审核与订阅分发。" } },
    { path: "/mail", component: UnifiedDomainView, meta: { title: "邮件投递", domain: "mail", description: "Critical、Transactional、Digest 队列与投递状态。" } },
    { path: "/feedback", component: UnifiedDomainView, meta: { title: "反馈中心", domain: "feedback", description: "平台反馈、题目反馈与两档 SLA 待办。" } },
    { path: "/food", component: UnifiedDomainView, meta: { title: "美食榜单", domain: "food", description: "五档投稿、社区校准、调档候选与异常票。" } },
    { path: "/system", component: UnifiedDomainView, meta: { title: "系统运行", domain: "system", description: "服务健康、Worker、Outbox、部署与数据新鲜度。" } },
    { path: "/quiz-operations", component: DomainOperationsView, meta: { title: "刷题运营", domain: "quiz", description: "QuizCraft 是唯一刷题数据 Owner。" } },
    { path: "/users", component: UsersView },
    { path: "/points", component: PointsView },
    { path: "/memberships", component: MembershipsView },
    { path: "/access-grants", component: AccessGrantsView },
    { path: "/orders", component: OrdersView },
    { path: "/payment-reconciliation", component: PaymentReconciliationView },
    { path: "/payment-incidents", component: PaymentIncidentsView },
    { path: "/packages", component: PackagesView },
    { path: "/courses", component: CoursesView },
    { path: "/materials", component: MaterialsView },
    { path: "/media-assets", component: MediaAssetsView },
    { path: "/downloads", component: DownloadsView },
    { path: "/operation-logs", component: OperationLogsView },
    { path: "/material-reviews", component: MaterialReviewsView, meta: { reviewer: true } },
    { path: "/wiki-reviews", component: WikiReviewsView, meta: { reviewer: true } },
    { path: "/wiki-creator-applications", component: WikiCreatorApplicationsView, meta: { reviewer: true } },
    { path: "/wiki-proposal-reviews", component: WikiProposalReviewsView, meta: { reviewer: true } },
    { path: "/blog-reviews", component: BlogReviewsView, meta: { reviewer: true } },
    { path: "/forum-reviews", component: ForumReviewsView, meta: { reviewer: true } },
    { path: "/forum-reply-reviews", component: ForumReplyReviewsView, meta: { reviewer: true } },
    { path: "/ai/drafts", component: AiDraftsView, meta: { reviewer: true } },
    { path: "/reports", component: ReportsView, meta: { reviewer: true } },
    { path: "/analytics", component: AnalyticsView },
  ],
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();
  if (to.meta.public) {
    if (!auth.ready) {
      await auth.loadMe();
    }
    if (auth.authenticated && auth.canAccessAdminConsole) {
      return auth.isAdmin ? "/dashboard" : "/ai/drafts";
    }
    return true;
  }
  if (!auth.ready) {
    await auth.loadMe();
  }
  if (!auth.authenticated || !auth.canAccessAdminConsole) {
    return "/login";
  }
  if (to.path === "/") {
    return auth.isAdmin ? "/dashboard" : "/ai/drafts";
  }
  if (to.meta.reviewer) {
    return auth.canReviewContent ? true : "/login";
  }
  if (!auth.isAdmin) {
    return "/login";
  }
	if (["/dashboard", "/notices", "/mail", "/feedback", "/food", "/system", "/quiz-operations"].includes(to.path)) {
		try {
			const config = (await adminRequest<UIConfig>("/admin/ui-config")).data;
			if (config.shell_version === "legacy") return "/legacy-dashboard";
		} catch {
			// The destination page owns the full-page BFF error state. Authentication
			// has already succeeded, so a transient config failure must not log out.
		}
	}
  return true;
});
