<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader } from "@/components/ui";
import { executeLibraryCommand, fetchLibraryWorkspace, resolveLibraryOperation, type LibraryCommandKind, type LibraryCorrection, type LibraryMaterial, type LibraryWorkspace, type LibraryWriteResult } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const workspace = ref<LibraryWorkspace>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const feedback = ref("");
const canManage = computed(() => props.permissions.includes("library.manage"));
const canReview = computed(() => props.permissions.includes("library.review"));
const courseName = ref("");
const courseSlug = ref("");
const courseGrade = ref("");
const courseSchoolID = ref("");
const courseCollegeID = ref("");
const courseMajorID = ref("");
const courseEdits = ref<Record<string, string>>({});
const materialCourseID = ref("");
const materialTitle = ref("");
const materialFileName = ref("");
const materialStorageKey = ref("");
const materialEdits = ref<Record<string, string>>({});

function operationKey(kind: string) { return `idem_library_${kind}_${crypto.randomUUID()}`; }

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const result = await fetchLibraryWorkspace();
  if (result.state === "authenticated") { workspace.value = result.workspace; state.value = "ready"; return; }
  state.value = result.state === "denied" ? "denied" : "unavailable";
}

async function finish(kind: LibraryCommandKind, key: string, initial: Promise<LibraryWriteResult>, success: string) {
  let result = await initial;
  if (result.state === "unknown") {
    feedback.value = "操作结果待确认，正在核对…";
    result = await resolveLibraryOperation(kind, key);
  }
  if (result.state === "succeeded") { feedback.value = success; await refresh(); return; }
  feedback.value = result.state === "conflict" ? "资料版本已变化，请刷新后重试。" : result.state === "denied" ? "当前账户缺少资料库操作权限。" : result.state === "invalid" ? "该操作暂不支持，请检查内容后重试。" : "操作没有完成，请稍后刷新页面重试。";
}

function courseStatusLabel(status: string) {
  return status === "draft" ? "草稿" : status === "published" ? "已发布" : status === "archived" ? "已归档" : status;
}

function materialStatusLabel(status: LibraryMaterial["status"]) {
  return status === "draft" ? "草稿" : status === "pending" ? "待审核" : status === "published" ? "已发布" : status === "rejected" ? "已拒绝" : status === "archived" ? "已归档" : status;
}

function materialTypeLabel(type: LibraryMaterial["type"]) {
  return type === "knowledge_note" ? "知识笔记" : type === "mock_paper" ? "模拟试卷" : type === "answer" ? "答案解析" : type === "quick_review" ? "快速复习" : type === "past_exam" ? "往年真题" : type === "other" ? "其他" : type;
}

function accessLevelLabel(level: LibraryMaterial["access_level"]) {
  return level === "public" ? "公开" : level === "authenticated" ? "登录可见" : level === "restricted" ? "受限" : level;
}

function workspaceStatusLabel(status: string) {
  if (status === "ok") return "正常";
  if (status === "partial") return "部分可用";
  if (status === "unavailable") return "暂不可用";
  if (status === "loading") return "加载中";
  return status;
}

function reviewSubmission(item: LibraryMaterial, decision: "approve" | "reject") {
  const kind: LibraryCommandKind = decision === "approve" ? "submission_approve" : "submission_reject";
  const key = operationKey(kind);
  return finish(kind, key, executeLibraryCommand({ kind, resource_id: item.id, expected_version: item.updated_at, payload: { reviewReason: decision === "approve" ? "Console 人工核验" : "资料不符合发布要求" } }, key), decision === "approve" ? "投稿已批准。" : "投稿已拒绝。");
}

function reviewCorrection(item: LibraryCorrection, decision: "resolve" | "reject") {
  const kind: LibraryCommandKind = decision === "resolve" ? "correction_resolve" : "correction_reject";
  const key = operationKey(kind);
  return finish(kind, key, executeLibraryCommand({ kind, resource_id: item.id, expected_version: item.updated_at, payload: { reviewReason: decision === "resolve" ? "纠错已处理" : "纠错不成立" } }, key), decision === "resolve" ? "纠错已处理。" : "纠错已驳回。");
}

function archive(kind: "course_archive" | "material_archive", id: string, version: string) {
  const key = operationKey(kind);
  return finish(kind, key, executeLibraryCommand({ kind, resource_id: id, expected_version: version, payload: {} }, key), kind === "course_archive" ? "课程已归档。" : "资料已归档。");
}

