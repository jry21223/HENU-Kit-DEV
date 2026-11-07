<script setup lang="ts">
import { ArrowRight, BookOpen, Globe, MessageSquare, RefreshCw, ShieldCheck, Utensils } from "@lucide/vue";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Button } from "@/components/ui";
import type { ModuleSummary } from "@/data/modules";
import { fetchAccountTicketFacts, fetchFoodFacts, fetchLibraryReadiness, fetchNoticeSnapshot, fetchPlatformOperations, type ConsoleAccountTicketFacts, type ConsoleScope, type FoodFacts, type LibraryReadiness } from "@/lib/console-gateway";

const props = defineProps<{
  summaries: ModuleSummary[];
  overviewState: "loading" | "ready" | "unavailable";
  permissions: string[];
  scopes: ConsoleScope[];
  navigation: Array<{ href: string; label: string; permission: string }>;
}>();
defineEmits<{ refresh: [] }>();

type ReadState = "loading" | "ready" | "denied" | "unavailable";
const food = ref<Pick<FoodFacts, "counts" | "as_of">>();
const tickets = ref<Pick<ConsoleAccountTicketFacts, "counts" | "as_of">>();
const library = ref<LibraryReadiness>();
const foodState = ref<ReadState>("loading");
const ticketState = ref<ReadState>("loading");
const libraryState = ref<ReadState>("loading");
const platformState = ref<ReadState>("loading");
const noticeState = ref<ReadState | "not_configured">("loading");
const platform = ref<{ unresolved: number; returned: number; hasMore: boolean; deadLetters: number; createdBefore: string }>();
const notice = ref<{ pending: number; approved: number; queued: number; delivered: number; returned: number; sourceTime: string }>();
const controllers = new Set<AbortController>();
let closed = false;
let overallTimer: ReturnType<typeof setTimeout> | undefined;
const busy = computed(() => [foodState.value, ticketState.value, libraryState.value, platformState.value, noticeState.value].includes("loading") || props.overviewState === "loading");
function allowed(permission: string, product?: string) {
  return props.permissions.includes(permission) && props.scopes.some((scope) => product ? scope.kind === "product" && scope.product_code === product : scope.kind === "platform");
}
function entry(permission: string, product?: string) {
  return allowed(permission, product) ? props.navigation.find((item) => item.permission === permission) : undefined;
}
const foodEntry = computed(() => entry("food.read", "food"));
const ticketEntry = computed(() => entry("account.tickets.read", "account-portfolio"));
const platformEntry = computed(() => entry("platform.operations.read"));
const noticeEntry = computed(() => entry("notice.read", "notice"));
const libraryEntry = computed(() => entry("library.read", "library"));
const accountEntries = computed(() => props.navigation.filter((item) => ["account.membership.write", "account.points.adjust"].includes(item.permission) && allowed(item.permission, "account-portfolio")));
const allClear = computed(() => foodState.value === "ready" && ticketState.value === "ready" && food.value?.counts.pending_submissions === 0 && food.value.counts.open_anomalies === 0 && food.value.counts.pending_tier_adjustments === 0 && tickets.value?.counts.unresolved === 0);
function time(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}
function explanation(state: ReadState) {
  return state === "loading" ? "正在确认待办数量…" : state === "denied" ? "当前账户没有该模块的查看权限。" : "本次未能确认待办数量，请重试或进入工作台核对。";
}

