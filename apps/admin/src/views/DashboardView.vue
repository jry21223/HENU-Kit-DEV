<template>
  <LegacyDashboardView v-if="config && !config.dashboard_v2_enabled" />
  <AdminShellV2 v-else title="运营总览" :environment="config?.environment ?? 'loading'">
    <div class="space-y-6">
      <section class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-sm font-medium text-primary">
            <Activity class="size-4" /> OPERATIONS OVERVIEW
          </div>
          <h1 class="text-3xl font-semibold tracking-tight text-slate-950 sm:text-4xl">今天需要处理什么？</h1>
          <p class="mt-2 text-sm text-muted-foreground">六个业务域始终保留，数据状态、趋势和异常在一个视图内完成判断。</p>
        </div>
        <Button variant="outline" :disabled="loading" @click="load">
          <RefreshCw :class="['size-4', { 'animate-spin': loading }]" />{{ loading ? "刷新中" : "刷新数据" }}
        </Button>
      </section>

      <div v-if="error" class="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{{ error }}</div>
      <div v-else-if="snapshot?.status === 'partial'" class="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
        <AlertTriangle class="size-4" /> 部分业务域降级；成功卡片和上次快照仍可继续使用。
      </div>

      <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Card v-for="item in overviewStats" :key="item.label">
          <CardHeader class="flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle class="text-sm font-medium text-muted-foreground">{{ item.label }}</CardTitle>
            <component :is="item.icon" :class="['size-4', item.iconClass]" />
          </CardHeader>
          <CardContent>
            <div class="text-2xl font-semibold tracking-tight">{{ item.value }}</div>
            <p class="mt-1 text-xs text-muted-foreground">{{ item.hint }}</p>
          </CardContent>
        </Card>
      </section>

      <section class="grid gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(320px,.8fr)]">
        <Card>
          <CardHeader class="gap-4 sm:flex-row sm:items-start sm:justify-between sm:space-y-0">
            <div>
              <CardTitle class="flex items-center gap-2"><TrendingUp class="size-5 text-primary" />平台趋势</CardTitle>
              <CardDescription class="mt-2">最近 14 天真实业务数据，来自统一指标口径接口。</CardDescription>
            </div>
            <Tabs v-model="selectedSeries" default-value="registered_users">
              <TabsList>
                <TabsTrigger value="registered_users">注册用户</TabsTrigger>
                <TabsTrigger value="new_users">新增用户</TabsTrigger>
                <TabsTrigger value="notice_imports">通知导入</TabsTrigger>
              </TabsList>
            </Tabs>
          </CardHeader>
          <CardContent>
            <div v-if="seriesLoading" class="h-[300px] animate-pulse rounded-lg bg-muted" />
            <TrendChart v-else :points="activeSeries?.points ?? []" :label="activeSeriesLabel" :color="activeSeriesColor" />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2"><CheckCircle2 class="size-5 text-primary" />业务接入状态</CardTitle>
            <CardDescription>六域实时连接情况与当前数据新鲜度。</CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            <div>
              <div class="mb-2 flex items-center justify-between text-sm"><span class="text-muted-foreground">可用域</span><strong>{{ availableDomains }}/6</strong></div>
              <div class="h-2 overflow-hidden rounded-full bg-muted"><div class="h-full rounded-full bg-primary transition-all" :style="{ width: `${availableDomains / 6 * 100}%` }" /></div>
            </div>
            <div class="space-y-3">
              <div v-for="card in orderedCards" :key="card.domain" class="flex items-center justify-between border-b pb-3 last:border-0 last:pb-0">
                <div class="flex items-center gap-2.5"><span :class="statusDot(card.status)" class="size-2 rounded-full" /><span class="text-sm font-medium">{{ card.title }}</span></div>
                <Badge :variant="statusVariant(card.status)">{{ statusText(card.status) }}</Badge>
              </div>
            </div>
          </CardContent>
        </Card>
      </section>

      <section>
        <div class="mb-4 flex items-center justify-between">
          <div><h2 class="text-xl font-semibold tracking-tight">六域业务快照</h2><p class="mt-1 text-sm text-muted-foreground">真实空值显示为 —，不会用零伪装数据。</p></div>
          <Badge variant="outline">数据时间 {{ snapshot ? formatTime(snapshot.as_of) : "—" }}</Badge>
        </div>
        <div v-if="loading && !snapshot" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <Card v-for="index in 6" :key="index" class="h-[310px] animate-pulse bg-muted" />
        </div>
        <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <MetricCard v-for="card in orderedCards" :key="card.domain" :card="card" />
        </div>
      </section>

      <Card>
        <CardHeader class="flex-row items-center justify-between space-y-0">
          <div><CardTitle class="flex items-center gap-2"><ListTodo class="size-5 text-primary" />今日待办</CardTitle><CardDescription class="mt-2">两档默认处理时限：紧急 24 小时，普通 72 小时。</CardDescription></div>
          <Badge :variant="urgentItems ? 'destructive' : 'secondary'">{{ actionItems.length }} 项</Badge>
        </CardHeader>
        <CardContent>
          <div v-if="actionItems.length" class="divide-y rounded-lg border">
            <RouterLink v-for="item in actionItems" :key="item.id" :to="item.action_path" class="grid gap-3 px-4 py-4 transition-colors hover:bg-muted/60 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center">
              <Badge :variant="item.urgency === 'urgent' ? 'destructive' : 'warning'">{{ item.urgency === "urgent" ? "24h" : "72h" }}</Badge>
              <div><strong class="text-sm">{{ item.summary }}</strong><p class="mt-1 text-xs text-muted-foreground">{{ domainName(item.domain) }} · 到期 {{ formatTime(item.due_at) }}</p></div>
              <ArrowUpRight class="size-4 text-muted-foreground" />
            </RouterLink>
          </div>
          <div v-else class="flex flex-col items-center justify-center py-10 text-center">
            <div class="mb-3 flex size-11 items-center justify-center rounded-full bg-emerald-50 text-emerald-700"><CheckCircle2 class="size-5" /></div>
            <strong class="text-sm">当前没有待办</strong><p class="mt-1 text-xs text-muted-foreground">真实空数据与未接入状态分开显示。</p>
          </div>
        </CardContent>
      </Card>
    </div>
  </AdminShellV2>
