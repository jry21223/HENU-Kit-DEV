<template>
  <main class="admin-shell">
    <aside class="sidebar">
      <strong>Final Review Admin</strong>
      <span class="muted">V2 管理后台</span>
      <nav class="side-nav">
        <RouterLink to="/dashboard">仪表盘</RouterLink>
        <RouterLink to="/courses">课程管理</RouterLink>
        <RouterLink to="/materials">资料管理</RouterLink>
      </nav>
      <div class="sidebar-footer">
        <span>{{ auth.user?.email }}</span>
        <el-button size="small" @click="handleLogout">退出</el-button>
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

const auth = useAuthStore();
const router = useRouter();

async function handleLogout() {
  await auth.logout();
  await router.push("/login");
}
</script>
