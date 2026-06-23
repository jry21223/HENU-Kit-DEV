import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";

import AnalyticsView from "../views/AnalyticsView.vue";
import CoursesView from "../views/CoursesView.vue";
import AiDraftsView from "../views/AiDraftsView.vue";
import DashboardView from "../views/DashboardView.vue";
import DownloadsView from "../views/DownloadsView.vue";
import LoginView from "../views/LoginView.vue";
import MaterialsView from "../views/MaterialsView.vue";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: DashboardView },
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/dashboard", component: DashboardView },
    { path: "/courses", component: CoursesView },
    { path: "/materials", component: MaterialsView },
    { path: "/downloads", component: DownloadsView },
    { path: "/ai/drafts", component: AiDraftsView, meta: { reviewer: true } },
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
    return auth.canReviewAI ? true : "/login";
  }
  if (!auth.isAdmin) {
    return "/login";
  }
  return true;
});
