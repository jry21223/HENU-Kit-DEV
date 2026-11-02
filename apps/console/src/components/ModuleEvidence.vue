<script setup lang="ts">
import { ArrowRight, X } from "@lucide/vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { ModuleMetric, ModuleStatus, ModuleSummary } from "@/data/modules";
import { localDateTime } from "@/lib/format";

defineProps<{
  summary: ModuleSummary;
  metrics: ModuleMetric[];
  statusLabel: string;
  badgeStatus: ModuleStatus;
  destination?: { href: string; label: string };
  closable?: boolean;
}>();
defineEmits<{ close: [] }>();
</script>

<template>
  <section class="module-evidence" :data-module-evidence="summary.id" :aria-label="`${summary.name} 来源证据`">
    <header class="evidence-heading">
      <div><p class="overview-eyebrow">来源证据</p><h2>{{ summary.name }}</h2></div>
      <button v-if="closable" type="button" class="evidence-close" :aria-label="`收起 ${summary.name} 证据`" @click="$emit('close')"><X :size="18" /></button>
      <StatusBadge v-else :status="badgeStatus">{{ statusLabel }}</StatusBadge>
    </header>
    <p class="evidence-description">{{ summary.description }}</p>
    <p v-if="summary.status === 'partial' || summary.status === 'stale'" class="evidence-status-message">{{ summary.statusMessage }}</p>
    <dl v-if="metrics.length" class="evidence-facts">
      <div v-for="metric in metrics" :key="metric.label"><dt>{{ metric.label }}</dt><dd>{{ metric.value }}<small v-if="metric.hint">{{ metric.hint }}</small></dd></div>
    </dl>
    <dl class="evidence-provenance">
      <div><dt>观测时间</dt><dd>{{ summary.asOf ? localDateTime(summary.asOf) : '未提供' }}</dd></div>
      <div v-if="summary.lastSuccessAt"><dt>最近成功</dt><dd>{{ localDateTime(summary.lastSuccessAt) }}</dd></div>
      <div v-if="summary.requestId"><dt>请求标识</dt><dd class="evidence-request">{{ summary.requestId }}</dd></div>
    </dl>
    <a v-if="destination" :href="destination.href" class="overview-primary">{{ destination.label }}<ArrowRight :size="16" aria-hidden="true" /></a>
    <p v-else class="evidence-readonly">{{ summary.id === 'portal' ? '公共站点信息只读呈现' : '此模块没有可用的管理入口' }}</p>
  </section>
</template>