function createCourse() {
  if (!courseName.value.trim() || !courseSlug.value.trim() || !courseGrade.value.trim() || !courseSchoolID.value || !courseCollegeID.value || !courseMajorID.value) { feedback.value = "请填写课程归属、年级、名称与课程标识。"; return; }
  const key = operationKey("course_create");
  const payload = { schoolId: courseSchoolID.value, collegeId: courseCollegeID.value, majorId: courseMajorID.value, name: courseName.value.trim(), slug: courseSlug.value.trim(), grade: courseGrade.value.trim(), status: "draft" as const };
  return finish("course_create", key, executeLibraryCommand({ kind: "course_create", payload }, key), "课程已创建。");
}

function updateCourse(item: LibraryWorkspace["courses"][number]) {
  const name = (courseEdits.value[item.id] ?? item.name).trim();
  if (!name) { feedback.value = "课程名称不能为空。"; return; }
  const key = operationKey("course_update");
  return finish("course_update", key, executeLibraryCommand({ kind: "course_update", resource_id: item.id, expected_version: item.updated_at, payload: { name } }, key), "课程已更新。");
}

function createMaterial() {
  if (!materialCourseID.value || !materialTitle.value.trim() || !materialStorageKey.value.trim() || !materialFileName.value.trim()) { feedback.value = "请选择课程并填写资料标题、文件标识与文件名。"; return; }
  const key = operationKey("material_create");
  const payload = { courseId: materialCourseID.value, title: materialTitle.value.trim(), type: "other" as const, storageKey: materialStorageKey.value.trim(), fileName: materialFileName.value.trim(), fileSize: 0, accessLevel: "login_required" as const, status: "draft" as const };
  return finish("material_create", key, executeLibraryCommand({ kind: "material_create", payload }, key), "资料已创建。");
}

