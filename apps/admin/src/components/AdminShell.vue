<template>
  <main class="admin-shell">
    <aside class="sidebar">
      <strong>Final Review Admin</strong>
      <span class="muted">{{ copy.subtitle }}</span>
      <nav class="side-nav">
        <RouterLink to="/dashboard">{{ copy.dashboard }}</RouterLink>
        <RouterLink to="/courses">{{ copy.courses }}</RouterLink>
        <RouterLink to="/materials">{{ copy.materials }}</RouterLink>
        <RouterLink to="/downloads">{{ copy.downloads }}</RouterLink>
      </nav>
      <div class="sidebar-footer">
        <span>{{ auth.user?.email }}</span>
        <el-button size="small" @click="handleLogout">{{ copy.logout }}</el-button>
      </div>
    </aside>
    <section class="content">
      <slot />
    </section>
  </main>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const copy = {
  subtitle: "\u0056\u0032 \u7ba1\u7406\u540e\u53f0",
  dashboard: "\u4eea\u8868\u76d8",
  courses: "\u8bfe\u7a0b\u7ba1\u7406",
  materials: "\u8d44\u6599\u7ba1\u7406",
  downloads: "\u4e0b\u8f7d\u5ba1\u8ba1",
  logout: "\u9000\u51fa",
};

const auth = useAuthStore();
const router = useRouter();

async function handleLogout() {
  await auth.logout();
  await router.push("/login");
}
</script>
