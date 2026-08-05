<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Label, PageHeader, Textarea } from "@/components/ui";
import { executeFoodCommand, fetchFoodWorkspace, resolveFoodOperation, type FoodAnomalyTicket, type FoodCommand, type FoodCommandKind, type FoodSubmission, type FoodTierAdjustment, type FoodWorkspace, type FoodWriteResult } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const workspace = ref<FoodWorkspace>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const feedback = ref("");
const busy = ref(false);
type PendingOperation = { kind: FoodCommandKind; key: string; input: FoodCommand; success: string };
type ConfirmTarget = { kind: FoodCommandKind; resourceID: string; version: number; title: string; confirmLabel: string; success: string; requireReason: boolean };
const pendingStorageKey = "henukit.food.pending-operation";
const pending = ref<PendingOperation>();
const confirmTarget = ref<ConfirmTarget>();
const confirmReason = ref("");
const foodCommandKinds: FoodCommandKind[] = ["submission_approve", "submission_reject", "anomaly_resolve", "anomaly_dismiss", "tier_adjustment_confirm", "tier_adjustment_reject"];
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
  if (stored && typeof stored.kind === "string" && foodCommandKinds.includes(stored.kind as FoodCommandKind) && typeof stored.key === "string" && typeof stored.success === "string" && !!stored.input && typeof stored.input === "object" && (stored.input as FoodCommand).kind === stored.kind) pending.value = stored as PendingOperation;
} catch { sessionStorage.removeItem(pendingStorageKey); }

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const result = await fetchFoodWorkspace();
  if (result.state === "authenticated") { workspace.value = result.workspace; state.value = "ready"; return; }
  state.value = result.state === "denied" ? "denied" : "unavailable";
}

function isConfirming(id: string, ...kinds: FoodCommandKind[]): boolean {
  const target = confirmTarget.value;
  return !!target && target.resourceID === id && kinds.includes(target.kind);
}

async function pollOperation(kind: FoodCommandKind, key: string): Promise<FoodWriteResult> {
  for (let attempt = 0; attempt < 3; attempt++) {
    if (attempt) await new Promise((resolve) => window.setTimeout(resolve, attempt * 250));
    const result = await resolveFoodOperation(kind, key);
    if (result.state === "unknown" || result.state === "unavailable") continue;
    return result;
  }
  return { state: "unavailable" };
}

async function finish(kind: FoodCommandKind, key: string, input: FoodCommand, success: string) {
  busy.value = true;
  let result = await executeFoodCommand(input, key);
  if (result.state === "unknown") {
    feedback.value = "操作结果待确认，正在核对…";
    persistPending({ kind, key, input, success });
    result = await pollOperation(kind, key);
  }
  if (result.state === "succeeded") { persistPending(); feedback.value = success; await refresh(); }
  else if (result.state === "conflict") { persistPending(); feedback.value = "数据有变化，请刷新后重试。"; }
  else if (result.state === "denied") { persistPending(); feedback.value = "当前账户缺少对应操作权限。"; }
  else if (result.state === "invalid") { persistPending(); feedback.value = "操作内容无效，请检查填写后重试。"; }
  else if (result.state === "signed_out") { persistPending(); feedback.value = "登录状态已过期，请重新登录后再操作。"; }
  else { persistPending({ kind, key, input, success }); feedback.value = "结果还没确认，可点下方按钮按原请求重试。"; }
  busy.value = false;
  confirmTarget.value = undefined;
  confirmReason.value = "";
}

async function retryPending() {
  const operation = pending.value;
  if (!operation || busy.value) return;
  await finish(operation.kind, operation.key, operation.input, operation.success);
}

function cancelConfirm() {
  confirmTarget.value = undefined;
  confirmReason.value = "";
}

function openSubmissionConfirm(item: FoodSubmission, approve: boolean) {
  confirmTarget.value = {
    kind: approve ? "submission_approve" : "submission_reject",
    resourceID: item.id,
    version: item.version,
    title: approve ? "批准投稿" : "拒绝投稿",
    confirmLabel: approve ? "确认批准投稿" : "确认拒绝投稿",
    success: approve ? "投稿已批准。" : "投稿已拒绝。",
    requireReason: !approve,
  };
  confirmReason.value = approve ? "Console 人工核验通过" : "";
}

function openAnomalyConfirm(item: FoodAnomalyTicket, resolve: boolean) {
  confirmTarget.value = {
    kind: resolve ? "anomaly_resolve" : "anomaly_dismiss",
    resourceID: item.id,
    version: item.version,
    title: resolve ? "标记已处理" : "驳回异常票",
    confirmLabel: resolve ? "确认标记已处理" : "确认驳回异常票",
    success: resolve ? "异常票已处理。" : "异常票已驳回。",
    requireReason: !resolve,
  };
  confirmReason.value = resolve ? "异常已核验并处理" : "";
}

function openTierConfirm(item: FoodTierAdjustment, confirm: boolean) {
  confirmTarget.value = {
    kind: confirm ? "tier_adjustment_confirm" : "tier_adjustment_reject",
    resourceID: item.id,
    version: item.version,
    title: confirm ? "确认调档" : "拒绝调档",
    confirmLabel: confirm ? "确认调档" : "确认拒绝调档",
    success: confirm ? "调档已确认。" : "调档已拒绝。",
    requireReason: !confirm,
  };
  confirmReason.value = confirm ? "确认调档建议" : "";
}

