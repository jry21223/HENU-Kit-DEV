<script setup lang="ts">
import { computed, ref, watch } from "vue";

import UiButton from "@/components/ui/UiButton.vue";
import { createNoticeSource, createNoticeVersion, distributeNoticeVersion, fetchNoticeSnapshot, resolveNoticeOperation, reviewNoticeVersion, type NoticeSnapshot, type NoticeVersion, type NoticeWriteResult } from "@/lib/console-gateway";

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
const canManage = computed(() => props.permissions.includes("notice.manage"));
const canReview = computed(() => props.permissions.includes("notice.review"));
const canDistribute = computed(() => props.permissions.includes("notice.distribute"));

function operationKey(kind: string) { return `idem_notice_${kind}_${crypto.randomUUID()}`; }

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const result = await fetchNoticeSnapshot();
  if (result.state === "authenticated") { snapshot.value = result.snapshot; state.value = "ready"; }
  else state.value = result.state === "denied" ? "denied" : "unavailable";
}

async function finishOperation(kind: "source_create" | "version_create" | "review" | "distribution", key: string, initial: Promise<NoticeWriteResult>, successMessage: string, onSuccess?: (result: Record<string, unknown>) => void) {
  let result = await initial;
  if (result.state === "unknown") { feedback.value = "操作结果未知，正在按幂等键核对…"; result = await resolveNoticeOperation(kind, key); }
  if (result.state === "succeeded") { onSuccess?.(result.result); feedback.value = successMessage; await refresh(); }
  else if (result.state === "conflict") { feedback.value = "版本状态或幂等历史已变化，已刷新当前事实。"; await refresh(); }
  else feedback.value = "操作未完成，请根据当前状态重试。";
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
  busyID.value = "version"; feedback.value = "正在创建不可变通知版本…"; const key = operationKey("version_create");
  const published = versionPublishedAt.value ? new Date(versionPublishedAt.value).toISOString() : undefined;
  await finishOperation("version_create", key, createNoticeVersion(versionSourceID.value, { title: versionTitle.value, body: versionBody.value, source_url: versionSourceURL.value, ...(published ? { source_published_at: published } : {}) }, key), "不可变通知版本已创建并进入待审核状态。", () => {
    versionTitle.value = ""; versionBody.value = ""; versionSourceURL.value = ""; versionPublishedAt.value = "";
  });
}

async function review(item: NoticeVersion, decision: "approved" | "rejected") {
  busyID.value = item.id; feedback.value = "正在提交审核…"; const key = operationKey("review");
  await finishOperation("review", key, reviewNoticeVersion(item.id, { decision, note: "Console 人工审核", expected_revision: item.revision }, key), decision === "approved" ? "审核已批准。" : "审核已拒绝。");
}