// Abort transports that cooperate, and settle the UI even if a response or its
// JSON body arrives after the deadline. No late result can refill this view.
function bounded<T extends { state: string }>(read: (signal: AbortSignal) => Promise<T>): Promise<T | { state: "unavailable" }> {
  const controller = new AbortController();
  controllers.add(controller);
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result: T | { state: "unavailable" }) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      controller.signal.removeEventListener("abort", expired);
      controllers.delete(controller);
      resolve(result);
    };
    const expired = () => finish({ state: "unavailable" });
    const timer = setTimeout(() => controller.abort(), 2000);
    controller.signal.addEventListener("abort", expired, { once: true });
    void read(controller.signal).then(finish, () => finish({ state: "unavailable" }));
  });
}
async function load() {
  overallTimer = setTimeout(() => { for (const controller of controllers) controller.abort(); }, 3000);
  await Promise.all([
    (async () => {
      if (!foodEntry.value) { foodState.value = "denied"; return; }
      const result = await bounded((signal) => fetchFoodFacts({ signal }));
      if (closed) return;
      foodState.value = result.state === "authenticated" ? "ready" : result.state === "denied" ? "denied" : "unavailable";
      if (result.state === "authenticated") food.value = { counts: result.facts.counts, as_of: result.facts.as_of };
    })(),
    (async () => {
      if (!ticketEntry.value) { ticketState.value = "denied"; return; }
      const result = await bounded((signal) => fetchAccountTicketFacts("unresolved", { signal }));
      if (closed) return;
      ticketState.value = result.state === "authenticated" ? "ready" : result.state === "denied" ? "denied" : "unavailable";
      if (result.state === "authenticated") tickets.value = { counts: result.facts.counts, as_of: result.facts.as_of };
    })(),
    (async () => {
      if (!platformEntry.value) { platformState.value = "denied"; return; }
      const result = await bounded((signal) => fetchPlatformOperations(undefined, { signal }));
      if (closed) return;
      platformState.value = result.state === "authenticated" ? "ready" : result.state === "denied" ? "denied" : "unavailable";
      if (result.state === "authenticated") {
        const snapshot = result.operations;
        platform.value = { unresolved: snapshot.inbox_items.filter((item) => ["open", "in_progress", "blocked"].includes(item.status)).length, returned: snapshot.inbox_items.length, hasMore: snapshot.pagination.inbox_items.next_page !== null, deadLetters: snapshot.mail.dead_letters, createdBefore: snapshot.generated_at };
      }
    })(),
    (async () => {
      if (!noticeEntry.value) { noticeState.value = "denied"; return; }
      const result = await bounded((signal) => fetchNoticeSnapshot({ signal }));
      if (closed) return;
      noticeState.value = result.state === "authenticated" ? "ready" : result.state === "denied" ? "denied" : result.state === "not_configured" ? "not_configured" : "unavailable";
      if (result.state === "authenticated") {
        const snapshot = result.snapshot;
        notice.value = { pending: snapshot.items.filter((item) => item.state === "pending_review").length, approved: snapshot.items.filter((item) => item.state === "approved").length, queued: snapshot.items.filter((item) => item.distribution_status === "queued").length, delivered: snapshot.items.filter((item) => item.distribution_status === "delivered").length, returned: snapshot.items.length, sourceTime: snapshot.generated_at };
      }
    })(),
    (async () => {
      if (!libraryEntry.value) { libraryState.value = "denied"; return; }
      const result = await bounded((signal) => fetchLibraryReadiness({ signal }));
      if (closed) return;
      libraryState.value = result.state === "authenticated" ? "ready" : result.state === "denied" ? "denied" : "unavailable";
      if (result.state === "authenticated") library.value = result.readiness;
    })(),
  ]);
  clearTimeout(overallTimer);
}
onMounted(load);
onBeforeUnmount(() => {
  closed = true;
  clearTimeout(overallTimer);
  for (const controller of controllers) controller.abort();
});
function diagnosticLabel(summary: ModuleSummary) {
  if (summary.unavailableReason === "operator_disabled") return "摘要已停用";
  if (summary.unavailableReason === "not_onboarded") return "摘要未配置";
  return { loading: "正在读取摘要", ok: "摘要可读取", empty: "摘要返回为空", partial: "摘要部分可用", stale: "摘要已过期", unavailable: "本次摘要不可用", denied: "无摘要查看权限" }[summary.status];
}
const portal = computed(() => props.summaries.find((summary) => summary.id === "portal"));
const diagnosticGrants: Partial<Record<ModuleSummary["id"], [string, string?]>> = { platform: ["platform.operations.read"], notice: ["notice.read", "notice"], library: ["library.read", "library"], food: ["food.read", "food"] };
const diagnostics = computed(() => props.summaries.map((summary) => {
  const grant = diagnosticGrants[summary.id];
  if (grant && !allowed(grant[0], grant[1])) return { ...summary, statusMessage: "当前账户未获该模块查看权限，未展示业务来源内容。", asOf: undefined, lastSuccessAt: undefined, requestId: undefined };
  return summary;
}));
</script>

