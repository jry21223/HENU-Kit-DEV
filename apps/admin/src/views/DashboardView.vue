<template>
  <LegacyDashboardView v-if="config && !config.dashboard_v2_enabled" />
  <AdminShellV2 v-else title="运营总览" :environment="config?.environment ?? 'loading'">
    <div class="admin-v2-page-heading">
      <div>
        <p>OPERATIONS OVERVIEW</p>
        <h1>今天需要处理什么？</h1>
        <span>六个业务域始终保留；未接入、过期和失败都会明确标示。</span>
      </div>
      <button type="button" :disabled="loading" @click="load">{{ loading ? "刷新中…" : "刷新数据" }}</button>
    </div>

    <Alert v-if="error" variant="danger">{{ error }}</Alert>
    <Alert v-else-if="snapshot?.status === 'partial'" variant="warning">部分业务域暂不可用，已加载的卡片仍可继续处理。</Alert>

    <div v-if="loading && !snapshot" class="admin-v2-card-grid" aria-label="正在加载六域指标">
      <Card v-for="index in 6" :key="index" class="metric-domain-card"><Skeleton class="dashboard-skeleton" /></Card>
    </div>
    <div v-else class="admin-v2-card-grid">
      <MetricCard v-for="card in orderedCards" :key="card.domain" :card="card" />
    </div>

    <Card class="admin-v2-action-panel">
      <header><div><p>TODAY</p><h2>今日待办</h2></div><span>{{ actionItems.length }} 项</span></header>
      <div v-if="actionItems.length" class="admin-v2-action-list">
        <RouterLink v-for="item in actionItems" :key="item.id" :to="item.action_path">
          <Badge :variant="item.urgency === 'urgent' ? 'danger' : 'warning'">{{ item.urgency === "urgent" ? "24h" : "72h" }}</Badge>
          <div><strong>{{ item.summary }}</strong><span>{{ item.domain }} · 到期 {{ formatTime(item.due_at) }}</span></div>
          <span>处理 →</span>
        </RouterLink>
      </div>
      <p v-else class="admin-v2-empty">当前没有待办。真实空数据与未接入状态分开显示。</p>
    </Card>
  </AdminShellV2>
</template>

<script setup lang="ts">
import { useQuery } from "@tanstack/vue-query";
import { computed } from "vue";
import AdminShellV2 from "../components/AdminShellV2.vue";
import MetricCard from "../components/admin/MetricCard.vue";
import Alert from "../components/ui/Alert.vue";
import Badge from "../components/ui/Badge.vue";
import Card from "../components/ui/Card.vue";
import Skeleton from "../components/ui/Skeleton.vue";
import { adminRequest, type ActionItem, type DashboardSnapshot, type UIConfig } from "../lib/admin-api";
import LegacyDashboardView from "./LegacyDashboardView.vue";

const order = ["users", "notice", "mail", "feedback", "food", "system"];
const configQuery = useQuery({ queryKey: ["admin", "ui-config"], queryFn: () => adminRequest<UIConfig>("/admin/ui-config") });
const config = computed(() => configQuery.data.value?.data ?? null);
const v2Enabled = computed(() => config.value?.dashboard_v2_enabled === true);
const snapshotQuery = useQuery({
  queryKey: ["admin", "dashboard-snapshot"],
  queryFn: () => adminRequest<DashboardSnapshot>("/admin/dashboard-snapshots/latest"),
  enabled: v2Enabled,
});
const actionsQuery = useQuery({
  queryKey: ["admin", "action-items"],
  queryFn: () => adminRequest<{ items: ActionItem[] }>("/admin/action-items"),
  enabled: v2Enabled,
});
const snapshot = computed(() => snapshotQuery.data.value?.data ?? null);
const actionItems = computed(() => actionsQuery.data.value?.data.items ?? []);
const loading = computed(() => configQuery.isFetching.value || snapshotQuery.isFetching.value || actionsQuery.isFetching.value);
const error = computed(() => {
  const caught = configQuery.error.value ?? snapshotQuery.error.value ?? actionsQuery.error.value;
  return caught instanceof Error ? caught.message : caught ? "管理后台数据加载失败" : "";
});
const orderedCards = computed(() => [...(snapshot.value?.cards ?? [])].sort((a, b) => order.indexOf(a.domain) - order.indexOf(b.domain)));

async function load() {
  await configQuery.refetch();
  if (!v2Enabled.value) return;
  await Promise.all([snapshotQuery.refetch(), actionsQuery.refetch()]);
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(value));
}
</script>
