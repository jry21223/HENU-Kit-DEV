import { defineStore } from "pinia";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    role: "admin",
    authenticated: false,
  }),
  actions: {
    markMockLoggedIn() {
      this.authenticated = true;
    },
  },
});
