<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from "vue";

import { Button, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { createNoticeSource, createNoticeVersion, distributeNoticeVersion, fetchNoticeSnapshot, resolveNoticeOperation, reviewNoticeVersion, type NoticeAudience, type NoticeSnapshot, type NoticeVersion, type NoticeWriteResult } from "@/lib/console-gateway";
import { consoleReturnTo } from "@/lib/console-session";
import { clearPendingNoticeOperation, readPendingNoticeOperation, writePendingNoticeOperation, type PendingNoticeOperation } from "@/lib/pending-operations";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; operatorID?: string; permissions: string[] }>();
const snapshot = ref<NoticeSnapshot>();
const versionStates = ["pending_review", "approved", "rejected", "distributed", "all"] as const;
const initialQuery = new URLSearchParams(window.location.search);
function isUUID(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(value);
}
const stateFilter = ref<(typeof versionStates)[number]>(versionStates.find((value) => value === initialQuery.get("state")) ?? "pending_review");
const selectionEstablished = ref(initialQuery.has("selected"));
const selectedID = ref(isUUID(initialQuery.get("selected")) ? initialQuery.get("selected")! : "");
const filteredVersions = computed(() => snapshot.value?.items.filter((item) => stateFilter.value === "all" || item.state === stateFilter.value) ?? []);
const selectedVersion = computed(() => snapshot.value?.items.find((item) => item.id === selectedID.value));
const managementOpen = ref(false);
const detailHeading = ref<HTMLElement>();
const filterControl = ref<HTMLSelectElement>();

function saveViewContext() {
  const location = new URL(consoleReturnTo(), window.location.origin);
  location.searchParams.set("state", stateFilter.value);
  if (selectedID.value) location.searchParams.set("selected", selectedID.value);
  else location.searchParams.delete("selected");
  window.history.replaceState({}, "", location.pathname + location.search + location.hash);
}
function returnToList() {
  selectionEstablished.value = true;
  selectedID.value = ""; saveViewContext();
  void nextTick(() => filterControl.value?.focus());
}
function selectVersion(id: string) {
  selectionEstablished.value = true;
  selectedID.value = id;
  saveViewContext();
  void nextTick(() => detailHeading.value?.focus({ preventScroll: true }));
}
function rememberManagementDisclosure(event: Event) {
  managementOpen.value = (event.currentTarget as HTMLDetailsElement).open;
}
watch(stateFilter, () => { selectionEstablished.value = true; if (selectedVersion.value) selectedID.value = ""; saveViewContext(); });
const state = ref<"loading" | "ready" | "stale" | "denied" | "not_configured" | "unavailable">("loading");
const feedback = ref("");
const busyID = ref("");
const sourceCode = ref("");
const sourceName = ref("");
const sourceCanonicalURL = ref("");
const versionSourceID = ref("");
const versionTitle = ref("");
const versionBody = ref("");
const versionSourceURL = ref("");
const versionPublishedAt = ref("");
const channel = ref<"in_app" | "email">("in_app");
const audienceKind = ref<"all_students" | "college" | "role">("all_students");
const audienceValue = ref("");
const reviewReasons = ref<Record<string, string>>({});
const pendingDistribution = ref<{ item: NoticeVersion; channel: "in_app" | "email"; audience: NoticeAudience }>();
const confirmDialog = ref<HTMLDialogElement>();
const pendingOperation = ref<PendingNoticeOperation>();
const restoredOperator = ref("");
let actorGeneration = 0;
let readGeneration = 0;
let operationGeneration = 0;
function currentActorBoundary() {
  const generation = actorGeneration;
  const operatorID = props.operatorID;
  return () => generation === actorGeneration && operatorID === props.operatorID && props.authState === "authenticated";
}
function currentOperationBoundary(command: PendingNoticeOperation) {
  const isCurrentActor = currentActorBoundary();
  const generation = operationGeneration;
  return () => isCurrentActor() && generation === operationGeneration &&
    pendingOperation.value?.idempotency_key === command.idempotency_key &&
    pendingOperation.value.operation === command.operation && pendingOperation.value.resource_id === command.resource_id;
}
onUnmounted(() => { actorGeneration++; readGeneration++; operationGeneration++; });
const canManage = computed(() => props.permissions.includes("notice.manage"));
const canReview = computed(() => props.permissions.includes("notice.review"));
const canDistribute = computed(() => props.permissions.includes("notice.distribute"));
const snapshotLabel = computed(() => state.value === "ready" ? `最近 50 个版本 · 本次返回 ${snapshot.value?.items.length ?? 0} 个` : state.value === "stale" ? "展示上次读取" : state.value === "loading" ? "加载中" : state.value === "denied" ? "无权访问" : "暂不可用");

