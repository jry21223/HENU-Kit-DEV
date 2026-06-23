<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Forum Reply Review</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <div class="action-row">
        <el-select v-model="statusFilter" class="status-filter" @change="loadReplies">
          <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="loadReplies">{{ copy.refresh }}</el-button>
      </div>
    </div>

    <section class="metric-grid">
      <el-card v-for="item in metrics" :key="item.label" shadow="never" class="metric-card">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </el-card>
    </section>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.queue }}</strong>
      </template>
      <el-table v-loading="loading" :data="replies" empty-text="No forum replies" style="width: 100%">
        <el-table-column :label="copy.reply" min-width="300">
          <template #default="{ row }">
            <strong>{{ row.content }}</strong>
            <p class="cell-muted">{{ copy.post }}: {{ row.postId }}</p>
            <p class="cell-muted">{{ copy.author }}: {{ row.authorId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.best" width="100">
          <template #default="{ row }">
            <el-tag :type="row.isBest ? 'success' : 'info'">{{ row.isBest ? copy.yes : copy.no }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.review" min-width="220">
          <template #default="{ row }">
            <p class="cell-title">{{ row.reviewReason || copy.noReviewReason }}</p>
            <p class="cell-muted">{{ formatDate(row.reviewedAt) }}</p>
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
                @click="openReview(row, 'approve')"
              >
                {{ copy.approve }}
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="!canReview(row.status)"
                @click="openReview(row, 'reject')"
              >
                {{ copy.reject }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="reviewOpen" :title="reviewDialogTitle" width="min(520px, 92vw)">
      <p class="cell-title">{{ reviewTarget?.content }}</p>
      <p class="cell-muted">{{ copy.reviewHint }}</p>
      <el-form label-position="top" class="mt-4">
        <el-form-item :label="copy.reviewReason">
          <el-input
            v-model="reviewReason"
            type="textarea"
            :rows="4"
            :placeholder="reviewAction === 'reject' ? copy.rejectPlaceholder : copy.approvePlaceholder"
            maxlength="1000"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="reviewOpen = false">{{ copy.cancel }}</el-button>
          <el-button
            :type="reviewAction === 'approve' ? 'success' : 'danger'"
            :loading="reviewing"
            @click="submitReview"
          >
            {{ reviewAction === "approve" ? copy.approve : copy.reject }}
          </el-button>
        </div>
      </template>
    </el-dialog>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type ForumReply } from "../lib/api";

const copy = {
  title: "\u56de\u590d\u5ba1\u6838",
  description:
    "\u5ba1\u6838\u5b66\u751f\u5728\u8bfe\u7a0b\u4e92\u52a9\u5e16\u4e0b\u63d0\u4ea4\u7684\u56de\u590d\u3002\u901a\u8fc7\u540e\u624d\u4f1a\u51fa\u73b0\u5728\u516c\u5f00\u5e16\u5b50\u8be6\u60c5\u91cc\u3002",
  refresh: "\u5237\u65b0",
  queue: "\u5ba1\u6838\u961f\u5217",
  reply: "\u56de\u590d",
  post: "\u5e16\u5b50",
  author: "\u4f5c\u8005",
  status: "\u72b6\u6001",
  best: "\u6700\u4f73",
  yes: "\u662f",
  no: "\u5426",
  review: "\u5ba1\u6838\u8bb0\u5f55",
  actions: "\u64cd\u4f5c",
  approve: "\u901a\u8fc7",
  reject: "\u9a73\u56de",
  cancel: "\u53d6\u6d88",
  reviewReason: "\u5ba1\u6838\u610f\u89c1",
  noReviewReason: "\u5c1a\u65e0\u5ba1\u6838\u610f\u89c1",
  notReviewed: "\u672a\u5ba1\u6838",
  reviewHint: "\u5ba1\u6838\u7ed3\u679c\u4f1a\u5199\u5165\u670d\u52a1\u7aef\u64cd\u4f5c\u65e5\u5fd7\u548c\u56de\u590d\u5ba1\u6838\u5b57\u6bb5\u3002",
  approvePlaceholder: "\u53ef\u9009\uff1a\u8bb0\u5f55\u901a\u8fc7\u4f9d\u636e\u6216\u540e\u7eed\u5907\u6ce8",
  rejectPlaceholder: "\u5fc5\u586b\uff1a\u8bf4\u660e\u9700\u8981\u4fee\u6539\u7684\u539f\u56e0",
  rejectReasonRequired: "\u8bf7\u586b\u5199\u9a73\u56de\u539f\u56e0\u3002",
  loadFailed: "\u56de\u590d\u5ba1\u6838\u961f\u5217\u52a0\u8f7d\u5931\u8d25",
  reviewDone: "\u56de\u590d\u5ba1\u6838\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  reviewFailed: "\u56de\u590d\u5ba1\u6838\u5931\u8d25",
  currentFilter: "\u5f53\u524d\u7b5b\u9009",
  currentResults: "\u5f53\u524d\u7ed3\u679c\u6570",
  draft: "\u8349\u7a3f",
  pending: "\u5f85\u5ba1\u6838",
  needsChanges: "\u9700\u4fee\u6539",
  published: "\u5df2\u901a\u8fc7",
  rejected: "\u5df2\u9a73\u56de",
};

const statuses = [
  { label: copy.draft, value: "draft" },
  { label: copy.pending, value: "pending" },
  { label: copy.needsChanges, value: "needs_changes" },
  { label: copy.published, value: "published" },
  { label: copy.rejected, value: "rejected" },
];

const statusFilter = ref("pending");
const replies = ref<ForumReply[]>([]);
const loading = ref(false);
const reviewing = ref(false);
const reviewOpen = ref(false);
const reviewAction = ref<"approve" | "reject">("approve");
const reviewTarget = ref<ForumReply | null>(null);
const reviewReason = ref("");
const message = ref("");
const error = ref("");

const metrics = computed(() => [
  { label: copy.currentFilter, value: statusLabel(statusFilter.value) },
  { label: copy.currentResults, value: replies.value.length },
]);

const reviewDialogTitle = computed(() => (reviewAction.value === "approve" ? copy.approve : copy.reject));

onMounted(loadReplies);

async function loadReplies() {
  loading.value = true;
  error.value = "";
  try {
    const response = await apiRequest<{ replies: ForumReply[] }>(`/admin/forum/replies?status=${statusFilter.value}`);
    replies.value = response.data?.replies ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function openReview(reply: ForumReply, action: "approve" | "reject") {
  reviewTarget.value = reply;
  reviewAction.value = action;
  reviewReason.value = "";
  message.value = "";
  error.value = "";
  reviewOpen.value = true;
}

async function submitReview() {
  if (!reviewTarget.value) return;
  const reason = reviewReason.value.trim();
  if (reviewAction.value === "reject" && !reason) {
    error.value = copy.rejectReasonRequired;
    return;
  }
  reviewing.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ reviewed: boolean; status: string; reviewReason: string }>(
      `/admin/forum/replies/${reviewTarget.value.id}/${reviewAction.value}`,
      { method: "POST", body: JSON.stringify({ reviewReason: reason }) },
    );
    message.value = copy.reviewDone;
    reviewOpen.value = false;
    await loadReplies();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.reviewFailed;
  } finally {
    reviewing.value = false;
  }
}

function canReview(status: string) {
  return status === "pending" || status === "draft" || status === "needs_changes";
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "published") return "success";
  if (status === "rejected") return "danger";
  if (status === "pending" || status === "needs_changes") return "warning";
  return "info";
}

function formatDate(value?: string) {
  if (!value) return copy.notReviewed;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
}
</script>
