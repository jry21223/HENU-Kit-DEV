import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";

import AccessGrantsView from "../views/AccessGrantsView.vue";
import AnalyticsView from "../views/AnalyticsView.vue";
import BlogReviewsView from "../views/BlogReviewsView.vue";
import CoursesView from "../views/CoursesView.vue";
import AiDraftsView from "../views/AiDraftsView.vue";
import DashboardView from "../views/DashboardView.vue";
import LegacyDashboardView from "../views/LegacyDashboardView.vue";
import DownloadsView from "../views/DownloadsView.vue";
import DomainOperationsView from "../views/DomainOperationsView.vue";
import ForumReplyReviewsView from "../views/ForumReplyReviewsView.vue";
import ForumReviewsView from "../views/ForumReviewsView.vue";
import LoginView from "../views/LoginView.vue";
import MaterialReviewsView from "../views/MaterialReviewsView.vue";
import MaterialsView from "../views/MaterialsView.vue";
import MediaAssetsView from "../views/MediaAssetsView.vue";
import OperationLogsView from "../views/OperationLogsView.vue";
import OrdersView from "../views/OrdersView.vue";
import PackagesView from "../views/PackagesView.vue";
import PaymentIncidentsView from "../views/PaymentIncidentsView.vue";
import PaymentReconciliationView from "../views/PaymentReconciliationView.vue";
import MembershipsView from "../views/MembershipsView.vue";
import PointsView from "../views/PointsView.vue";
import ReportsView from "../views/ReportsView.vue";
import UsersView from "../views/UsersView.vue";
import WikiCreatorApplicationsView from "../views/WikiCreatorApplicationsView.vue";
import WikiProposalReviewsView from "../views/WikiProposalReviewsView.vue";
import WikiReviewsView from "../views/WikiReviewsView.vue";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: DashboardView },
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/dashboard", component: DashboardView },
    { path: "/legacy-dashboard", component: LegacyDashboardView },
    { path: "/notices", component: DomainOperationsView, meta: { title: "校园通知", domain: "notice", description: "人工导入、不可变版本、审核与订阅分发。" } },
    { path: "/mail", component: DomainOperationsView, meta: { title: "邮件投递", domain: "mail", description: "Critical、Transactional、Digest 队列与投递状态。" } },
    { path: "/feedback", component: DomainOperationsView, meta: { title: "反馈中心", domain: "feedback", description: "平台反馈、题目反馈与两档 SLA 待办。" } },
    { path: "/food", component: DomainOperationsView, meta: { title: "美食榜单", domain: "food", description: "五档投稿、社区校准、调档候选与异常票。" } },
    { path: "/system", component: DomainOperationsView, meta: { title: "系统运行", domain: "system", description: "服务健康、Worker、Outbox、部署与数据新鲜度。" } },
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
  return true;
});