function stateLabel(state: NoticeVersion["state"]) {
  return state === "pending_review" ? "待审核" : state === "approved" ? "已通过" : state === "rejected" ? "已拒绝" : state === "distributed" ? "已提交分发" : state;
}

function distributionStatusLabel(status: NoticeVersion["distribution_status"]) {
  return status === "delivered" ? "最近一次分发已送达" : status === "failed" ? "最近一次分发失败" : status === "processing" ? "最近一次分发正在处理，尚未确认送达" : status === "queued" ? "最近一次分发已排队，尚未确认送达" : "最近一次分发结果待同步";
}

function operationKey(kind: string) { return `idem_notice_${kind}_${crypto.randomUUID()}`; }

function currentAudience(): NoticeAudience {
  return audienceKind.value === "all_students" ? { kind: "all_students" as const } : { kind: audienceKind.value, value: audienceValue.value };
}

function channelLabel(value: "in_app" | "email") { return value === "in_app" ? "站内" : "邮件"; }

function audienceLabel(value: NoticeAudience) {
  if (value.kind === "all_students") return "全体学生";
  if (value.kind === "college") return `学院：${value.value ?? ""}`;
  if (value.kind === "role") return `角色：${value.value ?? ""}`;
  return "未知受众";
}

function reviewReason(item: NoticeVersion) { return (reviewReasons.value[item.id] ?? "").trim(); }

function openDistributionConfirm(item: NoticeVersion) {
  pendingDistribution.value = { item, channel: channel.value, audience: currentAudience() };
  feedback.value = "";
  void nextTick(() => { if (confirmDialog.value && !confirmDialog.value.open) confirmDialog.value.showModal(); });
}

function closeDistributionConfirm() {
  if (confirmDialog.value?.open) confirmDialog.value.close();
  pendingDistribution.value = undefined;
}

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  const isCurrentActor = currentActorBoundary();
  const request = ++readGeneration;
  state.value = "loading";
  const result = await fetchNoticeSnapshot();
  if (!isCurrentActor() || request !== readGeneration) return;
  if (result.state === "authenticated") {
    snapshot.value = result.snapshot; state.value = "ready";
    if (!selectionEstablished.value) { selectedID.value = filteredVersions.value[0]?.id ?? ""; selectionEstablished.value = true; }
    saveViewContext();
  }
  else if (result.state === "unavailable" && snapshot.value) { state.value = "stale"; }
  else { snapshot.value = undefined; state.value = result.state === "denied" ? "denied" : result.state === "not_configured" ? "not_configured" : "unavailable"; }
}

function clearPendingOperation() {
  operationGeneration++;
  pendingOperation.value = undefined;
  clearPendingNoticeOperation();
}