function updateMaterial(item: LibraryMaterial) {
  const title = (materialEdits.value[item.id] ?? item.title).trim();
  if (!title) { feedback.value = "资料标题不能为空。"; return; }
  const key = operationKey("material_update");
  return finish("material_update", key, executeLibraryCommand({ kind: "material_update", resource_id: item.id, expected_version: item.updated_at, payload: { title } }, key), "资料已更新。");
}

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh(); return; }
  workspace.value = undefined;
  state.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="library-heading">
    <PageHeader eyebrow="兼容接入" title="资料库运营" title-id="library-heading" description="资料库内容由旧系统兼容接入，此处操作会同步生效。">
      <div class="access-context"><strong>{{ workspaceStatusLabel(workspace?.status ?? "loading") }}</strong></div>
    </PageHeader>

    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取资料库数据…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前账户没有资料库操作权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state"><p>资料库服务暂时不可用，请稍后重试。</p><Button class="mt-3" @click="refresh">重新加载</Button></div>
    <template v-else-if="workspace">
      <div v-if="workspace.degraded" class="operation-notice mt-5" role="status"><strong>数据不完整，仅展示部分内容：</strong><p>{{ workspace.status_message }}</p></div>
      <p v-if="feedback" class="operation-notice mt-5" role="status">{{ feedback }}</p>

      <div class="operation-summary-grid mt-6">
        <article><span>课程</span><strong>{{ workspace.courses.length }}</strong></article>
        <article><span>资料</span><strong>{{ workspace.materials.length }}</strong></article>
        <article><span>下载</span><strong>{{ workspace.downloads.length }}</strong></article>
        <article><span>投稿审核</span><strong>{{ workspace.submissions.length }}</strong></article>
        <article><span>资料纠错</span><strong>{{ workspace.corrections.length }}</strong></article>
      </div>

      <div class="mt-6 grid gap-5 xl:grid-cols-2">
        <Card class="p-4" aria-labelledby="library-courses">
          <h2 id="library-courses" class="text-lg font-bold">课程</h2>
          <form v-if="canManage" class="mt-3 grid gap-2 rounded-[var(--hk-radius-card)] border p-3" @submit.prevent="createCourse">
            <strong>创建课程</strong>
            <Label class="grid gap-1">学校标识<Input v-model="courseSchoolID" required type="text" /></Label>
            <Label class="grid gap-1">学院标识<Input v-model="courseCollegeID" required type="text" /></Label>
            <Label class="grid gap-1">专业标识<Input v-model="courseMajorID" required type="text" /></Label>
            <Label class="grid gap-1">课程名称<Input v-model="courseName" required maxlength="160" /></Label>
            <Label class="grid gap-1">课程标识<Input v-model="courseSlug" required maxlength="160" /></Label>
            <Label class="grid gap-1">年级<Input v-model="courseGrade" maxlength="32" /></Label>
            <Button type="submit">创建课程</Button>
          </form>
          <p v-if="!workspace.courses.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无课程。</p>
          <ul class="mt-3 grid gap-3"><li v-for="item in workspace.courses" :key="item.id" class="rounded-[var(--hk-radius-card)] border p-3"><strong>{{ item.name }}</strong><p class="text-sm text-[var(--hk-ink-muted)]">{{ item.grade }} · {{ courseStatusLabel(item.status) }}</p><div v-if="canManage && item.status !== 'archived'" class="mt-3 grid gap-2"><label class="grid gap-1 text-sm">编辑课程名称<input v-model="courseEdits[item.id]" :placeholder="item.name" maxlength="160" class="rounded-[var(--hk-radius-control)] border px-3 py-2"></label><div class="flex flex-wrap gap-2"><Button @click="updateCourse(item)">保存课程</Button><Button variant="ghost" @click="archive('course_archive', item.id, item.updated_at)">归档课程</Button></div></div></li></ul>
        </Card>
        <Card class="p-4" aria-labelledby="library-materials">
          <h2 id="library-materials" class="text-lg font-bold">资料</h2>
          <form v-if="canManage" class="mt-3 grid gap-2 rounded-[var(--hk-radius-card)] border p-3" @submit.prevent="createMaterial">
            <strong>创建资料</strong>
            <Label class="grid gap-1">所属课程<select v-model="materialCourseID" required class="rounded-[var(--hk-radius-control)] border px-3 py-2"><option value="" disabled>请选择课程</option><option v-for="course in workspace.courses" :key="course.id" :value="course.id">{{ course.name }}</option></select></Label>
            <Label class="grid gap-1">资料标题<Input v-model="materialTitle" required maxlength="200" /></Label>
            <Label class="grid gap-1">文件标识<Input v-model="materialStorageKey" required maxlength="512" /></Label>
            <Label class="grid gap-1">文件名<Input v-model="materialFileName" required maxlength="255" /></Label>
            <Button type="submit">创建资料</Button>
          </form>
          <p v-if="!workspace.materials.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无资料。</p>
          <ul class="mt-3 grid gap-3"><li v-for="item in workspace.materials" :key="item.id" class="rounded-[var(--hk-radius-card)] border p-3"><strong>{{ item.title }}</strong><p class="text-sm text-[var(--hk-ink-muted)]">{{ materialTypeLabel(item.type) }} · {{ materialStatusLabel(item.status) }} · {{ item.file_name }}</p><div v-if="canManage && item.status !== 'archived'" class="mt-3 grid gap-2"><label class="grid gap-1 text-sm">编辑资料标题<input v-model="materialEdits[item.id]" :placeholder="item.title" maxlength="200" class="rounded-[var(--hk-radius-control)] border px-3 py-2"></label><div class="flex flex-wrap gap-2"><Button @click="updateMaterial(item)">保存资料</Button><Button variant="ghost" @click="archive('material_archive', item.id, item.updated_at)">归档资料</Button></div></div></li></ul>
        </Card>
        <Card class="p-4" aria-labelledby="library-downloads"><h2 id="library-downloads" class="text-lg font-bold">下载</h2><p v-if="!workspace.downloads.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无下载记录。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.downloads" :key="item.id" class="rounded-[var(--hk-radius-card)] border p-3"><strong>{{ item.material_title }}</strong><p class="text-sm text-[var(--hk-ink-muted)]">{{ accessLevelLabel(item.access_level) }} · {{ new Date(item.downloaded_at).toLocaleString('zh-CN') }}</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="library-submissions"><h2 id="library-submissions" class="text-lg font-bold">投稿审核</h2><p v-if="!workspace.submissions.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无待审核投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.submissions" :key="item.id" class="rounded-[var(--hk-radius-card)] border p-3"><strong>{{ item.title }}</strong><p class="text-sm text-[var(--hk-ink-muted)]">{{ item.type }} · {{ item.file_name }}</p><div v-if="canReview" class="mt-3 flex flex-wrap gap-2"><Button @click="reviewSubmission(item, 'approve')">批准投稿</Button><Button variant="ghost" @click="reviewSubmission(item, 'reject')">拒绝投稿</Button></div><p v-else class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4 xl:col-span-2" aria-labelledby="library-corrections"><h2 id="library-corrections" class="text-lg font-bold">资料纠错</h2><p v-if="!workspace.corrections.length" class="mt-3 text-[var(--hk-ink-muted)]">暂无待处理纠错。</p><ul class="mt-3 grid gap-3 md:grid-cols-2"><li v-for="item in workspace.corrections" :key="item.id" class="rounded-[var(--hk-radius-card)] border p-3"><strong>{{ item.reason }}</strong><p class="mt-1 text-sm text-[var(--hk-ink-muted)]">{{ item.description }}</p><div v-if="canReview && item.status === 'pending'" class="mt-3 flex gap-2"><Button @click="reviewCorrection(item, 'resolve')">标记已处理</Button><Button variant="ghost" @click="reviewCorrection(item, 'reject')">驳回纠错</Button></div></li></ul></Card>
      </div>
    </template>
  </section>
</template>