</template>

<script setup lang="ts">
import { Activity, AlertTriangle, ArrowUpRight, CheckCircle2, CircleGauge, ListTodo, RefreshCw, ServerCog, ShieldAlert, TrendingUp, Users } from "@lucide/vue";
import { useQuery } from "@tanstack/vue-query";
import { computed, ref } from "vue";
import AdminShellV2 from "@/components/AdminShellV2.vue";
import MetricCard from "@/components/admin/MetricCard.vue";
import TrendChart from "@/components/admin/TrendChart.vue";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { adminRequest, type ActionItem, type DashboardSnapshot, type IntegrationStatus, type MetricSeries, type UIConfig } from "@/lib/admin-api";
import LegacyDashboardView from "./LegacyDashboardView.vue";

const order = ["users", "notice", "mail", "feedback", "food", "system"];
const selectedSeries = ref("registered_users");
const configQuery = useQuery({ queryKey: ["admin", "ui-config"], queryFn: () => adminRequest<UIConfig>("/admin/ui-config") });
const config = computed(() => configQuery.data.value?.data ?? null);
const v2Enabled = computed(() => config.value?.dashboard_v2_enabled === true);
const snapshotQuery = useQuery({ queryKey: ["admin", "dashboard-snapshot"], queryFn: () => adminRequest<DashboardSnapshot>("/admin/dashboard-snapshots/latest"), enabled: v2Enabled });
const actionsQuery = useQuery({ queryKey: ["admin", "action-items"], queryFn: () => adminRequest<{ items: ActionItem[] }>("/admin/action-items"), enabled: v2Enabled });
const registeredSeries = useQuery({ queryKey: ["admin", "metric-series", "registered_users"], queryFn: () => adminRequest<MetricSeries>("/admin/metric-series?metric=registered_users"), enabled: v2Enabled });
const newUserSeries = useQuery({ queryKey: ["admin", "metric-series", "new_users"], queryFn: () => adminRequest<MetricSeries>("/admin/metric-series?metric=new_users"), enabled: v2Enabled });
const noticeSeries = useQuery({ queryKey: ["admin", "metric-series", "notice_imports"], queryFn: () => adminRequest<MetricSeries>("/admin/metric-series?metric=notice_imports"), enabled: v2Enabled });