async function distribute(item: NoticeVersion) {
  busyID.value = item.id; feedback.value = "正在创建分发…"; const key = operationKey("distribution");
  const audience = audienceKind.value === "all_students" ? { kind: "all_students" as const } : { kind: audienceKind.value, value: audienceValue.value };
  await finishOperation("distribution", key, distributeNoticeVersion(item.id, { channel: channel.value, audience, expected_revision: item.revision }, key), "分发任务已创建，Worker 将同步交付状态。");
}

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh(); return; }
  snapshot.value = undefined;
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="notice-heading">
    <div class="overview-hero">
      <div><p class="eyebrow">Notice lifecycle</p><h1 id="notice-heading" class="mt-2 text-2xl font-bold sm:text-3xl">校园通知审核与分发</h1><p class="mt-2 max-w-2xl leading-7 text-[var(--hk-ink-muted)]">正文版本由 Notice 服务保持不可变；审核与分发使用乐观 revision、幂等键和服务端产品 Scope。</p></div>
      <div class="access-context"><span>scope:product/notice</span><strong>{{ snapshot?.items.length ?? 0 }} 个版本</strong></div>
    </div>
    <p v-if="feedback" class="mt-4 rounded-[4px] border border-[var(--hk-line)] bg-white px-4 py-3" role="status">{{ feedback }}</p>
    <div v-if="state === 'loading'" class="mt-6 rounded-[4px] bg-white p-6" aria-busy="true">正在读取 Notice 生命周期…</div>
    <div v-else-if="state === 'denied'" class="mt-6 rounded-[4px] bg-white p-6">当前账户缺少 Notice 产品 Scope 或读取权限。</div>
    <div v-else-if="state === 'unavailable'" class="mt-6 rounded-[4px] bg-white p-6"><p>Notice 服务暂不可用。</p><UiButton class="mt-4" @click="refresh">重新加载</UiButton></div>
    <div v-if="state === 'ready' && canManage" class="mt-6 grid gap-4 lg:grid-cols-2">
      <form class="rounded-[4px] border border-[var(--hk-line)] bg-white p-5" @submit.prevent="addSource">
        <h2 class="text-lg font-semibold">登记通知来源</h2>
        <div class="mt-4 grid gap-3">
          <label class="grid gap-1 text-sm">来源代码<input v-model="sourceCode" required pattern="[a-z0-9][a-z0-9-]{1,62}" class="rounded-[4px] border px-3 py-2" placeholder="henu-office"></label>
          <label class="grid gap-1 text-sm">来源名称<input v-model="sourceName" required maxlength="120" class="rounded-[4px] border px-3 py-2" placeholder="学校办公室"></label>
          <label class="grid gap-1 text-sm">来源主页<input v-model="sourceCanonicalURL" required type="url" pattern="https://.*" class="rounded-[4px] border px-3 py-2" placeholder="https://example.edu/notices"></label>
        </div>
        <UiButton class="mt-4" type="submit" :disabled="busyID !== ''">创建来源</UiButton>
      </form>
      <form class="rounded-[4px] border border-[var(--hk-line)] bg-white p-5" @submit.prevent="addVersion">
        <h2 class="text-lg font-semibold">创建不可变版本</h2>
        <div class="mt-4 grid gap-3">
          <label class="grid gap-1 text-sm">来源 ID<input v-model="versionSourceID" required class="rounded-[4px] border px-3 py-2" placeholder="UUID"></label>
          <label class="grid gap-1 text-sm">标题<input v-model="versionTitle" required maxlength="200" class="rounded-[4px] border px-3 py-2"></label>
          <label class="grid gap-1 text-sm">正文<textarea v-model="versionBody" required maxlength="100000" rows="4" class="rounded-[4px] border px-3 py-2"></textarea></label>
          <label class="grid gap-1 text-sm">原文链接<input v-model="versionSourceURL" required type="url" pattern="https://.*" class="rounded-[4px] border px-3 py-2"></label>
          <label class="grid gap-1 text-sm">原文发布时间（可选）<input v-model="versionPublishedAt" type="datetime-local" class="rounded-[4px] border px-3 py-2"></label>
        </div>
        <UiButton class="mt-4" type="submit" :disabled="busyID !== ''">创建版本</UiButton>
      </form>
    </div>
    <div v-if="state === 'ready' && canDistribute" class="mt-4 flex flex-wrap gap-3 rounded-[4px] border border-[var(--hk-line)] bg-white p-4">
      <label class="grid gap-1 text-sm">渠道<select v-model="channel" class="rounded-[4px] border px-3 py-2"><option value="in_app">站内</option><option value="email">邮件</option></select></label>
      <label class="grid gap-1 text-sm">受众<select v-model="audienceKind" class="rounded-[4px] border px-3 py-2"><option value="all_students">全体学生</option><option value="college">学院</option><option value="role">角色</option></select></label>
      <label v-if="audienceKind !== 'all_students'" class="grid flex-1 gap-1 text-sm">受众值<input v-model="audienceValue" required maxlength="120" class="rounded-[4px] border px-3 py-2" placeholder="例如 software-college"></label>
    </div>
    <div v-if="state === 'ready' && snapshot?.items.length === 0" class="mt-6 rounded-[4px] bg-white p-6">当前没有待处理的通知版本。</div>
    <div v-if="state === 'ready' && snapshot?.items.length" class="mt-6 grid gap-4">
      <article v-for="item in snapshot?.items" :key="item.id" class="rounded-[4px] border border-[var(--hk-line)] bg-white p-5 shadow-[var(--hk-shadow-card)]">
        <div class="flex flex-wrap items-start justify-between gap-3"><div><p class="eyebrow">{{ item.source.name }} · v{{ item.version }}</p><h2 class="mt-1 text-xl font-semibold">{{ item.title }}</h2></div><span class="rounded-full bg-[var(--hk-paper)] px-3 py-1 text-sm">{{ item.state }} · r{{ item.revision }}</span></div>
        <p class="mt-4 whitespace-pre-wrap leading-7">{{ item.body }}</p>
        <a :href="item.source_url" class="mt-3 inline-block text-sm underline" target="_blank" rel="noreferrer">核对原始来源</a>
        <div class="mt-5 flex flex-wrap gap-2">
          <template v-if="item.state === 'pending_review' && canReview"><UiButton :disabled="busyID !== ''" @click="review(item, 'approved')">批准</UiButton><UiButton variant="ghost" :disabled="busyID !== ''" @click="review(item, 'rejected')">拒绝</UiButton></template>
          <UiButton v-if="item.state === 'approved' && canDistribute" :disabled="busyID !== '' || (audienceKind !== 'all_students' && !audienceValue)" @click="distribute(item)">创建分发任务</UiButton>
          <span v-if="item.state === 'distributed'" class="text-sm text-[var(--hk-ink-muted)]">已创建 {{ item.distribution_count }} 个分发任务 · {{ item.distribution_status ?? '状态待同步' }}</span>
        </div>
      </article>
    </div>
  </section>
</template>