function submitConfirm() {
  const target = confirmTarget.value;
  if (!target || busy.value) return;
  const note = confirmReason.value.trim();
  if (note.length < 2) { feedback.value = target.requireReason ? "驳回类操作必须填写本次理由（至少 2 个字）。" : "操作理由至少需要 2 个字。"; return; }
  if (note.length > 1000) { feedback.value = "理由不能超过 1000 字。"; return; }
  const input: FoodCommand = { kind: target.kind, resource_id: target.resourceID, expected_version: target.version, payload: { note } };
  void finish(target.kind, operationKey(target.kind), input, target.success);
}

function anomalyKindLabel(kind: FoodAnomalyTicket["kind"]) {
  return kind === "duplicate" ? "重复投稿" : kind === "spam" ? "垃圾信息" : kind === "quality" ? "质量问题" : kind === "location" ? "位置问题" : `未知类型（${kind}）`;
}

function severityLabel(severity: FoodAnomalyTicket["severity"]) {
  return severity === "low" ? "低" : severity === "medium" ? "中" : severity === "high" ? "高" : `未知等级（${severity}）`;
}

function tierLabel(tier: FoodTierAdjustment["current_tier"]) {
  return tier === "featured" ? "精选" : tier === "recommended" ? "推荐" : tier === "standard" ? "标准" : tier === "watch" ? "观察" : `未知档位（${tier}）`;
}

function workspaceStatusLabel(status: string) {
  if (status === "ok") return "正常";
  if (status === "empty") return "暂无数据";
  if (status === "stale") return "数据过期";
  if (status === "loading") return "加载中";
  return `未知状态（${status}）`;
}

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh().then(() => { if (pending.value) feedback.value = "发现一项结果未确认的操作；可按原请求重试。"; }); return; }
  workspace.value = undefined;
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="food-heading">
    <PageHeader eyebrow="美食运营" title="Food 运营" title-id="food-heading" description="投稿、异常与调档均由服务端处理。">
      <div class="access-context"><strong>{{ workspaceStatusLabel(workspace?.status ?? "loading") }}</strong></div>
    </PageHeader>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取美食运营数据…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前账户没有美食运营权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state"><p>美食服务暂时不可用，请稍后重试。</p><Button class="mt-3" @click="refresh">重新加载</Button></div>
    <template v-else-if="workspace">
      <div v-if="workspace.stale" class="operation-notice mt-5" role="status"><strong>数据可能不是最新</strong><p>{{ workspace.status_message }}</p></div>
      <div v-if="feedback" class="operation-notice mt-5" role="status"><p>{{ feedback }}</p><Button v-if="pending && !busy" class="mt-3" @click="retryPending">按原请求重试</Button></div>
      <div class="operation-summary-grid mt-6"><article><span>投稿</span><strong>{{ workspace.submissions.length }}</strong></article><article><span>异常票</span><strong>{{ workspace.anomaly_tickets.length }}</strong></article><article><span>调档</span><strong>{{ workspace.tier_adjustments.length }}</strong></article></div>
      <div class="mt-6 grid gap-5 xl:grid-cols-3">
        <Card class="p-4" aria-labelledby="food-submissions"><h2 id="food-submissions" class="text-lg font-bold">投稿审核</h2><p v-if="!workspace.submissions.length" class="mt-3 text-muted-foreground">暂无待审核投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.submissions" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }} · {{ item.item_name }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.description }} · v{{ item.version }}</p><div v-if="canReview && item.status === 'pending'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'submission_approve', 'submission_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" :placeholder="confirmTarget?.requireReason ? '请填写本次操作理由' : '可修改本次操作理由'"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy" @click="openSubmissionConfirm(item, true)">批准投稿</Button><Button variant="ghost" :disabled="busy" @click="openSubmissionConfirm(item, false)">拒绝投稿</Button></div></div><p v-else-if="!canReview" class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="food-anomalies"><h2 id="food-anomalies" class="text-lg font-bold">异常票处理</h2><p v-if="!workspace.anomaly_tickets.length" class="mt-3 text-muted-foreground">暂无异常票。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.anomaly_tickets" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }} · {{ anomalyKindLabel(item.kind) }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.details }} · {{ severityLabel(item.severity) }} · v{{ item.version }}</p><div v-if="canHandleAnomaly && item.status === 'open'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'anomaly_resolve', 'anomaly_dismiss')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" :placeholder="confirmTarget?.requireReason ? '请填写本次操作理由' : '可修改本次操作理由'"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy" @click="openAnomalyConfirm(item, true)">标记已处理</Button><Button variant="ghost" :disabled="busy" @click="openAnomalyConfirm(item, false)">驳回异常票</Button></div></div><p v-else-if="!canHandleAnomaly" class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="food-tiers"><h2 id="food-tiers" class="text-lg font-bold">调档确认</h2><p v-if="!workspace.tier_adjustments.length" class="mt-3 text-muted-foreground">暂无待确认调档。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.tier_adjustments" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ tierLabel(item.current_tier) }} → {{ tierLabel(item.proposed_tier) }} · {{ item.reason }} · v{{ item.version }}</p><div v-if="canAdjustTier && item.status === 'pending'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'tier_adjustment_confirm', 'tier_adjustment_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" :placeholder="confirmTarget?.requireReason ? '请填写本次操作理由' : '可修改本次操作理由'"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy" @click="openTierConfirm(item, true)">确认调档</Button><Button variant="ghost" :disabled="busy" @click="openTierConfirm(item, false)">拒绝调档</Button></div></div><p v-else-if="!canAdjustTier" class="mt-3 text-sm">只读权限</p></li></ul></Card>
      </div>
    </template>
  </section>
</template>