const snapshot = computed(() => snapshotQuery.data.value?.data ?? null);
const actionItems = computed(() => actionsQuery.data.value?.data.items ?? []);
const orderedCards = computed(() => [...(snapshot.value?.cards ?? [])].sort((a, b) => order.indexOf(a.domain) - order.indexOf(b.domain)));
const loading = computed(() => configQuery.isFetching.value || snapshotQuery.isFetching.value || actionsQuery.isFetching.value);
const seriesLoading = computed(() => registeredSeries.isFetching.value || newUserSeries.isFetching.value || noticeSeries.isFetching.value);
const urgentItems = computed(() => actionItems.value.filter((item) => item.urgency === "urgent").length);
const availableDomains = computed(() => orderedCards.value.filter((card) => card.status === "ok" || card.status === "partial").length);
const userCard = computed(() => orderedCards.value.find((card) => card.domain === "users"));
const activeSeries = computed(() => ({ registered_users: registeredSeries.data.value?.data, new_users: newUserSeries.data.value?.data, notice_imports: noticeSeries.data.value?.data }[selectedSeries.value]));
const activeSeriesLabel = computed(() => ({ registered_users: "注册用户", new_users: "新增用户", notice_imports: "通知导入" }[selectedSeries.value] ?? "业务指标"));
const activeSeriesColor = computed(() => ({ registered_users: "#0f6b4f", new_users: "#2563eb", notice_imports: "#d97706" }[selectedSeries.value] ?? "#0f6b4f"));
const overviewStats = computed(() => [
  { label: "已验证用户", value: formatValue(userCard.value?.primary_metric.value), hint: "统一身份域实时汇总", icon: Users, iconClass: "text-blue-600" },
  { label: "可用业务域", value: `${availableDomains.value}/6`, hint: snapshot.value?.status === "partial" ? "存在部分失败" : "全部数据源响应正常", icon: CircleGauge, iconClass: "text-emerald-600" },
  { label: "紧急待办", value: urgentItems.value, hint: "默认 24 小时内处理", icon: ShieldAlert, iconClass: urgentItems.value ? "text-rose-600" : "text-slate-400" },
  { label: "系统状态", value: systemStatus(), hint: "API、PostgreSQL、Redis", icon: ServerCog, iconClass: "text-violet-600" },
]);
const error = computed(() => { const caught = configQuery.error.value ?? snapshotQuery.error.value ?? actionsQuery.error.value; return caught instanceof Error ? caught.message : caught ? "管理后台数据加载失败" : ""; });

async function load() {
  await configQuery.refetch();
  if (!v2Enabled.value) return;
  await Promise.all([snapshotQuery.refetch(), actionsQuery.refetch(), registeredSeries.refetch(), newUserSeries.refetch(), noticeSeries.refetch()]);
}
function formatValue(value: number | null | undefined) { return value === null || value === undefined ? "—" : new Intl.NumberFormat("zh-CN").format(value); }
function formatTime(value: string) { return new Intl.DateTimeFormat("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)); }
function systemStatus() { const card = orderedCards.value.find((item) => item.domain === "system"); return card?.status === "ok" ? "正常" : card ? "降级" : "—"; }
function statusText(status: IntegrationStatus) { return ({ not_integrated: "未接入", ok: "正常", partial: "部分可用", stale: "过期", unavailable: "不可用" } as const)[status]; }
function statusVariant(status: IntegrationStatus) { return ({ not_integrated: "muted", ok: "success", partial: "warning", stale: "warning", unavailable: "destructive" } as const)[status]; }
function statusDot(status: IntegrationStatus) { return status === "ok" ? "bg-emerald-500" : status === "partial" || status === "stale" ? "bg-amber-500" : status === "unavailable" ? "bg-rose-500" : "bg-slate-300"; }
function domainName(domain: ActionItem["domain"]) { return ({ users: "用户", notice: "校园通知", mail: "邮件", feedback: "反馈", food: "美食", system: "系统" } as const)[domain]; }
</script>
