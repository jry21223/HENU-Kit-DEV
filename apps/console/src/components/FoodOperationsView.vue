<script setup lang="ts">
import { computed, ref, watch } from "vue";

import UiButton from "@/components/ui/UiButton.vue";
import { executeFoodCommand, fetchFoodWorkspace, resolveFoodOperation, type FoodAnomalyTicket, type FoodCommandKind, type FoodSubmission, type FoodTierAdjustment, type FoodWorkspace, type FoodWriteResult } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const workspace = ref<FoodWorkspace>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const feedback = ref("");
type PendingOperation = { kind: FoodCommandKind; key: string; success: string };
const pendingStorageKey = "henukit.food.pending-operation";
const pending = ref<PendingOperation>();
const canReview = computed(() => props.permissions.includes("food.review"));
const canHandleAnomaly = computed(() => props.permissions.includes("food.anomaly"));
const canAdjustTier = computed(() => props.permissions.includes("food.tier_adjust"));

function operationKey(kind: string) { return `idem_food_${kind}_${crypto.randomUUID()}`; }
function persistPending(value?: PendingOperation) {
  pending.value = value;
  if (value) sessionStorage.setItem(pendingStorageKey, JSON.stringify(value)); else sessionStorage.removeItem(pendingStorageKey);
}
try {
  const stored = JSON.parse(sessionStorage.getItem(pendingStorageKey) ?? "null") as Partial<PendingOperation> | null;
  if (stored && typeof stored.kind === "string" && typeof stored.key === "string" && typeof stored.success === "string") pending.value = stored as PendingOperation;
} catch { sessionStorage.removeItem(pendingStorageKey); }

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const result = await fetchFoodWorkspace();
  if (result.state === "authenticated") { workspace.value = result.workspace; state.value = "ready"; return; }
  state.value = result.state === "denied" ? "denied" : "unavailable";
}

async function finish(kind: FoodCommandKind, key: string, initial: Promise<FoodWriteResult>, success: string) {
  const result = await initial;
  if (result.state === "unknown") {
    feedback.value = "操作结果未知，正在按幂等键核对…";
    persistPending({ kind, key, success });
    await continuePending();
    return;
  }
  if (result.state === "succeeded") { persistPending(); feedback.value = success; await refresh(); return; }
  feedback.value = result.state === "conflict" ? "Food 数据版本已变化，请刷新后重试。" : result.state === "denied" ? "当前账户缺少对应 Food 权限。" : result.state === "invalid" ? "Food 操作内容无效。" : "操作暂未完成。";
}

async function continuePending() {
  const operation = pending.value;
  if (!operation) return;
  for (let attempt = 0; attempt < 3; attempt++) {
    if (attempt) await new Promise((resolve) => window.setTimeout(resolve, attempt * 250));
    const result = await resolveFoodOperation(operation.kind, operation.key);
    if (result.state === "unknown" || result.state === "unavailable") continue;
    persistPending();
    if (result.state === "succeeded") { feedback.value = operation.success; await refresh(); return; }
    feedback.value = result.state === "conflict" ? "Food 数据版本已变化，请刷新后重试。" : result.state === "denied" ? "当前账户缺少对应 Food 权限。" : "操作核对失败。";
    return;
  }
  feedback.value = "操作结果仍未知，已保留幂等键；请稍后继续核对。";
}

function run(kind: FoodCommandKind, resourceID: string, version: number, note: string, success: string) {
  const key = operationKey(kind);
  return finish(kind, key, executeFoodCommand({ kind, resource_id: resourceID, expected_version: version, payload: { note } }, key), success);
}

