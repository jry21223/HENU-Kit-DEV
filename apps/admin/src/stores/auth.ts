import { defineStore } from "pinia";
import { apiRequest, getStoredToken, logout, type User } from "../lib/api";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    user: null as User | null,
    ready: false,
  }),
  getters: {
    authenticated: (state) => Boolean(state.user),
    isAdmin: (state) => state.user?.role === "admin" || state.user?.role === "super_admin",
    canReviewAI: (state) =>
      state.user?.role === "reviewer" || state.user?.role === "admin" || state.user?.role === "super_admin",
    canAccessAdminConsole: (state) =>
      state.user?.role === "reviewer" || state.user?.role === "admin" || state.user?.role === "super_admin",
  },
  actions: {
    async loadMe() {
      if (!getStoredToken()) {
        this.user = null;
        this.ready = true;
        return;
      }
      try {
        const response = await apiRequest<User>("/auth/me");
        this.user = response.data ?? null;
      } catch {
        this.user = null;
      } finally {
        this.ready = true;
      }
    },
    setUser(user: User | null) {
      this.user = user;
      this.ready = true;
    },
    async logout() {
      await logout();
      this.user = null;
      this.ready = true;
    },
  },
});
