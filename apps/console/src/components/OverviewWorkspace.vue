<script setup lang="ts">
import { Activity, ArrowRight, Bell, BookOpen, Building2, RefreshCw, ShieldCheck, Utensils } from "@lucide/vue";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import ModuleEvidence from "@/components/ModuleEvidence.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import type { ModuleId, ModuleStatus, ModuleSummary } from "@/data/modules";
import { localDateTime } from "@/lib/format";

const props = defineProps<{
  summaries: ModuleSummary[];
  generatedAt?: string;
  authenticated: boolean;
  overviewGranted: boolean;
  navigation: Array<{ href: string; label: string; permission: string }>;
}>();
defineEmits<{ refresh: [] }>();

const icons = { portal: Building2, platform: ShieldCheck, notice: Bell, library: BookOpen, quizcraft: Activity, food: Utensils };
const selectedId = ref<ModuleId>("portal");
const isMobile = ref(false);
const mobileExpanded = ref(false);
const sourceButtons = new Map<ModuleId, HTMLButtonElement>();
let media: MediaQueryList | undefined;
const selected = computed(() => props.summaries.find((module) => module.id === selectedId.value));
const portal = computed(() => props.summaries.find((module) => module.id === "portal"));
const busy = computed(() => props.summaries.some((module) => module.status === "loading"));
const readableCount = computed(() => props.summaries.filter((module) => !["unavailable", "denied", "loading"].includes(module.status)).length);
const notOnboardedCount = computed(() => props.summaries.filter((module) => module.unavailableReason === "not_onboarded").length);
const portalDeploymentLabels = ["部署版本", "Commit", "部署时间", "Readiness", "关键探测", "入口健康", "当前异常"];
const permissionByModule: Partial<Record<ModuleId, string>> = { platform: "platform.operations.read", notice: "notice.read", library: "library.read", food: "food.read" };
const statusLabels: Record<ModuleStatus, string> = { ok: "摘要可读取", loading: "读取中", empty: "暂无数据", partial: "部分可用", stale: "数据已过期", unavailable: "暂不可用", denied: "无权读取" };

function statusLabel(summary: ModuleSummary) {
  return summary.unavailableReason === "not_onboarded" ? "尚未接入" : summary.unavailableReason === "operator_disabled" ? "已停用" : statusLabels[summary.status];
}
function badgeStatus(summary: ModuleSummary): ModuleStatus { return summary.unavailableReason ? "denied" : summary.status; }
function businessMetrics(summary: ModuleSummary) { return summary.id === "portal" ? summary.metrics.filter((metric) => !portalDeploymentLabels.includes(metric.label)) : summary.metrics; }
function portalMetric(label: string) { return portal.value?.metrics.find((metric) => metric.label === label); }
function destination(summary: ModuleSummary) { return props.navigation.find((item) => item.permission === permissionByModule[summary.id]); }
function observationDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).format(date);
}
function observationClock(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(date);
}
function updateViewport() { isMobile.value = media?.matches ?? false; }
onMounted(() => {
  if (typeof window.matchMedia !== "function") return;
  media = window.matchMedia("(max-width: 760px)");
  updateViewport();
  media.addEventListener("change", updateViewport);
});
onBeforeUnmount(() => media?.removeEventListener("change", updateViewport));
async function selectModule(id: ModuleId) {
  selectedId.value = id;
  mobileExpanded.value = true;
  await nextTick();
  if (isMobile.value) sourceButtons.get(id)?.scrollIntoView?.({ block: "start", behavior: "instant" });
}
async function closeMobileEvidence() {
  mobileExpanded.value = false;
  await nextTick();
  sourceButtons.get(selectedId.value)?.focus({ preventScroll: true });
  sourceButtons.get(selectedId.value)?.scrollIntoView?.({ block: "nearest", behavior: "instant" });
}
</script>

