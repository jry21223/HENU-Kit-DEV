import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";

import CoursesView from "../views/CoursesView.vue";
import AiDraftsView from "../views/AiDraftsView.vue";
import DashboardView from "../views/DashboardView.vue";
import DownloadsView from "../views/DownloadsView.vue";
import LoginView from "../views/LoginView.vue";
import MaterialsView from "../views/MaterialsView.vue";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/dashboard" },
    { path: "/login", component: LoginView, meta: { public: true } },
    { path: "/dashboard", component: DashboardView },
    { path: "/courses", component: CoursesView },
    { path: "/materials", component: MaterialsView },
    { path: "/downloads", component: DownloadsView },
    { path: "/ai/drafts", component: AiDraftsView },
  ],
});

router.beforeEach(async (to) => {
  if (to.meta.public) {
    return true;
  }
  const auth = useAuthStore();
  if (!auth.ready) {
    await auth.loadMe();
  }
  if (!auth.authenticated || !auth.isAdmin) {
    return "/login";
  }
  return true;
});
