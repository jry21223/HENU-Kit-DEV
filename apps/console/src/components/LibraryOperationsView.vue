<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { executeLibraryCommand, fetchLibraryWorkspace, resolveLibraryOperation, type LibraryCommand, type LibraryCommandKind, type LibraryCorrection, type LibraryMaterial, type LibraryWorkspace, type LibraryWriteResult } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; permissions: string[] }>();
const workspace = ref<LibraryWorkspace>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const feedback = ref("");
const busy = ref(false);
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
type PendingOperation = { kind: LibraryCommandKind; key: string; input: LibraryCommand; success: string };
type ConfirmKind = "submission_approve" | "submission_reject" | "correction_resolve" | "correction_reject" | "course_archive" | "material_archive";
type ConfirmTarget = { kind: ConfirmKind; resourceID: string; version: string; title: string; confirmLabel: string; success: string; requireReason: boolean };
const pendingStorageKey = "henukit.library.pending-operation";
const pending = ref<PendingOperation>();
const confirmTarget = ref<ConfirmTarget>();
const confirmReason = ref("");
const libraryCommandKinds: LibraryCommandKind[] = ["course_create", "course_update", "course_archive", "material_create", "material_update", "material_archive", "submission_approve", "submission_reject", "correction_resolve", "correction_reject"];

function operationKey(kind: string) { return `idem_library_${kind}_${crypto.randomUUID()}`; }

function persistPending(value?: PendingOperation) {
  pending.value = value;
  if (value) sessionStorage.setItem(pendingStorageKey, JSON.stringify(value)); else sessionStorage.removeItem(pendingStorageKey);
}
try {
  const stored = JSON.parse(sessionStorage.getItem(pendingStorageKey) ?? "null") as Partial<PendingOperation> | null;
  if (stored && typeof stored.kind === "string" && libraryCommandKinds.includes(stored.kind as LibraryCommandKind) && typeof stored.key === "string" && typeof stored.success === "string" && !!stored.input && typeof stored.input === "object" && (stored.input as LibraryCommand).kind === stored.kind) pending.value = stored as PendingOperation;
} catch { sessionStorage.removeItem(pendingStorageKey); }

async function refresh() {
  if (props.authState !== "authenticated") { state.value = props.authState === "denied" ? "denied" : "unavailable"; return; }
  state.value = "loading";
  const result = await fetchLibraryWorkspace();
  if (result.state === "authenticated") { workspace.value = result.workspace; state.value = "ready"; return; }
  state.value = result.state === "denied" ? "denied" : "unavailable";
}

function isConfirming(id: string, ...kinds: LibraryCommandKind[]): boolean {
  const target = confirmTarget.value;
  return !!target && target.resourceID === id && kinds.includes(target.kind);
}