function reviewSubmission(item: FoodSubmission, approve: boolean) { return run(approve ? "submission_approve" : "submission_reject", item.id, item.version, approve ? "Console 人工核验通过" : "投稿信息不符合要求", approve ? "投稿已批准。" : "投稿已拒绝。"); }
function handleAnomaly(item: FoodAnomalyTicket, resolve: boolean) { return run(resolve ? "anomaly_resolve" : "anomaly_dismiss", item.id, item.version, resolve ? "异常已核验并处理" : "异常票不成立", resolve ? "异常票已处理。" : "异常票已驳回。"); }
function adjustTier(item: FoodTierAdjustment, confirm: boolean) { return run(confirm ? "tier_adjustment_confirm" : "tier_adjustment_reject", item.id, item.version, confirm ? "确认调档建议" : "调档依据不足", confirm ? "调档已确认。" : "调档已拒绝。"); }

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh().then(() => continuePending()); return; }
  workspace.value = undefined;
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="food-heading">
    <div class="overview-hero">
      <div><p class="eyebrow">Food owner operations</p><h1 id="food-heading" class="mt-2 text-2xl font-bold sm:text-3xl">Food 运营</h1><p class="mt-2 max-w-3xl leading-7 text-[var(--hk-ink-muted)]">投稿审核、异常票和调档确认均通过 Food API；Console 与 Gateway 不连接 Food 数据库。</p></div>
      <div class="access-context"><span>scope:product/food</span><strong>{{ workspace?.status ?? "loading" }}</strong></div>
    </div>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取 Food 工作区…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前账户缺少 Food 产品 Scope 或读取权限。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state"><p>Food API 暂不可用。</p><UiButton class="mt-3" @click="refresh">重新加载</UiButton></div>
    <template v-else-if="workspace">
      <div v-if="workspace.stale" class="operation-notice mt-5" role="status"><strong>数据可能已过期</strong><p>{{ workspace.status_message }}</p></div>
      <div v-if="feedback" class="operation-notice mt-5" role="status"><p>{{ feedback }}</p><UiButton v-if="pending" class="mt-3" @click="continuePending">继续核对</UiButton></div>
      <div class="operation-summary-grid mt-6"><article><span>投稿</span><strong>{{ workspace.submissions.length }}</strong></article><article><span>异常票</span><strong>{{ workspace.anomaly_tickets.length }}</strong></article><article><span>调档</span><strong>{{ workspace.tier_adjustments.length }}</strong></article></div>
      <div class="mt-6 grid gap-5 xl:grid-cols-3">
        <section class="operation-panel" aria-labelledby="food-submissions"><h2 id="food-submissions" class="text-lg font-bold">投稿审核</h2><p v-if="!workspace.submissions.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无待审核投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.submissions" :key="item.id" class="rounded-xl border p-3"><strong>{{ item.venue_name }} · {{ item.item_name }}</strong><p class="mt-1 text-sm text-[var(--hk-ink-muted)]">{{ item.description }} · v{{ item.version }}</p><div v-if="canReview && item.status === 'pending'" class="mt-3 flex flex-wrap gap-2"><UiButton @click="reviewSubmission(item, true)">批准投稿</UiButton><UiButton variant="ghost" @click="reviewSubmission(item, false)">拒绝投稿</UiButton></div><p v-else-if="!canReview" class="mt-3 text-sm">只读权限</p></li></ul></section>
        <section class="operation-panel" aria-labelledby="food-anomalies"><h2 id="food-anomalies" class="text-lg font-bold">异常票处理</h2><p v-if="!workspace.anomaly_tickets.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无异常票。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.anomaly_tickets" :key="item.id" class="rounded-xl border p-3"><strong>{{ item.venue_name }} · {{ item.kind }}</strong><p class="mt-1 text-sm text-[var(--hk-ink-muted)]">{{ item.details }} · {{ item.severity }} · v{{ item.version }}</p><div v-if="canHandleAnomaly && item.status === 'open'" class="mt-3 flex flex-wrap gap-2"><UiButton @click="handleAnomaly(item, true)">标记已处理</UiButton><UiButton variant="ghost" @click="handleAnomaly(item, false)">驳回异常票</UiButton></div><p v-else-if="!canHandleAnomaly" class="mt-3 text-sm">只读权限</p></li></ul></section>
        <section class="operation-panel" aria-labelledby="food-tiers"><h2 id="food-tiers" class="text-lg font-bold">调档确认</h2><p v-if="!workspace.tier_adjustments.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无待确认调档。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.tier_adjustments" :key="item.id" class="rounded-xl border p-3"><strong>{{ item.venue_name }}</strong><p class="mt-1 text-sm text-[var(--hk-ink-muted)]">{{ item.current_tier }} → {{ item.proposed_tier }} · {{ item.reason }} · v{{ item.version }}</p><div v-if="canAdjustTier && item.status === 'pending'" class="mt-3 flex flex-wrap gap-2"><UiButton @click="adjustTier(item, true)">确认调档</UiButton><UiButton variant="ghost" @click="adjustTier(item, false)">拒绝调档</UiButton></div><p v-else-if="!canAdjustTier" class="mt-3 text-sm">只读权限</p></li></ul></section>
      </div>
    </template>
  </section>
</template>
