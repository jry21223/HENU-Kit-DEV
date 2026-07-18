<template>
  <Card class="metric-domain-card">
    <header class="metric-domain-card__header">
      <div>
        <p class="metric-domain-card__eyebrow">{{ card.primary_metric.label }}</p>
        <h2>{{ card.title }}</h2>
      </div>
      <Badge :variant="badgeVariant">{{ statusLabel }}</Badge>
    </header>
    <strong class="metric-domain-card__primary">{{ formatMetric(card.primary_metric) }}</strong>
    <div class="metric-domain-card__metrics">
      <div v-for="item in card.metrics" :key="item.code">
        <span>{{ item.label }}</span>
        <strong>{{ formatMetric(item) }}</strong>
      </div>
    </div>
    <p class="metric-domain-card__message">{{ card.message }}</p>
    <footer>
      <span>数据时间：{{ formatTime(card.as_of) }}</span>
      <RouterLink v-if="card.action_path && card.status !== 'not_integrated'" :to="card.action_path">进入处理</RouterLink>
      <span v-else>尚无处理入口</span>
    </footer>
  </Card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AdminMetric, DashboardCard } from "../../lib/admin-api";
import Badge from "../ui/Badge.vue";
import Card from "../ui/Card.vue";

const props = defineProps<{ card: DashboardCard }>();
const labels = { not_integrated: "未接入", ok: "正常", partial: "部分可用", stale: "数据过期", unavailable: "不可用" } as const;
const statusLabel = computed(() => labels[props.card.status]);
const badgeVariant = computed(() => ({
  not_integrated: "muted",
  ok: "success",
  partial: "warning",
  stale: "warning",
  unavailable: "danger",
} as const)[props.card.status]);

function formatMetric(item: AdminMetric) {
  if (item.value === null) return "—";
  if (item.code.includes("rate")) return `${(item.value * 100).toFixed(1)}%`;
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(item.value);
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(value));
}
</script>
