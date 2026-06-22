<template>
  <main class="admin-shell">
    <aside class="sidebar">
      <div class="brand">
        <strong>资料运营工作台</strong>
        <span class="muted">软件学院课程资料库</span>
      </div>
      <nav class="side-nav" aria-label="后台导航">
        <RouterLink to="/dashboard">运营概览</RouterLink>
        <RouterLink to="/courses">课程维护</RouterLink>
        <RouterLink to="/materials">PDF 资料</RouterLink>
        <RouterLink to="/downloads">下载审计</RouterLink>
        <span class="nav-section">预留能力</span>
        <span class="disabled-nav" aria-disabled="true">内容审核</span>
        <span class="disabled-nav" aria-disabled="true">课程社区</span>
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
  logout: "退出",
};

const auth = useAuthStore();
const router = useRouter();

async function handleLogout() {
  await auth.logout();
  await router.push("/login");
}
</script>
