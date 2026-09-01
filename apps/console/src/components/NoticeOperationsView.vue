<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";

import { Button, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { createNoticeSource, createNoticeVersion, distributeNoticeVersion, fetchNoticeSnapshot, resolveNoticeOperation, reviewNoticeVersion, type NoticeAudience, type NoticeSnapshot, type NoticeVersion, type NoticeWriteResult } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const snapshot = ref<NoticeSnapshot>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
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
const canManage = computed(() => props.permissions.includes("notice.manage"));
const canReview = computed(() => props.permissions.includes("notice.review"));
const canDistribute = computed(() => props.permissions.includes("notice.distribute"));
const snapshotLabel = computed(() => state.value === "ready" ? `${snapshot.value?.items.length ?? 0} 个版本` : state.value === "loading" ? "加载中" : state.value === "denied" ? "无权访问" : "暂不可用");

function stateLabel(state: NoticeVersion["state"]) {
  return state === "pending_review" ? "待审核" : state === "approved" ? "已通过" : state === "rejected" ? "已拒绝" : state === "distributed" ? "已分发" : state;
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
  state.value = "loading";
  const result = await fetchNoticeSnapshot();
  if (result.state === "authenticated") { snapshot.value = result.snapshot; state.value = "ready"; }
  else state.value = result.state === "denied" ? "denied" : "unavailable";
}

async function finishOperation(kind: "source_create" | "version_create" | "review" | "distribution", key: string, initial: Promise<NoticeWriteResult>, successMessage: string, onSuccess?: (result: Record<string, unknown>) => void) {
  let result = await initial;
  if (result.state === "unknown") { feedback.value = "操作结果待确认，正在核对…"; result = await resolveNoticeOperation(kind, key); }
  if (result.state === "succeeded") { onSuccess?.(result.result); feedback.value = successMessage; await refresh(); }
  else if (result.state === "conflict") { feedback.value = "内容已更新，请基于最新状态操作。"; await refresh(); }
  else feedback.value = "操作没有完成，请刷新页面后重试；如仍失败请联系管理员。";
  busyID.value = "";
}

async function addSource() {
  busyID.value = "source"; feedback.value = "正在创建通知来源…"; const key = operationKey("source_create");
  await finishOperation("source_create", key, createNoticeSource({ code: sourceCode.value, name: sourceName.value, canonical_url: sourceCanonicalURL.value }, key), "通知来源已创建。", (result) => {
    if (typeof result.id === "string") versionSourceID.value = result.id;
    sourceCode.value = ""; sourceName.value = ""; sourceCanonicalURL.value = "";
  });
}

async function addVersion() {
  busyID.value = "version"; feedback.value = "正在创建通知版本…"; const key = operationKey("version_create");
  const published = versionPublishedAt.value ? new Date(versionPublishedAt.value).toISOString() : undefined;
  await finishOperation("version_create", key, createNoticeVersion(versionSourceID.value, { title: versionTitle.value, body: versionBody.value, source_url: versionSourceURL.value, ...(published ? { source_published_at: published } : {}) }, key), "通知版本已创建并进入待审核状态。", () => {
    versionTitle.value = ""; versionBody.value = ""; versionSourceURL.value = ""; versionPublishedAt.value = "";
  });
}

async function review(item: NoticeVersion, decision: "approved" | "rejected") {
  const reason = reviewReason(item);
  if (decision === "rejected" && !reason) { feedback.value = "请填写驳回理由后再提交。"; return; }
  busyID.value = item.id; feedback.value = "正在提交审核…"; const key = operationKey("review");
  await finishOperation("review", key, reviewNoticeVersion(item.id, { decision, note: reason || undefined, expected_revision: item.revision }, key), decision === "approved" ? "审核已批准。" : "审核已拒绝。", () => { reviewReasons.value[item.id] = ""; });
}

async function confirmDistribution() {
  const action = pendingDistribution.value;
  if (!action) return;
  busyID.value = action.item.id; feedback.value = "正在创建分发…"; const key = operationKey("distribution");
  await finishOperation("distribution", key, distributeNoticeVersion(action.item.id, { channel: action.channel, audience: action.audience, expected_revision: action.item.revision }, key), "分发任务已创建，将陆续推送给用户。");
  closeDistributionConfirm();
}

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh(); return; }
  snapshot.value = undefined;
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="notice-heading">
    <PageHeader eyebrow="通知流程" title="校园通知审核与分发" description="通知正文不可更改；审核与分发均由服务端记录。" title-id="notice-heading"><div class="access-context"><strong>{{ snapshotLabel }}</strong></div></PageHeader>
    <p v-if="feedback" class="mt-4 rounded-lg border border-border bg-white px-4 py-3" role="status">{{ feedback }}</p>
    <div v-if="state === 'loading'" class="mt-6 rounded-lg bg-white p-6" aria-busy="true">正在读取通知数据…</div>
    <div v-else-if="state === 'denied'" class="mt-6 rounded-lg bg-white p-6">当前账户没有通知审核权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="mt-6 rounded-lg bg-white p-6"><p>通知服务暂时不可用，请稍后重试。</p><Button class="mt-4" @click="refresh">重新加载</Button></div>
    <div v-if="state === 'ready' && canManage" class="mt-6 grid gap-4 lg:grid-cols-2">
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
    <div v-if="state === 'ready' && canDistribute" class="mt-4 flex flex-wrap gap-3 rounded-lg border border-border bg-white p-4">
      <Label class="grid gap-1">渠道<select v-model="channel" class="rounded-md border px-3 py-2"><option value="in_app">站内</option><option value="email">邮件</option></select></Label>
      <Label class="grid gap-1">受众<select v-model="audienceKind" class="rounded-md border px-3 py-2"><option value="all_students">全体学生</option><option value="college">学院</option><option value="role">角色</option></select></Label>
      <Label v-if="audienceKind !== 'all_students'" class="grid flex-1 gap-1">受众值<Input v-model="audienceValue" required maxlength="120" placeholder="例如 software-college" /></Label>
    </div>
    <div v-if="state === 'ready' && snapshot?.items.length === 0" class="mt-6 rounded-lg bg-white p-6">当前没有待处理的通知版本。</div>
    <div v-if="state === 'ready' && snapshot?.items.length" class="mt-6 grid gap-4">
      <article v-for="item in snapshot?.items" :key="item.id" class="rounded-lg border border-border bg-white p-5 ">
        <div class="flex flex-wrap items-start justify-between gap-3"><div><p class="eyebrow">{{ item.source.name }} · 版本 {{ item.version }}</p><h2 class="mt-1 text-xl font-semibold">{{ item.title }}</h2></div><span class="rounded-full bg-muted px-3 py-1 text-sm">{{ stateLabel(item.state) }} · 版本 {{ item.revision }}</span></div>
        <p class="mt-4 whitespace-pre-wrap leading-7">{{ item.body }}</p>
        <a :href="item.source_url" class="mt-3 inline-block text-sm underline" target="_blank" rel="noreferrer">核对原始来源</a>
        <div class="mt-5 flex flex-wrap gap-2">
          <template v-if="item.state === 'pending_review' && canReview">
            <Label class="grid w-full gap-1">审核理由<Textarea v-model="reviewReasons[item.id]" maxlength="1000" rows="2" class="min-h-[64px]" placeholder="驳回必须填写理由；批准可留空。" /></Label>
            <Button :disabled="busyID !== ''" @click="review(item, 'approved')">批准</Button>
            <Button variant="ghost" :disabled="busyID !== '' || !reviewReason(item)" @click="review(item, 'rejected')">拒绝</Button>
          </template>
          <Button v-if="item.state === 'approved' && canDistribute" :disabled="busyID !== '' || (audienceKind !== 'all_students' && !audienceValue)" @click="openDistributionConfirm(item)">创建分发任务</Button>
          <span v-if="item.state === 'distributed'" class="text-sm text-muted-foreground">已创建 {{ item.distribution_count }} 个分发任务 · {{ item.distribution_status ?? '状态待同步' }}</span>
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
          <div class="flex flex-wrap items-baseline justify-between gap-2"><dt class="text-muted-foreground">通知</dt><dd class="max-w-[24rem] truncate font-medium">{{ pendingDistribution.item.title }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">版本</dt><dd class="font-medium">{{ pendingDistribution.item.revision }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">渠道</dt><dd class="font-medium">{{ channelLabel(pendingDistribution.channel) }}</dd></div>
          <div class="flex items-baseline justify-between gap-2"><dt class="text-muted-foreground">受众</dt><dd class="font-medium">{{ audienceLabel(pendingDistribution.audience) }}</dd></div>
        </dl>
        <div class="mt-5 flex justify-end gap-2">
          <Button type="button" variant="ghost" :disabled="busyID !== ''" @click="closeDistributionConfirm">取消</Button>
          <Button type="submit" :disabled="busyID !== ''">确认分发</Button>
        </div>
      </form>
    </dialog>
  </section>
</template>