<template>
  <section class="overview-workspace" aria-labelledby="overview-heading">
    <header class="overview-heading">
      <div><h1 id="overview-heading">全局总览</h1><p class="overview-subtitle">从需要处理的事项，进入对应业务。</p></div>
      <Button variant="outline" :disabled="busy" aria-label="刷新总览" @click="$emit('refresh')"><RefreshCw :size="16" aria-hidden="true" :class="busy && 'is-refreshing'" />刷新</Button>
    </header>

    <section class="overview-section overview-attention" aria-labelledby="attention-heading">
      <div class="overview-section-heading"><h2 id="attention-heading">需要处理</h2><p>全局范围 · 各业务独立确认</p></div>
      <p v-if="allClear" class="overview-clear" role="status">美食与工单当前没有待处理事项。</p>
      <article class="overview-task" data-overview-task="food" :data-state="foodState" :aria-busy="foodState === 'loading'">
        <div class="overview-object"><Utensils :size="20" aria-hidden="true" /><div><h3>美食内容审核</h3><p v-if="foodState === 'ready' && food"><strong>{{ food.counts.pending_submissions }}</strong> 条投稿待审核<span class="overview-task-extra">异常 {{ food.counts.open_anomalies }} · 调档待确认 {{ food.counts.pending_tier_adjustments }}</span></p><p v-else role="status">{{ explanation(foodState) }}</p><p v-if="foodState === 'ready' && food" class="overview-source">全部校区 · 源数据 <time :datetime="food.as_of">{{ time(food.as_of) }}（北京时间）</time></p></div></div>
        <Button v-if="foodEntry" as="a" :href="foodEntry.href" :variant="foodState === 'ready' && food && food.counts.pending_submissions > 0 ? 'default' : 'outline'">进入美食运营<ArrowRight :size="16" aria-hidden="true" /></Button>
      </article>
      <article class="overview-task" data-overview-task="tickets" :data-state="ticketState" :aria-busy="ticketState === 'loading'">
        <div class="overview-object"><MessageSquare :size="20" aria-hidden="true" /><div><h3>账户工单</h3><p v-if="ticketState === 'ready' && tickets"><strong>{{ tickets.counts.unresolved }}</strong> 条未解决<span class="overview-task-extra">待处理 {{ tickets.counts.open }} · 处理中 {{ tickets.counts.in_progress }}</span></p><p v-else role="status">{{ explanation(ticketState) }}</p><p v-if="ticketState === 'ready' && tickets" class="overview-source">全部工单 · 未解决筛选 · 源数据 <time :datetime="tickets.as_of">{{ time(tickets.as_of) }}（北京时间）</time></p></div></div>
        <Button v-if="ticketEntry" as="a" :href="`${ticketEntry.href}?status=unresolved`" :variant="ticketState === 'ready' && tickets && tickets.counts.unresolved > 0 ? 'default' : 'outline'">查看未解决工单<ArrowRight :size="16" aria-hidden="true" /></Button>
      </article>
    </section>

    <section class="overview-section" aria-labelledby="business-heading">
      <div class="overview-section-heading"><h2 id="business-heading">业务概况</h2><p>可用入口与当前边界</p></div>
      <article class="overview-business-row" data-overview-platform :data-state="platformState"><div class="overview-object"><ShieldCheck :size="19" aria-hidden="true" /><div><h3>平台运营</h3><template v-if="platformState === 'ready' && platform"><p>运营收件箱 · 本页未结项 {{ platform.unresolved }} 条</p><p>本页载入 {{ platform.returned }} 条，最多 20 条<span v-if="platform.hasMore">；还有后续页</span>。<span class="overview-task-extra">邮件本次返回状态：未重新排队死信 {{ platform.deadLetters }} 条（全量）</span></p><p class="overview-source">列表创建时间上限 <time :datetime="platform.createdBefore">{{ time(platform.createdBefore) }}（北京时间）</time>；邮件未提供独立源时间。</p></template><p v-else role="status">{{ platformState === 'loading' ? '正在读取本页运营事实…' : platformState === 'denied' ? '当前账户没有平台运营查看权限。' : '本次未能读取平台运营事实，请重试或进入工作台核对。' }}</p></div></div><Button v-if="platformEntry" as="a" :href="platformEntry.href" variant="outline">进入平台工作台<ArrowRight :size="16" aria-hidden="true" /></Button></article>
      <article class="overview-business-row" data-overview-library><div class="overview-object"><BookOpen :size="19" aria-hidden="true" /><div><h3>学习资料</h3><p>公开目录与资料管理独立提供。</p><p v-if="libraryState === 'loading'" role="status">正在确认资料管理状态…</p><p v-else-if="libraryState === 'ready' && library?.management === 'not_ready'">资料管理尚未就绪；当前可查看公开目录。</p><p v-else-if="libraryState === 'ready' && library">服务已报告管理能力可用，当前后台版本尚未接入管理操作；当前可查看公开目录。</p><p v-else-if="libraryState === 'unavailable'" role="status">本次未能确认资料管理状态，可重试或进入运营页核对。</p><p v-else>当前账户没有资料管理查看权限，公开目录仍可直接访问。</p><p v-if="library" class="overview-source">能力检查 <time :datetime="library.checked_at">{{ time(library.checked_at) }}（北京时间）</time></p></div></div><div class="overview-row-actions"><Button as="a" href="https://henukit.cn/library" variant="outline">打开公开目录<ArrowRight :size="16" aria-hidden="true" /></Button><a v-if="libraryEntry" :href="libraryEntry.href" class="overview-text-link">资料管理状态</a></div></article>
      <article class="overview-business-row" data-overview-notice :data-state="noticeState"><div><h3>校园通知</h3><template v-if="noticeState === 'ready' && notice"><p>最近 50 个版本内：待审核 {{ notice.pending }} · 已通过 {{ notice.approved }}</p><p>本次载入 {{ notice.returned }} 个版本；各版本最近一次分发状态：排队 {{ notice.queued }} 个版本 · 标记送达 {{ notice.delivered }} 个版本。排队不表示送达。</p><p class="overview-source">源数据 <time :datetime="notice.sourceTime">{{ time(notice.sourceTime) }}（北京时间）</time></p></template><p v-else role="status">{{ noticeState === 'loading' ? '正在读取通知版本窗口…' : noticeState === 'denied' ? '当前账户没有通知运营查看权限。' : noticeState === 'not_configured' ? '通知工作台尚未接入，当前无法核对审核与分发状态。' : '本次未能读取通知版本，请重试或进入工作台核对。' }}</p></div><Button v-if="noticeEntry" as="a" :href="noticeEntry.href" variant="outline">进入通知工作台<ArrowRight :size="16" aria-hidden="true" /></Button></article>
      <article v-if="accountEntries.length" class="overview-business-row"><div><h3>会员与积分</h3><p>按指定账户查询与操作，不提供全站经营统计。</p></div><div class="overview-row-actions"><Button v-for="item in accountEntries" :key="item.href" as="a" :href="item.href" variant="outline">{{ item.label }}<ArrowRight :size="16" aria-hidden="true" /></Button></div></article>
      <article class="overview-business-row"><div class="overview-object"><Globe :size="19" aria-hidden="true" /><div><h3>公共站点</h3><p>打开 HENU Kit 公开主站。练习管理入口当前未开放。</p></div></div><Button as="a" href="https://henukit.cn" variant="outline">打开主站<ArrowRight :size="16" aria-hidden="true" /></Button></article>
    </section>

    <details class="overview-diagnostics">
      <summary>连接与诊断</summary>
      <p>这里只说明摘要接线与访问上下文。摘要失败不会替代上方独立读取的业务事实。</p>
      <p v-if="overviewState === 'loading'" role="status">正在读取摘要诊断…</p>
      <p v-else-if="overviewState === 'unavailable'" role="status">本次摘要诊断未能完成；已确认的业务事实仍可使用。</p>
      <dl v-else class="overview-diagnostic-list"><div v-for="summary in diagnostics" :key="summary.id"><dt>{{ summary.name }} · {{ diagnosticLabel(summary) }}</dt><dd>{{ summary.statusMessage }}<span v-if="summary.asOf"> · 来源时间 <time :datetime="summary.asOf">{{ time(summary.asOf) }}（北京时间）</time></span><span v-if="summary.lastSuccessAt"> · 最近成功 <time :datetime="summary.lastSuccessAt">{{ time(summary.lastSuccessAt) }}（北京时间）</time></span><code v-if="summary.requestId">{{ summary.requestId }}</code></dd></div></dl>
      <dl v-if="portal && overviewState === 'ready'" class="overview-diagnostic-list"><div v-for="metric in portal.metrics" :key="metric.label"><dt>Portal · {{ metric.label }}</dt><dd>{{ metric.value }}<span v-if="metric.hint"> · {{ metric.hint }}</span></dd></div></dl>
      <p v-if="food">美食事实原始时间：<code>{{ food.as_of }}</code></p><p v-if="tickets">工单事实原始时间：<code>{{ tickets.as_of }}</code></p><p v-if="library">资料能力检查原始时间：<code>{{ library.checked_at }}</code></p>
      <p>当前账户的权限</p><div class="overview-permissions" aria-label="服务端验证的权限代码"><code v-for="permission in permissions" :key="permission">{{ permission }}</code></div>
    </details>
  </section>
</template>
