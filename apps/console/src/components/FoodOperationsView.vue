<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { executeFoodCommand, fetchFoodWorkspace, resolveFoodOperation, type FoodAnomalyTicket, type FoodCampus, type FoodCommand, type FoodCommandKind, type FoodCommandPayload, type FoodOperationLookupResult, type FoodPost, type FoodPostTier, type FoodSubmission, type FoodTierAdjustment, type FoodWorkspace } from "@/lib/console-gateway";
import { clearPendingFoodOperation, readPendingFoodOperation, writePendingFoodOperation, type PendingFoodOperation } from "@/lib/pending-operations";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; operatorID?: string; permissions: string[] }>();
const workspace = ref<FoodWorkspace>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const feedback = ref("");
const busy = ref(false);
const campusFilter = ref<FoodCampus | "">(readCampusFilter());
let workspaceRequest = 0;
let principalGeneration = 0;
type ConfirmTarget = { kind: FoodCommandKind; resourceID: string; version: number; title: string; confirmLabel: string; success: string; requireReason: boolean; extraPayload?: Partial<FoodCommandPayload> };
type EditTarget = { resourceID: string; version: number; original: { venueName: string; itemName: string; description: string; campus: FoodCampus | "" } };
type PostEditTarget = { resourceID: string; version: number; original: { venueName: string; campus: FoodCampus; tier: FoodPostTier; reviewText: string; priceReference: string; hoursReference: string; hidden: boolean } };
const pending = ref<PendingFoodOperation>();
const confirmTarget = ref<ConfirmTarget>();
const confirmReason = ref("");
const editTarget = ref<EditTarget>();
const editVenueName = ref("");
const editItemName = ref("");
const editDescription = ref("");
const editCampus = ref<FoodCampus | "">("");
const editNote = ref("");
const postEditTarget = ref<PostEditTarget>();
const postEditVenueName = ref("");
const postEditCampus = ref<FoodCampus>("minglun");
const postEditTier = ref<FoodPostTier>("hang");
const postEditReviewText = ref("");
const postEditPriceReference = ref("");
const postEditHoursReference = ref("");
const postEditHidden = ref(false);
const postEditNote = ref("");
const canReview = computed(() => props.permissions.includes("food.review"));
const canHandleAnomaly = computed(() => props.permissions.includes("food.anomaly"));
const canAdjustTier = computed(() => props.permissions.includes("food.tier_adjust"));
const canQueryPending = computed(() => {
  const operation = pending.value;
  if (!operation || props.authState !== "authenticated" || operation.operator_id !== props.operatorID) return false;
  if (operation.operation.startsWith("anomaly_")) return canHandleAnomaly.value;
  if (operation.operation.startsWith("tier_adjustment_")) return canAdjustTier.value;
  return canReview.value;
});
const visiblePostCount = computed(() => workspace.value?.posts.filter((post) => !post.hidden).length ?? 0);
const hiddenPostCount = computed(() => workspace.value?.posts.filter((post) => post.hidden).length ?? 0);

function workspaceTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function readCampusFilter(): FoodCampus | "" {
  const value = new URL(window.location.href).searchParams.get("campus");
  return value === "minglun" || value === "jinming" || value === "longzihu" ? value : "";
}

function operationKey(kind: string) { return `idem_food_${kind}_${crypto.randomUUID()}`; }
function persistPending(value?: PendingFoodOperation) {
  if (value && !writePendingFoodOperation(value)) return false;
  if (!value) clearPendingFoodOperation();
  pending.value = value;
  return true;
}

function isCurrentOperator(generation: number, operatorID: string) {
  return generation === principalGeneration && props.authState === "authenticated" && props.operatorID === operatorID;
}

