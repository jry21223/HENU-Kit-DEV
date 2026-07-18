<script setup lang="ts">
import { ChevronDown, CircleAlert, Clock3, LockKeyhole } from "@lucide/vue";
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from "reka-ui";
import type { Component } from "vue";

import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { ModuleStatus, ModuleSummary } from "@/data/modules";

defineProps<{
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
</script>

<template>
  <article
    class="module-card scroll-mt-24"
    :class="`module-card--${summary.status}`"
    :data-module-card="summary.id"
    :data-state="summary.status"
    :aria-label="`${summary.name}：${statusLabels[summary.status]}`"
  >
    <template v-if="summary.status === 'loading'">
      <div class="flex items-center gap-3">
        <div class="skeleton size-11 rounded-xl" />
        <div class="flex-1 space-y-2"><div class="skeleton h-3 w-20" /><div class="skeleton h-5 w-32" /></div>
      </div>
      <div class="mt-5 grid grid-cols-2 gap-3"><div class="skeleton h-20 rounded-xl" /><div class="skeleton h-20 rounded-xl" /></div>
      <span class="sr-only">{{ summary.name }} 正在加载</span>
    </template>

    <template v-else>
      <header class="flex items-start gap-3">
        <div class="module-icon"><component :is="icon" :size="20" aria-hidden="true" /></div>
        <div class="min-w-0 flex-1">
          <p class="text-[0.68rem] font-bold uppercase tracking-[0.12em] text-[var(--hk-ink-muted)]">{{ summary.eyebrow }}</p>
          <h2 class="mt-1 text-lg font-bold tracking-[-0.02em]">{{ summary.name }}</h2>
        </div>
        <StatusBadge :status="summary.status">{{ statusLabels[summary.status] }}</StatusBadge>
      </header>

      <p class="mt-3 min-h-12 text-sm leading-6 text-[var(--hk-ink-muted)]">{{ summary.description }}</p>

      <div v-if="summary.metrics.length" class="mt-4 grid grid-cols-2 gap-3">
        <div v-for="metric in summary.metrics" :key="metric.label" class="metric-tile">
          <span>{{ metric.label }}</span><strong>{{ metric.value }}</strong><small v-if="metric.hint">{{ metric.hint }}</small>
        </div>
      </div>

      <div v-else class="state-panel" :class="`state-panel--${summary.status}`">
        <LockKeyhole v-if="summary.status === 'denied'" :size="18" aria-hidden="true" />
        <CircleAlert v-else-if="summary.status === 'unavailable'" :size="18" aria-hidden="true" />
        <Clock3 v-else :size="18" aria-hidden="true" />
        <span>{{ summary.statusMessage }}</span>
      </div>

      <div v-if="summary.trend" class="mt-4 rounded-xl border border-[var(--hk-paper-line)] bg-[var(--hk-paper)]/45 p-3">
        <div class="flex items-center justify-between gap-3">
          <span class="text-xs font-semibold">关键页面探针 · 5 日</span>
          <span class="text-[0.68rem] text-[var(--hk-ink-muted)]">成功次数</span>
        </div>
        <div class="mt-3 flex h-16 items-end gap-2" role="img" aria-label="Portal 最近五日关键页面探针成功次数柱状图">
          <div v-for="point in summary.trend" :key="point.label" class="flex flex-1 flex-col items-center gap-1">
            <div class="w-full rounded-sm bg-[var(--hk-ink-green)]" :style="{ height: `${Math.max(12, point.value * 1.55)}px` }" />
            <span class="text-[0.62rem] text-[var(--hk-ink-muted)]">{{ point.label.slice(1) }}</span>
          </div>
        </div>
        <CollapsibleRoot class="mt-2">
          <CollapsibleTrigger class="flex min-h-9 w-full items-center justify-between rounded-lg px-2 text-xs font-semibold text-[var(--hk-ink-green-deep)] hover:bg-[var(--hk-ink-green-soft)]">
            查看表格数据 <ChevronDown :size="15" aria-hidden="true" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-2">
            <table class="w-full text-left text-xs" aria-label="Portal 探针成功次数表格">
              <thead><tr><th class="py-1">日期</th><th class="py-1 text-right">成功次数</th></tr></thead>
              <tbody><tr v-for="point in summary.trend" :key="point.label"><td class="py-1">{{ point.label }}</td><td class="py-1 text-right tabular-nums">{{ point.value }}</td></tr></tbody>
            </table>
          </CollapsibleContent>
        </CollapsibleRoot>
      </div>

      <footer class="mt-4 flex min-h-8 items-end justify-between gap-3 border-t border-[var(--hk-paper-line)] pt-3 text-[0.68rem] text-[var(--hk-ink-muted)]">
        <span v-if="summary.status !== 'empty'">{{ summary.statusMessage }}</span>
        <span v-else>正常空状态</span>
        <span v-if="summary.status === 'stale'">最近成功 {{ summary.lastSuccessAt }}</span>
        <span v-else-if="summary.requestId" class="font-mono">{{ summary.requestId }}</span>
        <span v-else-if="summary.asOf">截至 {{ summary.asOf }}</span>
      </footer>
    </template>
  </article>
</template>

<style scoped>
.module-card {
  min-width: 0;
  border: 1px solid var(--hk-paper-line);
  border-radius: var(--hk-radius-feature);
  background: white;
  padding: 1.15rem;
  box-shadow: var(--hk-shadow-card);
}

.module-card--partial { border-top: 3px solid var(--hk-warning); }
.module-card--stale { border-top: 3px solid #d97706; }
.module-card--unavailable { border-top: 3px solid var(--hk-danger); }
.module-card--denied { border-style: dashed; background: #f5f5f4; }

.module-icon {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  flex: none;
  place-items: center;
  border-radius: 0.8rem;
  background: var(--hk-ink-green-soft);
  color: var(--hk-ink-green-deep);
}

.metric-tile {
  display: grid;
  gap: 0.15rem;
  border-radius: var(--hk-radius-card);
  background: var(--hk-paper);
  padding: 0.75rem;
}

.metric-tile span, .metric-tile small { color: var(--hk-ink-muted); font-size: 0.68rem; }
.metric-tile strong { font-size: 1.25rem; letter-spacing: -0.03em; }

.state-panel {
  display: flex;
  min-height: 5rem;
  align-items: center;
  gap: 0.65rem;
  margin-top: 1rem;
  border-radius: var(--hk-radius-card);
  background: var(--hk-paper);
  padding: 0.85rem;
  color: var(--hk-ink-muted);
  font-size: 0.78rem;
  line-height: 1.45;
}

.state-panel--unavailable { background: #fff1f0; color: var(--hk-danger); }
.state-panel--denied { background: #e7e5e4; color: #57534e; }

.skeleton {
  background: linear-gradient(90deg, #ecebe7 20%, #f7f6f2 50%, #ecebe7 80%);
  background-size: 220% 100%;
  animation: shimmer 1.3s infinite linear;
}

@keyframes shimmer { to { background-position: -220% 0; } }

@media (prefers-reduced-motion: reduce) { .skeleton { animation: none; } }
</style>
