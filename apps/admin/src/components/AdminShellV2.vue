<template>
  <div class="admin-v2-shell">
    <button class="admin-v2-mobile-trigger" type="button" :aria-expanded="mobileOpen" @click="mobileOpen = !mobileOpen">菜单</button>
    <aside :class="['admin-v2-sidebar', { 'is-open': mobileOpen }]">
      <div class="admin-v2-brand">
        <strong>HENU Kit 工作台</strong>
        <span>学生自主运营 · 非河南大学官方项目</span>
      </div>
      <nav aria-label="统一管理后台导航">
        <RouterLink to="/dashboard">总览</RouterLink>
        <section v-for="group in groups" :key="group.label">
          <p>{{ group.label }}</p>
          <RouterLink v-for="item in group.items" :key="item.to" :to="item.to">{{ item.label }}</RouterLink>
        </section>
      </nav>
      <div class="admin-v2-sidebar__footer">
        <span>{{ auth.user?.email }}</span>
        <button type="button" @click="logout">退出</button>
      </div>
    </aside>
    <div class="admin-v2-main">
      <header class="admin-v2-header">
        <div><span>统一管理后台</span><strong>{{ title }}</strong></div>
        <div class="admin-v2-header__meta"><span>{{ environment }}</span><span>学生自主运营</span></div>
      </header>
      <main class="admin-v2-content"><slot /></main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

withDefaults(defineProps<{ title?: string; environment?: string }>(), { title: "运营总览", environment: "unknown" });
const mobileOpen = ref(false);
const auth = useAuthStore();
const router = useRouter();
const groups = [
  { label: "核心运营", items: [{ label: "用户与受众", to: "/users" }, { label: "校园通知", to: "/notices" }, { label: "邮件投递", to: "/mail" }, { label: "反馈中心", to: "/feedback" }, { label: "美食榜单", to: "/food" }] },
  { label: "学习业务", items: [{ label: "资料库运营", to: "/materials" }, { label: "刷题运营", to: "/quiz-operations" }] },
  { label: "平台治理", items: [{ label: "系统运行", to: "/system" }, { label: "审计与安全", to: "/operation-logs" }] },
  { label: "旧版运营", items: [{ label: "旧版总览", to: "/legacy-dashboard" }, { label: "支付异常", to: "/payment-incidents" }, { label: "会员与积分", to: "/memberships" }] },
];

async function logout() {
  await auth.logout();
  await router.push("/login");
}
</script>