async function refresh() {
  const request = ++workspaceRequest;
  const generation = principalGeneration;
  const operatorID = props.operatorID;
  if (props.authState !== "authenticated" || !operatorID) { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const campus = campusFilter.value;
  const result = await fetchFoodWorkspace(campus || undefined);
  if (request !== workspaceRequest || campus !== campusFilter.value || !isCurrentOperator(generation, operatorID)) return;
  if (result.state === "authenticated") { workspace.value = result.workspace; state.value = "ready"; return; }
  state.value = result.state === "denied" ? "denied" : "unavailable";
}

function isConfirming(id: string, ...kinds: FoodCommandKind[]): boolean {
  const target = confirmTarget.value;
  return !!target && target.resourceID === id && kinds.includes(target.kind);
}

function isEditing(id: string): boolean {
  return editTarget.value?.resourceID === id;
}

async function pollOperation(operation: PendingFoodOperation, generation: number): Promise<FoodOperationLookupResult | undefined> {
  for (let attempt = 0; attempt < 3; attempt++) {
    if (attempt) await new Promise((resolve) => window.setTimeout(resolve, attempt * 250));
    if (!isCurrentOperator(generation, operation.operator_id)) return;
    if (!canQueryPending.value) return { state: "denied" };
    const result = await resolveFoodOperation(operation.operation, operation.idempotency_key);
    if (!isCurrentOperator(generation, operation.operator_id)) return;
    if (result.state === "unknown" || result.state === "unavailable") continue;
    return result;
  }
  return { state: "unavailable" };
}

async function applyOperationResult(result: FoodOperationLookupResult, operation: PendingFoodOperation, success: string, fromLookup = false) {
  if (fromLookup && (result.state === "denied" || result.state === "signed_out" || result.state === "invalid" || result.state === "conflict")) {
    feedback.value = result.state === "denied" ? "当前账户无法查询原操作结果，请联系管理员核对；结果仍未确认。" : result.state === "signed_out" ? "请重新登录后查询原操作结果；结果仍未确认。" : "原操作结果暂时无法查询，请稍后查询；结果仍未确认。";
    return;
  }
  if (fromLookup && result.state === "failed" && result.result.operation === operation.operation && result.result.resource_id === operation.resource_id) {
    persistPending();
    feedback.value = "原操作已确认未完成：内容有更新，请核对后重新选择并提交。";
    await refresh();
    return;
  }
  if (result.state === "succeeded" && result.result?.operation === operation.operation && result.result.resource_id === operation.resource_id) { persistPending(); feedback.value = success; await refresh(); }
  else if (result.state === "conflict") { persistPending(); feedback.value = "操作遇到冲突，请核对最新状态后再操作。"; await refresh(); }
  else if (result.state === "denied") { persistPending(); feedback.value = "当前账户缺少对应操作权限。"; }
  else if (result.state === "invalid") { persistPending(); feedback.value = "操作内容无效，请检查填写后重试。"; }
  else if (result.state === "signed_out") { persistPending(); feedback.value = "登录状态已过期，请重新登录后再操作。"; }
  else { feedback.value = "结果尚未确认，请查询操作结果；不会重新提交。"; }
}

function closeEditors() {
  confirmTarget.value = undefined;
  confirmReason.value = "";
  editTarget.value = undefined;
  postEditTarget.value = undefined;
}

async function finish(kind: FoodCommandKind, key: string, input: FoodCommand, success: string) {
  if (busy.value || pending.value || props.authState !== "authenticated" || !props.operatorID) return;
  const generation = principalGeneration;
  const operation: PendingFoodOperation = { version: 1, operator_id: props.operatorID, operation: kind, idempotency_key: key, resource_id: input.resource_id };
  if (!persistPending(operation)) {
    feedback.value = "暂时无法保存操作恢复信息，尚未提交。填写内容已保留，请检查浏览器的站点存储设置后重试。";
    return;
  }
  busy.value = true;
  let fromLookup = false;
  let result: FoodOperationLookupResult | undefined = await executeFoodCommand(input, key);
  if (!isCurrentOperator(generation, operation.operator_id)) return;
  if (result.state === "unknown") {
    fromLookup = true;
    feedback.value = "操作结果待确认，正在核对…";
    result = await pollOperation(operation, generation);
  }
  if (!result || !isCurrentOperator(generation, operation.operator_id)) return;
  await applyOperationResult(result, operation, success, fromLookup);
  if (!isCurrentOperator(generation, operation.operator_id)) return;
  busy.value = false;
  closeEditors();
}

async function retryPending() {
  const operation = pending.value;
  if (!operation || busy.value || operation.operator_id !== props.operatorID || props.authState !== "authenticated") return;
  if (!canQueryPending.value) {
    feedback.value = "当前账户缺少查询原操作所需权限，请联系管理员核对；结果仍未确认。";
    return;
  }
  const generation = principalGeneration;
  busy.value = true;
  const result = await pollOperation(operation, generation);
  if (!result || !isCurrentOperator(generation, operation.operator_id)) return;
  const success: Record<FoodCommandKind, string> = { submission_approve: "投稿已批准。", submission_reject: "投稿已拒绝。", submission_edit: "投稿信息已更新。", post_edit: "点评变更已完成。", anomaly_resolve: "异常票已处理。", anomaly_dismiss: "异常票已驳回。", tier_adjustment_confirm: "调档已确认。", tier_adjustment_reject: "调档已拒绝。" };
  await applyOperationResult(result, operation, success[operation.operation], true);
  if (isCurrentOperator(generation, operation.operator_id)) busy.value = false;
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
  confirmReason.value = "";
}

function openSubmissionEdit(item: FoodSubmission) {
  editTarget.value = {
    resourceID: item.id,
    version: item.version,
    original: { venueName: item.venue_name, itemName: item.item_name, description: item.description, campus: item.campus ?? "" },
  };
  editVenueName.value = item.venue_name;
  editItemName.value = item.item_name;
  editDescription.value = item.description;
  editCampus.value = item.campus ?? "";
  editNote.value = "";
  feedback.value = "";
}

function cancelEdit() {
  editTarget.value = undefined;
}

function submitEdit() {
  const target = editTarget.value;
  if (!target || busy.value) return;
  const note = editNote.value.trim();
  if (note.length < 2) { feedback.value = "操作理由至少需要 2 个字。"; return; }
  if (note.length > 1000) { feedback.value = "理由不能超过 1000 字。"; return; }
  const venueName = editVenueName.value.trim();
  const itemName = editItemName.value.trim();
  if (!venueName) { feedback.value = "店名不能为空。"; return; }
  if (venueName.length > 160) { feedback.value = "店名不能超过 160 字。"; return; }
  if (!itemName) { feedback.value = "菜名不能为空。"; return; }
  if (itemName.length > 160) { feedback.value = "菜名不能超过 160 字。"; return; }
  if (editDescription.value.length > 2000) { feedback.value = "描述不能超过 2000 字。"; return; }
  const payload: FoodCommandPayload = { note };
  if (venueName !== target.original.venueName) payload.venue_name = venueName;
  if (itemName !== target.original.itemName) payload.item_name = itemName;
  if (editDescription.value !== target.original.description) payload.description = editDescription.value;
  if (editCampus.value !== "" && editCampus.value !== target.original.campus) payload.campus = editCampus.value;
  if (!("venue_name" in payload) && !("item_name" in payload) && !("description" in payload) && !("campus" in payload)) { feedback.value = "至少修改一项内容。"; return; }
  const input: FoodCommand = { kind: "submission_edit", resource_id: target.resourceID, expected_version: target.version, payload };
  void finish("submission_edit", operationKey("submission_edit"), input, "投稿信息已更新。");
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
  confirmReason.value = "";
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
  confirmReason.value = "";
}

function submitConfirm() {
  const target = confirmTarget.value;
  if (!target || busy.value) return;
  const note = confirmReason.value.trim();
  if (note.length < 2) { feedback.value = target.requireReason ? "驳回类操作必须填写本次理由（至少 2 个字）。" : "操作理由至少需要 2 个字。"; return; }
  if (note.length > 1000) { feedback.value = "理由不能超过 1000 字。"; return; }
  const input: FoodCommand = { kind: target.kind, resource_id: target.resourceID, expected_version: target.version, payload: { note, ...target.extraPayload } };
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

function campusLabel(campus: FoodCampus | null) {
  return campus === "minglun" ? "明伦" : campus === "jinming" ? "金明" : campus === "longzihu" ? "龙子湖" : "未分配";
}

function postTierLabel(tier: FoodPostTier) {
  return tier === "hang" ? "夯" : tier === "top" ? "顶级" : tier === "elite" ? "人上人" : tier === "npc" ? "NPC" : tier === "bad" ? "拉完了" : `未知档位（${tier}）`;
}

function isPostEditing(id: string): boolean {
  return postEditTarget.value?.resourceID === id;
}

function openPostEdit(item: FoodPost) {
  postEditTarget.value = {
    resourceID: item.id,
    version: item.version,
    original: { venueName: item.venue_name, campus: item.campus, tier: item.tier, reviewText: item.review_text, priceReference: item.price_reference, hoursReference: item.hours_reference, hidden: item.hidden },
  };
  postEditVenueName.value = item.venue_name;
  postEditCampus.value = item.campus;
  postEditTier.value = item.tier;
  postEditReviewText.value = item.review_text;
  postEditPriceReference.value = item.price_reference;
  postEditHoursReference.value = item.hours_reference;
  postEditHidden.value = item.hidden;
  postEditNote.value = "";
  feedback.value = "";
}

function openPostHideToggle(item: FoodPost) {
  confirmTarget.value = {
    kind: "post_edit",
    resourceID: item.id,
    version: item.version,
    title: item.hidden ? "恢复展示" : "隐藏投稿",
    confirmLabel: item.hidden ? "确认恢复展示" : "确认隐藏投稿",
    success: item.hidden ? "投稿已恢复展示。" : "投稿已隐藏。",
    requireReason: true,
    extraPayload: { hidden: !item.hidden },
  };
  confirmReason.value = "";
}

function cancelPostEdit() {
  postEditTarget.value = undefined;
}

function submitPostEdit() {
  const target = postEditTarget.value;
  if (!target || busy.value) return;
  const note = postEditNote.value.trim();
  if (note.length < 2) { feedback.value = "操作理由至少需要 2 个字。"; return; }
  if (note.length > 1000) { feedback.value = "理由不能超过 1000 字。"; return; }
  const venueName = postEditVenueName.value.trim();
  if (!venueName) { feedback.value = "店名不能为空。"; return; }
  if (venueName.length > 160) { feedback.value = "店名不能超过 160 字。"; return; }
  const reviewText = postEditReviewText.value.trim();
  if (reviewText.length < 2) { feedback.value = "点评至少需要 2 个字。"; return; }
  if (reviewText.length > 2000) { feedback.value = "点评不能超过 2000 字。"; return; }
  if (postEditPriceReference.value.length > 200) { feedback.value = "价格参考不能超过 200 字。"; return; }
  if (postEditHoursReference.value.length > 200) { feedback.value = "营业时间参考不能超过 200 字。"; return; }
  const payload: FoodCommandPayload = { note };
  if (venueName !== target.original.venueName) payload.venue_name = venueName;
  if (postEditCampus.value !== target.original.campus) payload.campus = postEditCampus.value;
  if (postEditTier.value !== target.original.tier) payload.tier = postEditTier.value;
  if (reviewText !== target.original.reviewText) payload.review_text = reviewText;
  if (postEditPriceReference.value !== target.original.priceReference) payload.price_reference = postEditPriceReference.value;
  if (postEditHoursReference.value !== target.original.hoursReference) payload.hours_reference = postEditHoursReference.value;
  if (postEditHidden.value !== target.original.hidden) payload.hidden = postEditHidden.value;
  if (!("venue_name" in payload) && !("campus" in payload) && !("tier" in payload) && !("review_text" in payload) && !("price_reference" in payload) && !("hours_reference" in payload) && !("hidden" in payload)) { feedback.value = "至少修改一项内容。"; return; }
  const input: FoodCommand = { kind: "post_edit", resource_id: target.resourceID, expected_version: target.version, payload };
  void finish("post_edit", operationKey("post_edit"), input, "投稿信息已更新。");
}

function workspaceStatusLabel(status: string) {
  if (status === "ok") return "正常";
  if (status === "empty") return "暂无数据";
  if (status === "stale") return "数据过期";
  if (status === "loading") return "加载中";
  if (status === "denied") return "无访问权限";
  if (status === "unavailable") return "暂不可用";
  return `未知状态（${status}）`;
}

watch(() => [props.authState, props.operatorID] as const, ([value, operatorID]) => {
  principalGeneration++;
  workspaceRequest++;
  workspace.value = undefined;
  pending.value = undefined;
  busy.value = false;
  feedback.value = "";
  closeEditors();
  if (value === "authenticated" && operatorID) {
    pending.value = readPendingFoodOperation(operatorID);
    void refresh();
    if (pending.value) { feedback.value = "发现一项结果待确认的操作，正在查询原操作结果。"; void retryPending(); }
    return;
  }
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });

watch(campusFilter, (campus) => {
  closeEditors();
  const url = new URL(window.location.href);
  if (url.pathname.replace(/\/+$/, "") === "/food") {
    if (campus) url.searchParams.set("campus", campus); else url.searchParams.delete("campus");
    history.replaceState(history.state, "", url);
  }
  if (props.authState === "authenticated") void refresh();
});
onBeforeUnmount(() => { principalGeneration++; workspaceRequest++; });
</script>

<template>
  <section aria-labelledby="food-heading">
    <PageHeader eyebrow="美食运营" title="美食运营" title-id="food-heading" description="校区筛选用于投稿和点评；异常与调档保留全局队列。每类最多返回 200 条。">
      <div class="access-context"><strong>{{ workspaceStatusLabel(state === 'ready' ? workspace?.status ?? 'loading' : state) }}</strong></div>
      <label class="grid gap-1 text-sm">校区筛选<select v-model="campusFilter" aria-label="校区筛选" class="rounded-md border bg-transparent px-2 py-1 text-sm"><option value="">全部校区</option><option value="minglun">明伦</option><option value="jinming">金明</option><option value="longzihu">龙子湖</option></select></label>
    </PageHeader>
    <div v-if="feedback && props.authState === 'authenticated' && props.operatorID" class="operation-notice mt-5" role="status"><p>{{ feedback }}</p><Button v-if="pending" :disabled="busy || !canQueryPending" class="mt-3" @click="retryPending">查询操作结果</Button></div>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取美食运营数据…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前账户没有美食运营权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state"><p>美食服务暂时不可用，请稍后重试。</p><Button class="mt-3" @click="refresh">重新加载</Button></div>
    <template v-else-if="workspace">
      <p class="mt-4 text-sm text-muted-foreground">{{ workspace.stale ? '最近成功快照' : '数据截至' }} <time :datetime="workspace.as_of" :title="workspace.as_of">{{ workspaceTime(workspace.as_of) }}</time>（北京时间）<span v-if="workspace.stale">；快照最长保留 24 小时。</span></p>
      <div v-if="workspace.stale" class="operation-notice mt-5" role="status"><strong>数据可能不是最新</strong><p>{{ workspace.status_message }}</p></div>
      <div class="operation-summary-grid mt-6"><article><span>本次待审核</span><strong>{{ workspace.submissions.length }}</strong></article><article><span>本次全局异常</span><strong>{{ workspace.anomaly_tickets.length }}</strong></article><article><span>本次全局调档</span><strong>{{ workspace.tier_adjustments.length }}</strong></article><article><span>本次返回点评</span><strong>{{ workspace.posts.length }}</strong><p class="text-sm">公开可见 {{ visiblePostCount }} · 已隐藏 {{ hiddenPostCount }}</p></article></div>
      <div class="mt-6 grid gap-5 xl:grid-cols-3">
        <Card class="p-4" aria-labelledby="food-submissions"><h2 id="food-submissions" class="text-lg font-bold">投稿审核</h2><p v-if="!workspace.submissions.length" class="mt-3 text-muted-foreground">暂无待审核投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.submissions" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }} · {{ item.item_name }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.description }} · {{ campusLabel(item.campus) }} · v{{ item.version }}</p><div v-if="canReview && item.status === 'pending'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'submission_approve', 'submission_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelConfirm">取消</Button></div></div></template><div v-else-if="isEditing(item.id)" class="grid gap-2 rounded-lg border p-3"><strong>编辑投稿</strong><label class="grid gap-1 text-sm">店名<Input v-model="editVenueName" maxlength="160"></Input></label><label class="grid gap-1 text-sm">菜名<Input v-model="editItemName" maxlength="160"></Input></label><label class="grid gap-1 text-sm">描述<Textarea v-model="editDescription" maxlength="2000" rows="3"></Textarea></label><label class="grid gap-1 text-sm">校区<select v-model="editCampus" aria-label="投稿校区" class="rounded-md border bg-transparent px-2 py-1 text-sm"><option value="">未分配</option><option value="minglun">明伦</option><option value="jinming">金明</option><option value="longzihu">龙子湖</option></select></label><label class="grid gap-1 text-sm">操作理由<Textarea v-model="editNote" maxlength="1000" rows="2" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || editNote.trim().length < 2" @click="submitEdit">保存修改</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelEdit">取消</Button></div></div><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy || !!pending" @click="openSubmissionConfirm(item, true)">批准投稿</Button><Button variant="ghost" :disabled="busy || !!pending" @click="openSubmissionEdit(item)">编辑投稿</Button><Button variant="ghost" :disabled="busy || !!pending" @click="openSubmissionConfirm(item, false)">拒绝投稿</Button></div></div><p v-else-if="!canReview" class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="food-anomalies"><h2 id="food-anomalies" class="text-lg font-bold">异常票处理</h2><p v-if="!workspace.anomaly_tickets.length" class="mt-3 text-muted-foreground">暂无异常票。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.anomaly_tickets" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }} · {{ anomalyKindLabel(item.kind) }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.details }} · {{ severityLabel(item.severity) }} · v{{ item.version }}</p><div v-if="canHandleAnomaly && item.status === 'open'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'anomaly_resolve', 'anomaly_dismiss')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy || !!pending" @click="openAnomalyConfirm(item, true)">标记已处理</Button><Button variant="ghost" :disabled="busy || !!pending" @click="openAnomalyConfirm(item, false)">驳回异常票</Button></div></div><p v-else-if="!canHandleAnomaly" class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="food-tiers"><h2 id="food-tiers" class="text-lg font-bold">调档确认</h2><p v-if="!workspace.tier_adjustments.length" class="mt-3 text-muted-foreground">暂无待确认调档。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.tier_adjustments" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ tierLabel(item.current_tier) }} → {{ tierLabel(item.proposed_tier) }} · {{ item.reason }} · v{{ item.version }}</p><div v-if="canAdjustTier && item.status === 'pending'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'tier_adjustment_confirm', 'tier_adjustment_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy || !!pending" @click="openTierConfirm(item, true)">确认调档</Button><Button variant="ghost" :disabled="busy || !!pending" @click="openTierConfirm(item, false)">拒绝调档</Button></div></div><p v-else-if="!canAdjustTier" class="mt-3 text-sm">只读权限</p></li></ul></Card>
                <Card class="p-4" aria-labelledby="food-posts"><h2 id="food-posts" class="text-lg font-bold">已发布投稿</h2><p v-if="!workspace.posts.length" class="mt-3 text-muted-foreground">暂无已发布投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.posts" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.venue_name }} · {{ campusLabel(item.campus) }} · {{ postTierLabel(item.tier) }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.review_text }} · {{ item.author_display_name }}{{ item.hidden ? " · 已隐藏" : "" }} · v{{ item.version }}</p><div v-if="canReview" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'post_edit')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">操作理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || confirmReason.trim().length < 2" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelConfirm">取消</Button></div></div></template><div v-else-if="isPostEditing(item.id)" class="grid gap-2 rounded-lg border p-3"><strong>编辑已发布投稿</strong><label class="grid gap-1 text-sm">店名<Input v-model="postEditVenueName" maxlength="160"></Input></label><label class="grid gap-1 text-sm">校区<select v-model="postEditCampus" aria-label="投稿校区" class="rounded-md border bg-transparent px-2 py-1 text-sm"><option value="minglun">明伦</option><option value="jinming">金明</option><option value="longzihu">龙子湖</option></select></label><label class="grid gap-1 text-sm">档位<select v-model="postEditTier" aria-label="档位" class="rounded-md border bg-transparent px-2 py-1 text-sm"><option value="hang">夯</option><option value="top">顶级</option><option value="elite">人上人</option><option value="npc">NPC</option><option value="bad">拉完了</option></select></label><label class="grid gap-1 text-sm">点评<Textarea v-model="postEditReviewText" maxlength="2000" rows="3"></Textarea></label><label class="grid gap-1 text-sm">价格参考<Input v-model="postEditPriceReference" maxlength="200"></Input></label><label class="grid gap-1 text-sm">营业时间参考<Input v-model="postEditHoursReference" maxlength="200"></Input></label><label class="flex items-center gap-2 text-sm"><input v-model="postEditHidden" type="checkbox" class="rounded border">隐藏此投稿（公开榜单不再展示）</label><label class="grid gap-1 text-sm">操作理由<Textarea v-model="postEditNote" maxlength="1000" rows="2" placeholder="请填写本次操作理由"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || postEditNote.trim().length < 2" @click="submitPostEdit">保存修改</Button><Button variant="ghost" :disabled="busy || !!pending" @click="cancelPostEdit">取消</Button></div></div><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy || !!pending" @click="openPostEdit(item)">编辑已发布投稿</Button><Button variant="ghost" :disabled="busy || !!pending" @click="openPostHideToggle(item)">{{ item.hidden ? "恢复展示" : "隐藏投稿" }}</Button></div></div><p v-else class="mt-3 text-sm">只读权限</p></li></ul></Card>
      </div>
    </template>
  </section>
</template>