function confirmedOperation(command: PendingNoticeOperation, result: Record<string, unknown>): boolean {
  if (command.operation === "source_create") return isUUID(result.id) && result.code === command.resource_id && typeof result.name === "string" && result.name.trim() !== "" && typeof result.canonical_url === "string" && result.canonical_url.startsWith("https://");
  if (command.operation === "version_create") return isUUID(result.id) && result.source_id === command.resource_id && Number.isSafeInteger(result.version) && Number(result.version) > 0 && result.state === "pending_review" && Number.isSafeInteger(result.revision) && Number(result.revision) > 0;
  if (command.operation === "review") return result.version_id === command.resource_id && ["approved", "rejected"].includes(String(result.state)) && Number.isSafeInteger(result.revision) && Number(result.revision) > 1;
  return result.version_id === command.resource_id && result.status === "queued" && Number.isSafeInteger(result.revision) && Number(result.revision) > (command.distribution?.expected_revision ?? 0);
}
function unresolvedFeedback(result: NoticeWriteResult) {
  return result.state === "denied" ? "核对请求被拒绝，原请求仍已保留；请联系管理员。" : "结果仍未确认，原请求仍已保留；请勿重复提交，可稍后再次查询。";
}
async function finishOperation(command: PendingNoticeOperation, initial: () => Promise<NoticeWriteResult>, successMessage: string, onSuccess?: (result: Record<string, unknown>) => void) {
  operationGeneration++;
  pendingOperation.value = command;
  const ownsOperation = currentOperationBoundary(command);
  if (!writePendingNoticeOperation(command)) {
    pendingOperation.value = undefined; busyID.value = "";
    feedback.value = "浏览器无法安全保存原请求，操作未提交。请允许会话存储后重试。";
    return;
  }
  let result = await initial();
  if (!ownsOperation()) return;
  if (result.state === "conflict" || result.state === "invalid" || result.state === "denied") {
    clearPendingOperation(); busyID.value = "";
    feedback.value = result.state === "conflict" ? "内容已更新，请核对最新修订后操作。" : result.state === "denied" ? "操作被拒绝，请联系管理员确认权限。" : "操作内容无效，请检查后重试。";
    await refresh(); return;
  }
  if (result.state !== "succeeded" || !confirmedOperation(command, result.result)) {
    feedback.value = "结果尚未确认，已保留原请求；正在查询原结果，请勿重复提交。";
    result = await resolveNoticeOperation(command.operation, command.idempotency_key);
    if (!ownsOperation()) return;
  }
  if (result.state === "succeeded" && confirmedOperation(command, result.result)) {
    clearPendingOperation(); onSuccess?.(result.result);
    feedback.value = command.operation === "review" ? `${result.result.state === "approved" ? "审核已批准。" : "审核已拒绝。"} 已确认修订 r${result.result.revision}。` : successMessage;
    busyID.value = ""; await refresh();
  } else { feedback.value = unresolvedFeedback(result); }
}

function pendingCommand(operation: PendingNoticeOperation["operation"], resourceID: string, distribution?: PendingNoticeOperation["distribution"]): PendingNoticeOperation | undefined {
  if (!props.operatorID) { feedback.value = "当前登录身份尚未确认，请刷新后重试。"; return undefined; }
  const validTarget = operation === "source_create" ? /^[a-z0-9][a-z0-9-]{1,62}$/.test(resourceID) : isUUID(resourceID);
  if (!validTarget) { feedback.value = "操作目标无效，请检查来源或重新选择通知版本。"; return undefined; }
  const targetLabel = { source_create: "登记通知来源", version_create: "创建通知版本", review: "通知审核", distribution: "通知分发" }[operation];
  return { version: 1, operator_id: props.operatorID, operation, idempotency_key: operationKey(operation), resource_id: resourceID, target_label: targetLabel, ...(distribution ? { distribution } : {}) };
}

async function addSource() {
  if (!canManage.value || state.value !== "ready" || busyID.value || pendingOperation.value) return;
  const command = pendingCommand("source_create", sourceCode.value); if (!command) return;
  busyID.value = "source"; feedback.value = "正在创建通知来源…";
  await finishOperation(command, () => createNoticeSource({ code: sourceCode.value, name: sourceName.value, canonical_url: sourceCanonicalURL.value }, command.idempotency_key), "通知来源已创建。", (result) => {
    if (typeof result.id === "string") versionSourceID.value = result.id;
    sourceCode.value = ""; sourceName.value = ""; sourceCanonicalURL.value = "";
  });
}