<template>
  <section class="overview-workspace" :aria-busy="busy" aria-labelledby="overview-heading">
    <header class="overview-heading">
      <div><h1 id="overview-heading">全局平台概览</h1></div>
      <div class="overview-heading-actions"><span v-if="generatedAt" class="overview-generated">最近汇总 {{ localDateTime(generatedAt) }}</span><button type="button" class="overview-refresh" :disabled="busy || !authenticated" aria-label="刷新概览" @click="$emit('refresh')"><RefreshCw :size="17" :class="busy && 'is-refreshing'" /></button></div>
    </header>

    <section class="overview-coverage" aria-labelledby="coverage-heading">
      <div class="coverage-heading"><h2 id="coverage-heading">模块摘要与可用性</h2><p v-if="notOnboardedCount">{{ notOnboardedCount }} 项摘要尚未接入</p></div>
      <div class="coverage-modules"><button v-for="summary in summaries" :key="summary.id" type="button" :class="['coverage-module', selectedId === summary.id && 'is-selected']" :aria-label="`选择 ${summary.name} 模块`" @click="selectModule(summary.id)"><component :is="icons[summary.id]" :size="17" aria-hidden="true" /><span><strong>{{ summary.name }}</strong><small :data-status="summary.status" :data-configured-state="summary.unavailableReason">{{ statusLabel(summary) }}</small></span></button></div>
    </section>

    <div class="overview-cross-section">
      <section class="overview-ledger" aria-labelledby="ledger-heading">
        <header class="ledger-heading"><h2 id="ledger-heading">模块证据对照</h2><span class="ledger-module-count">{{ summaries.length }} 个模块</span></header>
        <div class="ledger-columns" aria-hidden="true"><span>模块</span><span>业务事实</span><span>摘要状态</span><span>观测时间</span><span>证据</span></div>
        <article v-for="summary in summaries" :key="summary.id" :data-module-card="summary.id" :data-state="summary.status" :aria-label="`${summary.name}：${statusLabel(summary)}`" :class="['overview-module', selectedId === summary.id && 'is-selected']">
          <button :ref="(element) => { if (element) sourceButtons.set(summary.id, element as HTMLButtonElement); }" type="button" class="overview-module-row" :aria-label="`查看 ${summary.name} 证据`" :aria-expanded="isMobile ? selectedId === summary.id && mobileExpanded : undefined" @click="selectModule(summary.id)">
            <span class="ledger-module"><span class="ledger-module-icon"><component :is="icons[summary.id]" :size="19" aria-hidden="true" /></span><span><strong>{{ summary.name }}</strong><small>{{ summary.eyebrow }}</small></span></span>
            <span class="ledger-facts"><template v-if="summary.status === 'loading'"><span class="overview-skeleton" /><span class="sr-only">{{ summary.name }} 正在加载</span></template><template v-else-if="businessMetrics(summary).length"><span v-for="metric in businessMetrics(summary)" :key="metric.label" class="ledger-metric"><small>{{ metric.label }}</small><strong>{{ metric.value }}</strong></span><span v-if="summary.status === 'partial' || summary.status === 'stale'" class="ledger-state-message ledger-metric-explanation">{{ summary.statusMessage }}</span></template><span v-else class="ledger-state-message">{{ summary.statusMessage }}</span></span>
            <span class="ledger-status"><StatusBadge :status="badgeStatus(summary)">{{ statusLabel(summary) }}</StatusBadge></span>
            <span class="ledger-time"><time v-if="summary.asOf" :datetime="summary.asOf" class="ledger-observation"><span>{{ observationDate(summary.asOf) }}</span><span>{{ observationClock(summary.asOf) }}</span></time><span v-else>观测时间未提供</span><small v-if="summary.status === 'stale' && summary.lastSuccessAt">最近成功见证据</small></span>
            <span class="ledger-view">查看<ArrowRight :size="14" aria-hidden="true" /></span>
          </button>
          <ModuleEvidence v-if="isMobile && mobileExpanded && selectedId === summary.id" :summary="summary" :metrics="businessMetrics(summary)" :status-label="statusLabel(summary)" :badge-status="badgeStatus(summary)" :destination="destination(summary)" closable @close="closeMobileEvidence" />
        </article>
        <p class="ledger-boundary">尚未接入表示没有摘要来源；暂不可用表示本次读取未完成。</p>
      </section>

      <aside class="overview-secondary" aria-label="独立探测与选中模块证据">
        <section v-if="portal" class="portal-evidence" aria-labelledby="portal-evidence-heading">
          <header class="portal-evidence-heading"><div><p class="overview-eyebrow">PORTAL / 公共站点</p><h2 id="portal-evidence-heading">发布与本轮探测</h2></div><Building2 :size="18" aria-hidden="true" /></header>
          <div v-if="portalMetric('部署版本')" class="portal-release"><small>部署版本</small><strong>{{ portalMetric('部署版本')?.value }}</strong></div>
          <dl class="portal-facts"><template v-for="label in portalDeploymentLabels.filter((label) => label !== '部署版本')" :key="label"><div v-if="portalMetric(label)"><dt>{{ label }}</dt><dd>{{ portalMetric(label)?.value }}<small v-if="portalMetric(label)?.hint">{{ portalMetric(label)?.hint }}</small></dd></div></template></dl>
          <p class="portal-summary-message">{{ portal.statusMessage }}</p>
          <p class="portal-boundary">本轮 HTTP 响应是可达性证据，不能代表通知送达、下载完成或作答成功。</p>
        </section>
        <ModuleEvidence v-if="!isMobile && selected" :summary="selected" :metrics="businessMetrics(selected)" :status-label="statusLabel(selected)" :badge-status="badgeStatus(selected)" :destination="destination(selected)" />
      </aside>
    </div>

    <div class="overview-access" aria-label="服务端验证的访问上下文"><span>{{ overviewGranted ? '已授予概览查看权限' : authenticated ? '已登录，访问范围见当前账户权限' : '需要服务端权限' }}</span><strong>{{ readableCount }}/{{ summaries.length }} 摘要可读取</strong></div>
  </section>
</template>
