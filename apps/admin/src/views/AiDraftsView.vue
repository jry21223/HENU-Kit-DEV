<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">AI Review</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <div class="stat-grid">
      <el-card v-for="item in stats" :key="item.label" shadow="never">
        <p class="muted">{{ item.label }}</p>
        <strong class="stat-number">{{ item.value }}</strong>
      </el-card>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.drafts }}</strong>
      </template>
      <el-table v-loading="loading" :data="drafts" empty-text="No AI drafts" style="width: 100%">
        <el-table-column :label="copy.draft" min-width="250">
          <template #default="{ row }">
            <strong>{{ row.outputType }}</strong>
            <p class="cell-muted">{{ row.id }}</p>
            <p class="cell-muted">{{ copy.task }}: {{ row.taskId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.content" min-width="300">
          <template #default="{ row }">
            <pre class="json-preview">{{ formatJson(row.draftContent) }}</pre>
          </template>
        </el-table-column>
        <el-table-column :label="copy.review" min-width="190">
          <template #default="{ row }">
            <span v-if="row.reviewedAt">{{ formatDate(row.reviewedAt) }}</span>
            <span v-else class="muted">{{ copy.notReviewed }}</span>
            <p v-if="row.reviewerId" class="cell-muted">{{ row.reviewerId }}</p>
            <p v-if="row.reviewReason" class="cell-muted">{{ copy.reason }}: {{ row.reviewReason }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.actions" min-width="190" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button
                size="small"
                type="success"
                plain
                :disabled="!canReview(row.status)"
                :loading="reviewingId === row.id"
                :data-testid="`ai-draft-review-approve-${row.id}`"
                @click="openReviewDialog(row, 'approve')"
              >
                {{ copy.approve }}
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="!canReview(row.status)"
                :loading="reviewingId === row.id"
                :data-testid="`ai-draft-review-reject-${row.id}`"
                @click="openReviewDialog(row, 'reject')"
              >
                {{ copy.reject }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="reviewDialogVisible" :title="reviewDialogTitle" width="min(520px, 92vw)">
      <el-form label-position="top" @submit.prevent>
        <el-form-item :label="copy.reviewReason">
          <el-input
            v-model="reviewReason"
            type="textarea"
            :rows="4"
            maxlength="1000"
            show-word-limit
            :placeholder="reviewAction === 'reject' ? copy.rejectReasonPlaceholder : copy.approveReasonPlaceholder"
          />
        </el-form-item>
        <p class="cell-muted">{{ reviewAction === "reject" ? copy.rejectReasonRequired : copy.approveReasonOptional }}</p>
      </el-form>
      <template #footer>
        <el-button @click="reviewDialogVisible = false">{{ copy.cancel }}</el-button>
        <el-button
          :type="reviewAction === 'approve' ? 'success' : 'danger'"
          :loading="Boolean(reviewingId)"
          data-testid="ai-draft-review-submit"
          @click="submitReview"
        >
          {{ reviewAction === "approve" ? copy.approve : copy.reject }}
        </el-button>
      </template>
    </el-dialog>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.tasks }}</strong>
      </template>
      <el-table v-loading="loading" :data="tasks" empty-text="No AI tasks" style="width: 100%">
        <el-table-column :label="copy.task" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.type }}</strong>
            <p class="cell-muted">{{ row.id }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="taskStatusTag(row.status)">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.owner" min-width="190">
          <template #default="{ row }">
            <span>{{ row.userId ?? copy.system }}</span>
            <p v-if="row.courseId" class="cell-muted">{{ copy.course }}: {{ row.courseId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.payload" min-width="260">
          <template #default="{ row }">
            <pre class="json-preview compact">{{ formatJson(row.result ?? row.input) }}</pre>
          </template>
        </el-table-column>
        <el-table-column prop="error" :label="copy.error" min-width="160" show-overflow-tooltip />
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.createdAt) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type AIDraft, type AITask } from "../lib/api";

const copy = {
  title: "\u0041\u0049 \u8349\u7a3f\u5ba1\u6838",
  description:
    "\u67e5\u770b worker \u4ea7\u751f\u7684 AI \u4efb\u52a1\u548c\u8349\u7a3f\u3002\u5ba1\u6838\u53ea\u6539\u53d8\u8349\u7a3f\u72b6\u6001\uff0c\u4e0d\u4f1a\u81ea\u52a8\u53d1\u5e03\u4e3a\u6b63\u5f0f\u8d44\u6599\u3001\u9898\u76ee\u6216 Wiki \u5185\u5bb9\u3002",
  refresh: "\u5237\u65b0",
  drafts: "\u5f85\u5904\u7406\u8349\u7a3f",
  draft: "\u8349\u7a3f",
  task: "\u4efb\u52a1",
  status: "\u72b6\u6001",
  content: "\u5185\u5bb9\u9884\u89c8",
  review: "\u5ba1\u6838\u8bb0\u5f55",
  reason: "\u610f\u89c1",
  actions: "\u64cd\u4f5c",
  approve: "\u901a\u8fc7",
  reject: "\u9a73\u56de",
  cancel: "\u53d6\u6d88",
  reviewReason: "\u5ba1\u6838\u610f\u89c1",
  approveReasonOptional: "\u901a\u8fc7\u53ef\u586b\u5199\u5907\u6ce8\uff0c\u4f8b\u5982\u4fdd\u7559\u9700\u540e\u7eed\u53d1\u5e03\u5230\u54ea\u7c7b\u6b63\u5f0f\u8d44\u6e90\u3002",
  rejectReasonRequired: "\u9a73\u56de\u5fc5\u987b\u5199\u660e\u539f\u56e0\uff0c\u65b9\u4fbf\u540e\u7eed\u8ffd\u6eaf\u548c\u4fee\u6539\u3002",
  approveReasonPlaceholder: "\u53ef\u9009\uff1a\u5199\u4e0b\u5ba1\u6838\u5907\u6ce8",
  rejectReasonPlaceholder: "\u5fc5\u586b\uff1a\u8bf4\u660e\u9a73\u56de\u539f\u56e0",
  reviewReasonRequired: "\u8bf7\u586b\u5199\u9a73\u56de\u539f\u56e0\u3002",
  notReviewed: "\u672a\u5ba1\u6838",
  tasks: "\u4efb\u52a1\u6d41\u6c34",
  owner: "\u6240\u6709\u8005",
  system: "\u7cfb\u7edf",
  course: "\u8bfe\u7a0b",
  payload: "\u8f93\u5165 / \u7ed3\u679c",
  error: "\u9519\u8bef",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  pendingDrafts: "\u5f85\u5ba1\u6838",
  approvedDrafts: "\u5df2\u901a\u8fc7",
  totalTasks: "\u4efb\u52a1\u603b\u6570",
  loadFailed: "\u0041\u0049 \u5ba1\u6838\u6570\u636e\u52a0\u8f7d\u5931\u8d25",
  reviewDone: "\u8349\u7a3f\u5ba1\u6838\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  reviewFailed: "\u8349\u7a3f\u5ba1\u6838\u5931\u8d25",
};

const tasks = ref<AITask[]>([]);
const drafts = ref<AIDraft[]>([]);
const loading = ref(false);
const reviewingId = ref("");
const reviewDialogVisible = ref(false);
const reviewAction = ref<"approve" | "reject">("approve");
const reviewTargetId = ref("");
const reviewReason = ref("");
const message = ref("");
const error = ref("");

const stats = computed(() => [
  { label: copy.pendingDrafts, value: drafts.value.filter((item) => canReview(item.status)).length },
  { label: copy.approvedDrafts, value: drafts.value.filter((item) => item.status === "approved").length },
  { label: copy.totalTasks, value: tasks.value.length },
]);
const reviewDialogTitle = computed(() => (reviewAction.value === "approve" ? copy.approve : copy.reject));

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [taskResponse, draftResponse] = await Promise.all([
      apiRequest<{ tasks: AITask[] }>("/admin/ai/tasks"),
      apiRequest<{ drafts: AIDraft[] }>("/admin/ai/drafts"),
    ]);
    tasks.value = taskResponse.data?.tasks ?? [];
    drafts.value = draftResponse.data?.drafts ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function openReviewDialog(row: AIDraft, action: "approve" | "reject") {
  reviewTargetId.value = row.id;
  reviewAction.value = action;
  reviewReason.value = "";
  message.value = "";
  error.value = "";
  reviewDialogVisible.value = true;
}

async function submitReview() {
  const reason = reviewReason.value.trim();
  if (reviewAction.value === "reject" && !reason) {
    error.value = copy.reviewReasonRequired;
    return;
  }
  error.value = "";
  message.value = "";
  reviewingId.value = reviewTargetId.value;
  try {
    await apiRequest<{ reviewed: boolean; status: string; reviewReason: string }>(
      `/admin/ai/drafts/${reviewTargetId.value}/${reviewAction.value}`,
      { method: "POST", body: JSON.stringify({ reviewReason: reason }) },
    );
    message.value = copy.reviewDone;
    reviewDialogVisible.value = false;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.reviewFailed;
  } finally {
    reviewingId.value = "";
  }
}

function canReview(status: string) {
  return status === "pending" || status === "draft" || status === "needs_changes";
}

function statusLabel(status: string) {
  if (status === "approved") return "\u5df2\u901a\u8fc7";
  if (status === "rejected") return "\u5df2\u9a73\u56de";
  if (status === "pending") return "\u5f85\u5ba1\u6838";
  if (status === "needs_changes") return "\u9700\u4fee\u6539";
  return status;
}

function statusTag(status: string) {
  if (status === "approved") return "success";
  if (status === "rejected") return "danger";
  if (status === "pending" || status === "needs_changes") return "warning";
  return "info";
}

function taskStatusTag(status: string) {
  if (status === "completed") return "success";
  if (status === "failed") return "danger";
  if (status === "processing" || status === "pending") return "warning";
  return "info";
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatJson(value: unknown) {
  if (value === undefined || value === null || value === "") return "-";
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}
</script>