async function addVersion() {
  if (!canManage.value || state.value !== "ready" || busyID.value || pendingOperation.value) return;
  const command = pendingCommand("version_create", versionSourceID.value); if (!command) return;
  busyID.value = "version"; feedback.value = "正在创建通知版本…";
  const published = versionPublishedAt.value ? new Date(versionPublishedAt.value).toISOString() : undefined;
  await finishOperation(command, () => createNoticeVersion(versionSourceID.value, { title: versionTitle.value, body: versionBody.value, source_url: versionSourceURL.value, ...(published ? { source_published_at: published } : {}) }, command.idempotency_key), "通知版本已创建并进入待审核状态。", () => {
    versionTitle.value = ""; versionBody.value = ""; versionSourceURL.value = ""; versionPublishedAt.value = "";
  });
}

async function review(item: NoticeVersion, decision: "approved" | "rejected") {
  if (!canReview.value || state.value !== "ready" || busyID.value || pendingOperation.value) return;
  const reason = reviewReason(item);
  if (decision === "rejected" && !reason) { feedback.value = "请填写驳回理由后再提交。"; return; }
  const command = pendingCommand("review", item.id); if (!command) return;
  busyID.value = item.id; feedback.value = "正在提交审核…";
  await finishOperation(command, () => reviewNoticeVersion(item.id, { decision, note: reason || undefined, expected_revision: item.revision }, command.idempotency_key), decision === "approved" ? "审核已批准。" : "审核已拒绝。", () => { reviewReasons.value[item.id] = ""; });
}

async function confirmDistribution() {
  if (!canDistribute.value || state.value !== "ready" || busyID.value || pendingOperation.value) return;
  const action = pendingDistribution.value;
  if (!action) return;
  const audience = action.audience.kind === "college" || action.audience.kind === "role" ? { kind: action.audience.kind, value: action.audience.value } : { kind: "all_students" as const };
  const distribution = { channel: action.channel, audience, expected_revision: action.item.revision };
  const command = pendingCommand("distribution", action.item.id, distribution); if (!command) return;
  busyID.value = action.item.id; feedback.value = "正在创建分发…";
  closeDistributionConfirm();
  await finishOperation(command, () => distributeNoticeVersion(action.item.id, distribution, command.idempotency_key), "分发任务已排队，尚未确认送达。");
}

async function resolvePendingOperationStatus() {
  const restored = pendingOperation.value;
  if (!restored) return;
  const ownsOperation = currentOperationBoundary(restored);
  busyID.value = restored.resource_id;
  feedback.value = "正在核对之前未确认的通知操作…";
  const result = await resolveNoticeOperation(restored.operation, restored.idempotency_key);
  if (!ownsOperation()) return;
  if (result.state === "succeeded" && confirmedOperation(restored, result.result)) {
    clearPendingOperation(); busyID.value = "";
    feedback.value = restored.operation === "distribution" ? "之前的分发请求已确认排队，尚未确认送达。" : restored.operation === "review" ? `之前的审核已${result.result.state === "approved" ? "批准" : "拒绝"}，已确认修订 r${result.result.revision}。` : "之前的操作已由通知服务确认完成。";
    await refresh();
  } else { feedback.value = unresolvedFeedback(result); }
}

async function restorePendingOperation() {
  const operatorID = props.operatorID;
  if (!operatorID) {
    pendingOperation.value = undefined;
    busyID.value = "";
    feedback.value = "";
    closeDistributionConfirm();
    restoredOperator.value = "";
    return;
  }
  if (restoredOperator.value === operatorID) return;
  pendingOperation.value = undefined;
  busyID.value = "";
  feedback.value = "";
  closeDistributionConfirm();
  restoredOperator.value = operatorID;
  const restored = readPendingNoticeOperation(operatorID);
  if (!restored) return;
  operationGeneration++;
  pendingOperation.value = restored;
  busyID.value = restored.resource_id;
  await resolvePendingOperationStatus();
}

