<template>
  <Card :data-domain-card="card.domain" class="group overflow-hidden transition-all hover:-translate-y-0.5 hover:shadow-md">
    <CardHeader class="flex-row items-start justify-between space-y-0 pb-3">
      <div class="flex items-center gap-3">
        <div class="flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <component :is="domainIcon" class="size-5" />
        </div>
        <div>
          <CardTitle class="text-base">{{ card.title }}</CardTitle>
          <CardDescription class="mt-1 text-xs">{{ card.primary_metric.label }}</CardDescription>
        </div>
      </div>
      <Badge :variant="badgeVariant">{{ statusLabel }}</Badge>
    </CardHeader>

    <CardContent class="pb-4">
      <div class="flex items-end justify-between gap-3">
        <strong class="text-3xl font-semibold tracking-tight text-slate-950">{{ formatMetric(card.primary_metric) }}</strong>
        <span v-if="card.primary_metric.change_rate !== null" :class="changeClass(card.primary_metric.change_rate)" class="flex items-center gap-1 text-xs font-medium">
          <TrendingUp class="size-3.5" />{{ formatChange(card.primary_metric.change_rate) }}
        </span>
      </div>

      <div class="mt-5 grid grid-cols-3 gap-2">
        <div v-for="item in card.metrics" :key="item.code" class="rounded-lg border bg-muted/40 px-3 py-2.5">
          <span class="block min-h-8 text-[11px] leading-4 text-muted-foreground">{{ item.label }}</span>
          <strong class="mt-1 block text-sm font-semibold text-slate-900">{{ formatMetric(item) }}</strong>
        </div>
      </div>

      <p class="mt-4 min-h-10 text-xs leading-5 text-muted-foreground">{{ card.message }}</p>
    </CardContent>

    <CardFooter class="justify-between border-t bg-muted/20 px-6 py-3 text-[11px] text-muted-foreground">
      <span class="flex items-center gap-1.5"><Clock3 class="size-3.5" />{{ formatTime(card.as_of) }}</span>
      <RouterLink v-if="card.action_path && card.status !== 'not_integrated'" class="flex items-center gap-1 font-medium text-primary hover:underline" :to="card.action_path">
        进入处理 <ArrowUpRight class="size-3.5" />
      </RouterLink>
      <span v-else>尚无处理入口</span>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import { ArrowUpRight, BellRing, Clock3, Mail, MessageSquareWarning, ServerCog, TrendingUp, Users, Utensils } from "@lucide/vue";
import { computed } from "vue";
import type { AdminMetric, DashboardCard } from "@/lib/admin-api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";

const props = defineProps<{ card: DashboardCard }>();
const labels = { not_integrated: "未接入", ok: "正常", partial: "部分可用", stale: "数据过期", unavailable: "不可用" } as const;
const icons = { users: Users, notice: BellRing, mail: Mail, feedback: MessageSquareWarning, food: Utensils, system: ServerCog } as const;
const statusLabel = computed(() => labels[props.card.status]);
const domainIcon = computed(() => icons[props.card.domain]);
const badgeVariant = computed(() => ({ not_integrated: "muted", ok: "success", partial: "warning", stale: "warning", unavailable: "destructive" } as const)[props.card.status]);

function formatMetric(item: AdminMetric) {
  if (item.value === null) return "—";
  if (item.code.includes("rate")) return `${(item.value * 100).toFixed(1)}%`;
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format(item.value);
}
function formatChange(value: number) { return `${value >= 0 ? "+" : ""}${(value * 100).toFixed(1)}%`; }
function changeClass(value: number) { return value >= 0 ? "text-emerald-600" : "text-rose-600"; }
function formatTime(value: string) { return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
</script>
