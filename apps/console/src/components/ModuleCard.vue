<script setup lang="ts">
import { CircleAlert, Clock3, LockKeyhole } from "@lucide/vue";
import type { Component } from "vue";
import { computed } from "vue";

import StatusBadge from "@/components/ui/StatusBadge.vue";
import { type ModuleStatus, type ModuleSummary } from "@/data/modules";
import { localDateTime } from "@/lib/format";

const props = defineProps<{
  summary: ModuleSummary;
  icon: Component;
}>();

const statusLabels: Record<ModuleStatus, string> = {
  ok: "正常",
  loading: "加载中",
  empty: "暂无数据",
  partial: "部分可用",
  stale: "数据已过期",
  unavailable: "暂不可用",
  denied: "无权访问",
};

const statusLabel = computed(() => props.summary.unavailableReason === "not_onboarded"
  ? "尚未接入"
  : props.summary.unavailableReason === "operator_disabled"
    ? "已停用"
    : statusLabels[props.summary.status]);
</script>

<template>
  <article
    class="min-w-0 rounded-lg border border-border bg-card p-4"
    :class="summary.status === 'denied' && 'border-dashed bg-muted/40'"
    :data-module-card="summary.id"
    :data-state="summary.status"
    :aria-label="`${summary.name}：${statusLabel}`"
  >
    <template v-if="summary.status === 'loading'">
      <div class="flex items-center gap-3">
        <div class="skeleton size-9 rounded-md" />
        <div class="flex-1 space-y-2"><div class="skeleton h-2.5 w-16" /><div class="skeleton h-4 w-28" /></div>
      </div>
      <div class="mt-4 grid grid-cols-2 gap-2"><div class="skeleton h-16 rounded-md" /><div class="skeleton h-16 rounded-md" /></div>
      <span class="sr-only">{{ summary.name }} 正在加载</span>
    </template>

    <template v-else>
      <header class="flex items-start gap-3">
        <div class="grid size-9 flex-none place-items-center rounded-md border border-border bg-muted text-muted-foreground">
          <component :is="icon" :size="17" aria-hidden="true" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs text-muted-foreground">{{ summary.eyebrow }}</p>
          <h2 class="mt-0.5 truncate text-sm font-semibold tracking-tight">{{ summary.name }}</h2>
        </div>
        <StatusBadge :status="summary.status">{{ statusLabel }}</StatusBadge>
      </header>

      <p class="mt-3 text-sm leading-6 text-muted-foreground">{{ summary.description }}</p>

      <div v-if="summary.metrics.length" class="mt-4 grid grid-cols-2 gap-2">
        <div
          v-for="metric in summary.metrics"
          :key="metric.label"
          data-metric-tile
          class="grid gap-0.5 rounded-md border border-border p-2.5"
          :aria-label="`${metric.label}：${metric.value}`"
        >
          <span class="text-xs text-muted-foreground">{{ metric.label }}</span>
          <strong class="text-base font-semibold tracking-tight tabular-nums">{{ metric.value }}</strong>
          <small v-if="metric.hint" class="text-xs text-muted-foreground">{{ metric.hint }}</small>
        </div>
      </div>

      <!-- Degraded modules carry their explanation here. It used to be repeated
           verbatim in the footer below, which read as a rendering bug on every
           unavailable / denied / partial card. -->
      <div
        v-else
        class="mt-4 flex items-center gap-2.5 rounded-md border p-3 text-sm"
        :class="
          summary.status === 'unavailable'
            ? 'border-destructive/25 bg-destructive/5 text-destructive'
            : 'border-border bg-muted/50 text-muted-foreground'
        "
      >
        <LockKeyhole v-if="summary.status === 'denied'" :size="16" aria-hidden="true" class="shrink-0" />
        <CircleAlert v-else-if="summary.status === 'unavailable'" :size="16" aria-hidden="true" class="shrink-0" />
        <Clock3 v-else :size="16" aria-hidden="true" class="shrink-0" />
        <span>{{ summary.statusMessage }}</span>
      </div>

      <footer
        v-if="summary.metrics.length || summary.asOf || (summary.status === 'stale' && summary.lastSuccessAt)"
        class="mt-4 flex flex-wrap items-end justify-between gap-x-3 gap-y-1 border-t border-border pt-3 text-xs text-muted-foreground"
      >
        <span v-if="summary.metrics.length">{{ summary.statusMessage }}</span>
        <span class="ml-auto flex flex-col items-end gap-0.5">
          <span v-if="summary.asOf">截至 {{ localDateTime(summary.asOf) }}</span>
          <span v-if="summary.status === 'stale' && summary.lastSuccessAt">最近成功 {{ localDateTime(summary.lastSuccessAt) }}</span>
        </span>
      </footer>
    </template>
  </article>
</template>

<style scoped>
.skeleton {
  background: linear-gradient(
    90deg,
    color-mix(in oklab, var(--muted) 100%, transparent) 20%,
    color-mix(in oklab, var(--background) 100%, transparent) 50%,
    color-mix(in oklab, var(--muted) 100%, transparent) 80%
  );
  background-size: 220% 100%;
  animation: shimmer 1.3s infinite linear;
}

@keyframes shimmer {
  to {
    background-position: -220% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .skeleton {
    animation: none;
  }
}
</style>