watch(() => [props.authState, props.operatorID] as const, ([authState]) => {
  actorGeneration++; readGeneration++; operationGeneration++;
  snapshot.value = undefined;
  reviewReasons.value = {};
  managementOpen.value = false;
  sourceCode.value = ""; sourceName.value = ""; sourceCanonicalURL.value = "";
  versionSourceID.value = ""; versionTitle.value = ""; versionBody.value = ""; versionSourceURL.value = ""; versionPublishedAt.value = "";
  audienceValue.value = "";
  restoredOperator.value = "";
  pendingOperation.value = undefined; busyID.value = ""; feedback.value = "";
  closeDistributionConfirm();
  state.value = authState === "denied" ? "denied" : authState === "loading" ? "loading" : "unavailable";
  if (authState === "authenticated") { void refresh(); void restorePendingOperation(); }
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="notice-heading">
    <PageHeader eyebrow="通知流程" title="校园通知审核与分发" description="通知正文不可更改；审核与分发均由服务端记录。" title-id="notice-heading"><div class="access-context"><strong>{{ snapshotLabel }}</strong></div></PageHeader>
    <p v-if="feedback" class="mt-4 rounded-lg border border-border bg-white px-4 py-3" role="status">{{ feedback }} <Button v-if="pendingOperation" type="button" variant="ghost" @click="resolvePendingOperationStatus">再次查询结果</Button></p>
    <div v-if="state === 'loading'" class="mt-6 rounded-lg bg-white p-6" aria-busy="true">正在读取通知数据…</div>
    <div v-else-if="state === 'denied'" class="mt-6 rounded-lg bg-white p-6">当前账户无权查看通知工作台，请联系管理员。</div>
    <div v-else-if="state === 'not_configured'" class="mt-6 rounded-lg border border-border bg-background p-6"><p>通知工作台尚未接入，请联系管理员完成接入。此状态不代表校园通知停止服务。</p><Button class="mt-4" @click="refresh">重新检查接入</Button></div>
    <div v-else-if="state === 'unavailable'" class="mt-6 rounded-lg bg-white p-6"><p>通知服务暂时不可用，请稍后重试。</p><Button class="mt-4" @click="refresh">重新加载</Button></div>
    <details v-if="state === 'ready' && canManage" :open="managementOpen" class="mt-6 rounded-lg border border-border bg-background p-4" @toggle="rememberManagementDisclosure">
      <summary class="cursor-pointer text-sm font-medium">管理来源与通知版本</summary>
      <div class="mt-4 grid gap-4 lg:grid-cols-2">
      <form class="rounded-lg border border-border bg-white p-5" @submit.prevent="addSource">
        <h2 class="text-lg font-semibold">登记通知来源</h2>
        <div class="mt-4 grid gap-3">
          <Label class="grid gap-1">来源代码<Input v-model="sourceCode" required pattern="[a-z0-9][a-z0-9-]{1,62}" placeholder="henu-office" /></Label>
          <Label class="grid gap-1">来源名称<Input v-model="sourceName" required maxlength="120" placeholder="学校办公室" /></Label>
          <Label class="grid gap-1">来源主页<Input v-model="sourceCanonicalURL" required type="url" pattern="https://.*" placeholder="https://example.com/notices" /></Label>
        </div>
        <Button class="mt-4" type="submit" :disabled="busyID !== ''">创建来源</Button>
      </form>
      <form class="rounded-lg border border-border bg-white p-5" @submit.prevent="addVersion">
        <h2 class="text-lg font-semibold">创建通知版本</h2>
        <div class="mt-4 grid gap-3">
          <Label class="grid gap-1">来源 ID<Input v-model="versionSourceID" required placeholder="来源 ID" /></Label>
          <Label class="grid gap-1">标题<Input v-model="versionTitle" required maxlength="200" /></Label>
          <Label class="grid gap-1">正文<Textarea v-model="versionBody" required maxlength="100000" rows="4" /></Label>
          <Label class="grid gap-1">原文链接<Input v-model="versionSourceURL" required type="url" pattern="https://.*" /></Label>
          <Label class="grid gap-1">原文发布时间（可选）<Input v-model="versionPublishedAt" type="datetime-local" /></Label>
        </div>
        <Button class="mt-4" type="submit" :disabled="busyID !== ''">创建版本</Button>
      </form>
      </div>
    </details>
    <p v-if="state === 'stale'" class="mt-5 rounded-lg border border-border p-4" role="status">更新失败，展示上次读取；以下内容仅供核对，重新读取成功前不能审核或分发。</p>
    <div v-if="state === 'ready' || state === 'stale'" class="mt-6 space-y-3">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <Label class="grid gap-1">版本状态<select ref="filterControl" v-model="stateFilter" aria-label="版本状态筛选" class="min-h-11 rounded-md border border-border bg-background px-3 py-2"><option value="pending_review">待审核</option><option value="approved">已通过</option><option value="rejected">已拒绝</option><option value="distributed">已提交分发</option><option value="all">全部状态</option></select></Label>
        <Button variant="outline" :disabled="busyID !== ''" @click="refresh">刷新版本</Button>
      </div>
      <p class="text-sm text-muted-foreground">仅在最近 50 个版本内筛选，当前筛选 {{ filteredVersions.length }} 个。此列表不是全站通知统计。</p>
      <p v-if="snapshot" class="text-xs text-muted-foreground">读取时间（UTC）：<time :datetime="snapshot.generated_at">{{ new Date(snapshot.generated_at).toISOString() }}</time></p>
      <p v-if="selectedID && !selectedVersion" role="status" class="rounded-lg border border-border p-4">选中的通知版本不在最近 50 个版本中，请从当前列表重新选择。</p>
      <ul data-notice-list class="divide-y divide-border rounded-lg border border-border bg-background">
        <li v-for="item in filteredVersions" :key="item.id"><button type="button" class="flex min-h-14 w-full items-start justify-between gap-3 px-4 py-3 text-left focus-visible:outline-2 focus-visible:outline-ring" :aria-pressed="selectedID === item.id" @click="selectVersion(item.id)"><span class="min-w-0 break-words font-medium">{{ item.title }}<span class="mt-1 block text-xs font-normal text-muted-foreground">{{ item.source.name }} · 正文 v{{ item.version }}</span></span><span class="shrink-0 text-xs text-muted-foreground">{{ stateLabel(item.state) }}</span></button></li>
      </ul>
      <p v-if="filteredVersions.length === 0" class="rounded-lg border border-border p-5">{{ snapshot?.items.length ? '最近 50 个版本中没有符合当前筛选的通知。' : '当前没有通知版本。' }}</p>
    </div>
    <div v-if="(state === 'ready' || state === 'stale') && snapshot?.items.length" class="mt-6 grid gap-4">
      <article v-if="selectedVersion" :key="selectedVersion.id" data-notice-detail class="rounded-lg border border-border bg-white p-5 ">
        <Button variant="ghost" class="mb-3" @click="returnToList">返回版本列表</Button>
        <div class="flex flex-wrap items-start justify-between gap-3"><div><p class="eyebrow">{{ selectedVersion.source.name }} · v{{ selectedVersion.version }}</p><h2 ref="detailHeading" tabindex="-1" class="mt-1 break-words text-xl font-semibold">{{ selectedVersion.title }}</h2></div><span class="rounded-full bg-muted px-3 py-1 text-sm">{{ stateLabel(selectedVersion.state) }} · 修订 r{{ selectedVersion.revision }}</span></div>
        <p class="mt-4 whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-7">{{ selectedVersion.body }}</p>
        <a :href="selectedVersion.source_url" class="mt-3 inline-block text-sm underline" target="_blank" rel="noreferrer">核对原始来源</a>
    <div v-if="state === 'ready' && canDistribute && selectedVersion.state === 'approved'" class="mt-4 flex flex-wrap gap-3 rounded-lg border border-border bg-white p-4">
      <Label class="grid gap-1">渠道<select v-model="channel" class="rounded-md border px-3 py-2"><option value="in_app">站内</option><option value="email">邮件</option></select></Label>
      <Label class="grid gap-1">受众<select v-model="audienceKind" class="rounded-md border px-3 py-2"><option value="all_students">全体学生</option><option value="college">学院</option><option value="role">角色</option></select></Label>
      <Label v-if="audienceKind !== 'all_students'" class="grid flex-1 gap-1">受众值<Input v-model="audienceValue" required maxlength="120" placeholder="例如 software-college" /></Label>
    </div>
        <div class="mt-5 flex flex-wrap gap-2">
          <template v-if="state === 'ready' && selectedVersion.state === 'pending_review' && canReview">
            <Label class="grid w-full gap-1">审核理由<Textarea v-model="reviewReasons[selectedVersion.id]" maxlength="1000" rows="2" class="min-h-[64px]" placeholder="驳回必须填写理由；批准可留空。" /></Label>
            <Button :disabled="busyID !== ''" @click="review(selectedVersion, 'approved')">批准</Button>
            <Button variant="ghost" :disabled="busyID !== '' || !reviewReason(selectedVersion)" @click="review(selectedVersion, 'rejected')">拒绝</Button>
          </template>
          <Button v-if="state === 'ready' && selectedVersion.state === 'approved' && canDistribute" :disabled="busyID !== '' || (audienceKind !== 'all_students' && !audienceValue)" @click="openDistributionConfirm(selectedVersion)">创建分发任务</Button>
          <span v-if="selectedVersion.state === 'distributed'" class="text-sm text-muted-foreground">已创建 {{ selectedVersion.distribution_count }} 个分发任务 · {{ distributionStatusLabel(selectedVersion.distribution_status) }}</span>
        </div>
      </article>
    </div>
    <dialog
      ref="confirmDialog"
      class="m-auto w-[min(92vw,36rem)] rounded-lg border border-border bg-background p-5 shadow-xl backdrop:bg-foreground/30"
      aria-labelledby="distribution-confirm-heading"
      @close="pendingDistribution = undefined"
      @click.self="closeDistributionConfirm"
    >
      <form v-if="pendingDistribution" @submit.prevent="confirmDistribution">
        <p class="eyebrow">分发确认</p>
        <h2 id="distribution-confirm-heading" class="mt-1 text-xl font-semibold">确认创建分发任务</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">取消不会产生任何写入；确认后按以下渠道与受众创建分发任务。</p>
        <dl class="mt-4 grid gap-3 border-t border-border pt-4 text-sm">
          <div class="flex flex-wrap items-baseline justify-between gap-2"><dt class="text-muted-foreground">通知</dt><dd class="max-w-[24rem] break-words font-medium">{{ pendingDistribution.item.title }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">版本</dt><dd class="font-medium">正文 v{{ pendingDistribution.item.version }} · 修订 r{{ pendingDistribution.item.revision }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">渠道</dt><dd class="font-medium">{{ channelLabel(pendingDistribution.channel) }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">受众</dt><dd class="min-w-0 break-all text-right font-medium">{{ audienceLabel(pendingDistribution.audience) }}</dd></div>
        </dl>
        <div class="mt-5 flex justify-end gap-2">
          <Button type="button" variant="ghost" :disabled="busyID !== ''" @click="closeDistributionConfirm">取消</Button>
          <Button type="submit" :disabled="busyID !== ''">确认分发</Button>
        </div>
      </form>
    </dialog>
  </section>
</template>