async function finish(kind: LibraryCommandKind, key: string, input: LibraryCommand, success: string) {
  busy.value = true;
  let result = await executeLibraryCommand(input, key);
  if (result.state === "unknown") {
    feedback.value = "操作结果待确认，正在核对…";
    persistPending({ kind, key, input, success });
    result = await resolveLibraryOperation(kind, key);
  }
  if (result.state === "succeeded") { persistPending(); feedback.value = success; await refresh(); }
  else if (result.state === "conflict") { persistPending(); feedback.value = "资料版本已变化，请刷新后重试。"; }
  else if (result.state === "denied") { persistPending(); feedback.value = "当前账户缺少资料库操作权限。"; }
  else if (result.state === "invalid") { persistPending(); feedback.value = "该操作暂不支持，请检查内容后重试。"; }
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

function openSubmissionConfirm(item: LibraryMaterial, decision: "approve" | "reject") {
  confirmTarget.value = {
    kind: decision === "approve" ? "submission_approve" : "submission_reject",
    resourceID: item.id,
    version: item.updated_at,
    title: decision === "approve" ? "批准投稿" : "拒绝投稿",
    confirmLabel: decision === "approve" ? "确认批准" : "确认拒绝",
    success: decision === "approve" ? "投稿已批准。" : "投稿已拒绝。",
    requireReason: decision === "reject",
  };
  confirmReason.value = "";
}

function openCorrectionConfirm(item: LibraryCorrection, decision: "resolve" | "reject") {
  confirmTarget.value = {
    kind: decision === "resolve" ? "correction_resolve" : "correction_reject",
    resourceID: item.id,
    version: item.updated_at,
    title: decision === "resolve" ? "标记已处理" : "驳回纠错",
    confirmLabel: decision === "resolve" ? "确认处理" : "确认驳回",
    success: decision === "resolve" ? "纠错已处理。" : "纠错已驳回。",
    requireReason: decision === "reject",
  };
  confirmReason.value = "";
}

function openArchiveConfirm(kind: "course_archive" | "material_archive", resourceID: string, version: string) {
  confirmTarget.value = {
    kind,
    resourceID,
    version,
    title: kind === "course_archive" ? "归档课程" : "归档资料",
    confirmLabel: "确认归档",
    success: kind === "course_archive" ? "课程已归档。" : "资料已归档。",
    requireReason: false,
  };
  confirmReason.value = "";
}

function submitConfirm() {
  const target = confirmTarget.value;
  if (!target || busy.value) return;
  const reason = confirmReason.value.trim();
  if (target.requireReason && !reason) { feedback.value = "拒绝类操作必须填写本次理由。"; return; }
  if (reason.length > 1000) { feedback.value = "理由不能超过 1000 字。"; return; }
  const key = operationKey(target.kind);
  const kind = target.kind;
  if (kind === "course_archive" || kind === "material_archive") {
    void finish(kind, key, { kind, resource_id: target.resourceID, expected_version: target.version, payload: {} }, target.success);
    return;
  }
  void finish(kind, key, { kind, resource_id: target.resourceID, expected_version: target.version, payload: reason ? { reviewReason: reason } : {} }, target.success);
}

function courseStatusLabel(status: string) {
  return status === "draft" ? "草稿" : status === "published" ? "已发布" : status === "archived" ? "已归档" : `未知状态（${status}）`;
}

function materialStatusLabel(status: LibraryMaterial["status"]) {
  return status === "draft" ? "草稿" : status === "pending" ? "待审核" : status === "published" ? "已发布" : status === "rejected" ? "已拒绝" : status === "archived" ? "已归档" : `未知状态（${status}）`;
}

function materialTypeLabel(type: LibraryMaterial["type"]) {
  return type === "knowledge_note" ? "知识笔记" : type === "mock_paper" ? "模拟试卷" : type === "answer" ? "答案解析" : type === "quick_review" ? "快速复习" : type === "past_exam" ? "往年真题" : type === "other" ? "其他" : `未知类型（${type}）`;
}

function accessLevelLabel(level: LibraryMaterial["access_level"]) {
  return level === "public" ? "公开" : level === "authenticated" ? "登录可见" : level === "restricted" ? "受限" : `未知权限（${level}）`;
}

function workspaceStatusLabel(status: string) {
  if (status === "ok") return "正常";
  if (status === "partial") return "部分可用";
  if (status === "unavailable") return "暂不可用";
  if (status === "loading") return "加载中";
  return `未知状态（${status}）`;
}

function createCourse() {
  if (busy.value) return;
  if (!courseName.value.trim() || !courseSlug.value.trim() || !courseGrade.value.trim() || !courseSchoolID.value || !courseCollegeID.value || !courseMajorID.value) { feedback.value = "请填写课程归属、年级、名称与课程标识。"; return; }
  const key = operationKey("course_create");
  const payload = { schoolId: courseSchoolID.value, collegeId: courseCollegeID.value, majorId: courseMajorID.value, name: courseName.value.trim(), slug: courseSlug.value.trim(), grade: courseGrade.value.trim(), status: "draft" as const };
  void finish("course_create", key, { kind: "course_create", payload }, "课程已创建。");
}

function updateCourse(item: LibraryWorkspace["courses"][number]) {
  if (busy.value) return;
  const name = (courseEdits.value[item.id] ?? item.name).trim();
  if (!name) { feedback.value = "课程名称不能为空。"; return; }
  const key = operationKey("course_update");
  void finish("course_update", key, { kind: "course_update", resource_id: item.id, expected_version: item.updated_at, payload: { name } }, "课程已更新。");
}

function createMaterial() {
  if (busy.value) return;
  if (!materialCourseID.value || !materialTitle.value.trim() || !materialStorageKey.value.trim() || !materialFileName.value.trim()) { feedback.value = "请选择课程并填写资料标题、文件标识与文件名。"; return; }
  const key = operationKey("material_create");
  const payload = { courseId: materialCourseID.value, title: materialTitle.value.trim(), type: "other" as const, storageKey: materialStorageKey.value.trim(), fileName: materialFileName.value.trim(), fileSize: 0, accessLevel: "login_required" as const, status: "draft" as const };
  void finish("material_create", key, { kind: "material_create", payload }, "资料已创建。");
}

function updateMaterial(item: LibraryMaterial) {
  if (busy.value) return;
  const title = (materialEdits.value[item.id] ?? item.title).trim();
  if (!title) { feedback.value = "资料标题不能为空。"; return; }
  const key = operationKey("material_update");
  void finish("material_update", key, { kind: "material_update", resource_id: item.id, expected_version: item.updated_at, payload: { title } }, "资料已更新。");
}

watch(() => props.authState, (value) => {
  if (value === "authenticated") { void refresh().then(() => { if (pending.value) feedback.value = "发现一项结果未确认的操作；可按原请求重试。"; }); return; }
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
      <p v-if="feedback" class="operation-notice mt-5" role="status">
        {{ feedback }}
        <Button v-if="pending && !busy" class="mt-3" @click="retryPending">按原请求重试</Button>
      </p>

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
          <form v-if="canManage" class="mt-3 grid gap-2 rounded-lg border p-3" @submit.prevent="createCourse">
            <strong>创建课程</strong>
            <Label class="grid gap-1">学校标识<Input v-model="courseSchoolID" required type="text" /></Label>
            <Label class="grid gap-1">学院标识<Input v-model="courseCollegeID" required type="text" /></Label>
            <Label class="grid gap-1">专业标识<Input v-model="courseMajorID" required type="text" /></Label>
            <Label class="grid gap-1">课程名称<Input v-model="courseName" required maxlength="160" /></Label>
            <Label class="grid gap-1">课程标识<Input v-model="courseSlug" required maxlength="160" /></Label>
            <Label class="grid gap-1">年级<Input v-model="courseGrade" maxlength="32" /></Label>
            <Button type="submit" :disabled="busy">创建课程</Button>
          </form>
          <p v-if="!workspace.courses.length" class="mt-3 text-muted-foreground">暂无课程。</p>
          <ul class="mt-3 grid gap-3"><li v-for="item in workspace.courses" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.name }}</strong><p class="text-sm text-muted-foreground">{{ item.grade }} · {{ courseStatusLabel(item.status) }}</p><div v-if="canManage && item.status !== 'archived'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'course_archive')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><div class="flex flex-wrap gap-2"><Button :disabled="busy" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><template v-else><label class="grid gap-1 text-sm">编辑课程名称<input v-model="courseEdits[item.id]" :placeholder="item.name" maxlength="160" class="rounded-md border px-3 py-2"></label><div class="flex flex-wrap gap-2"><Button :disabled="busy" @click="updateCourse(item)">保存课程</Button><Button variant="ghost" :disabled="busy" @click="openArchiveConfirm('course_archive', item.id, item.updated_at)">归档课程</Button></div></template></div></li></ul>
        </Card>
        <Card class="p-4" aria-labelledby="library-materials">
          <h2 id="library-materials" class="text-lg font-bold">资料</h2>
          <form v-if="canManage" class="mt-3 grid gap-2 rounded-lg border p-3" @submit.prevent="createMaterial">
            <strong>创建资料</strong>
            <Label class="grid gap-1">所属课程<select v-model="materialCourseID" required class="rounded-md border px-3 py-2"><option value="" disabled>请选择课程</option><option v-for="course in workspace.courses" :key="course.id" :value="course.id">{{ course.name }}</option></select></Label>
            <Label class="grid gap-1">资料标题<Input v-model="materialTitle" required maxlength="200" /></Label>
            <Label class="grid gap-1">文件标识<Input v-model="materialStorageKey" required maxlength="512" /></Label>
            <Label class="grid gap-1">文件名<Input v-model="materialFileName" required maxlength="255" /></Label>
            <Button type="submit" :disabled="busy">创建资料</Button>
          </form>
          <p v-if="!workspace.materials.length" class="mt-3 text-muted-foreground">暂无资料。</p>
          <ul class="mt-3 grid gap-3"><li v-for="item in workspace.materials" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.title }}</strong><p class="text-sm text-muted-foreground">{{ materialTypeLabel(item.type) }} · {{ materialStatusLabel(item.status) }} · {{ item.file_name }}</p><div v-if="canManage && item.status !== 'archived'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'material_archive')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><div class="flex flex-wrap gap-2"><Button :disabled="busy" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><template v-else><label class="grid gap-1 text-sm">编辑资料标题<input v-model="materialEdits[item.id]" :placeholder="item.title" maxlength="200" class="rounded-md border px-3 py-2"></label><div class="flex flex-wrap gap-2"><Button :disabled="busy" @click="updateMaterial(item)">保存资料</Button><Button variant="ghost" :disabled="busy" @click="openArchiveConfirm('material_archive', item.id, item.updated_at)">归档资料</Button></div></template></div></li></ul>
        </Card>
        <Card class="p-4" aria-labelledby="library-downloads"><h2 id="library-downloads" class="text-lg font-bold">下载</h2><p v-if="!workspace.downloads.length" class="mt-3 text-muted-foreground">暂无下载记录。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.downloads" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.material_title }}</strong><p class="text-sm text-muted-foreground">{{ accessLevelLabel(item.access_level) }} · {{ new Date(item.downloaded_at).toLocaleString('zh-CN') }}</p></li></ul></Card>
        <Card class="p-4" aria-labelledby="library-submissions"><h2 id="library-submissions" class="text-lg font-bold">投稿审核</h2><p v-if="!workspace.submissions.length" class="mt-3 text-muted-foreground">暂无待审核投稿。</p><ul class="mt-3 grid gap-3"><li v-for="item in workspace.submissions" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.title }}</strong><p class="text-sm text-muted-foreground">{{ materialTypeLabel(item.type) }} · {{ item.file_name }}</p><div v-if="canReview" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'submission_approve', 'submission_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">审核理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" :placeholder="confirmTarget?.requireReason ? '请填写本次拒绝理由' : '可填写本次审核意见（选填）'"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || (confirmTarget?.requireReason === true && !confirmReason.trim())" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex flex-wrap gap-2"><Button :disabled="busy" @click="openSubmissionConfirm(item, 'approve')">批准投稿</Button><Button variant="ghost" :disabled="busy" @click="openSubmissionConfirm(item, 'reject')">拒绝投稿</Button></div></div><p v-else class="mt-3 text-sm">只读权限</p></li></ul></Card>
        <Card class="p-4 xl:col-span-2" aria-labelledby="library-corrections"><h2 id="library-corrections" class="text-lg font-bold">资料纠错</h2><p v-if="!workspace.corrections.length" class="mt-3 text-muted-foreground">暂无待处理纠错。</p><ul class="mt-3 grid gap-3 md:grid-cols-2"><li v-for="item in workspace.corrections" :key="item.id" class="rounded-lg border p-3"><strong>{{ item.reason }}</strong><p class="mt-1 text-sm text-muted-foreground">{{ item.description }}</p><div v-if="canReview && item.status === 'pending'" class="mt-3 grid gap-2"><template v-if="isConfirming(item.id, 'correction_resolve', 'correction_reject')"><div class="grid gap-2 rounded-lg border p-3"><strong>{{ confirmTarget?.title }}</strong><label class="grid gap-1 text-sm">处理理由<Textarea v-model="confirmReason" maxlength="1000" rows="3" :placeholder="confirmTarget?.requireReason ? '请填写本次驳回理由' : '可填写本次处理意见（选填）'"></Textarea></label><div class="flex flex-wrap gap-2"><Button :disabled="busy || (confirmTarget?.requireReason === true && !confirmReason.trim())" @click="submitConfirm">{{ confirmTarget?.confirmLabel }}</Button><Button variant="ghost" :disabled="busy" @click="cancelConfirm">取消</Button></div></div></template><div v-else class="flex gap-2"><Button :disabled="busy" @click="openCorrectionConfirm(item, 'resolve')">标记已处理</Button><Button variant="ghost" :disabled="busy" @click="openCorrectionConfirm(item, 'reject')">驳回纠错</Button></div></div></li></ul></Card>
      </div>
    </template>
  </section>
</template>
