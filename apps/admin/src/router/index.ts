import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";

import AnalyticsView from "../views/AnalyticsView.vue";
import BlogReviewsView from "../views/BlogReviewsView.vue";
import CoursesView from "../views/CoursesView.vue";
import AiDraftsView from "../views/AiDraftsView.vue";
import DashboardView from "../views/DashboardView.vue";
import DownloadsView from "../views/DownloadsView.vue";
import ForumReplyReviewsView from "../views/ForumReplyReviewsView.vue";
import ForumReviewsView from "../views/ForumReviewsView.vue";
import LoginView from "../views/LoginView.vue";
import MaterialReviewsView from "../views/MaterialReviewsView.vue";
import MaterialsView from "../views/MaterialsView.vue";
import OperationLogsView from "../views/OperationLogsView.vue";
import ReportsView from "../views/ReportsView.vue";
import UsersView from "../views/UsersView.vue";
import WikiProposalReviewsView from "../views/WikiProposalReviewsView.vue";
import WikiReviewsView from "../views/WikiReviewsView.vue";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: DashboardView },
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/dashboard", component: DashboardView },
    { path: "/users", component: UsersView },
    { path: "/courses", component: CoursesView },
    { path: "/materials", component: MaterialsView },
    { path: "/downloads", component: DownloadsView },
    { path: "/operation-logs", component: OperationLogsView },
    { path: "/material-reviews", component: MaterialReviewsView, meta: { reviewer: true } },
    { path: "/wiki-reviews", component: WikiReviewsView, meta: { reviewer: true } },
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
