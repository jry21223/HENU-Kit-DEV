<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Wiki Proposal Review</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <div class="action-row">
        <el-select v-model="statusFilter" class="status-filter" @change="loadProposals">
          <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="loadProposals">{{ copy.refresh }}</el-button>
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
      <el-table v-loading="loading" :data="proposals" empty-text="No wiki proposals" style="width: 100%">
        <el-table-column :label="copy.proposal" min-width="300">
          <template #default="{ row }">
            <strong>{{ row.proposedTitle }}</strong>
            <p class="cell-muted">{{ copy.entry }}: {{ row.entryId }}</p>
            <p class="cell-muted">{{ copy.editor }}: {{ row.editorId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.version" width="130">
          <template #default="{ row }">v{{ row.baseVersion }} -> v{{ row.baseVersion + 1 }}</template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.content" min-width="320">
          <template #default="{ row }">
            <p class="content-preview">{{ row.proposedContent }}</p>
            <p class="cell-muted">{{ row.summary || copy.noSummary }}</p>
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

    <el-dialog v-model="reviewOpen" :title="reviewDialogTitle" width="min(560px, 92vw)">
      <p class="cell-title">{{ reviewTarget?.proposedTitle }}</p>
      <p class="cell-muted">{{ copy.reviewHint }}</p>
      <el-alert
        v-if="versionMismatch"
        class="notice"
        type="warning"
        :closable="false"
        :title="copy.versionMismatch"
      />
      <div class="proposal-compare">
        <section class="compare-panel">
          <span class="cell-muted">{{ copy.baseEntry }}</span>
          <strong>{{ copy.proposalBaseVersion }}: v{{ reviewTarget?.baseVersion }}</strong>
          <p class="cell-muted">{{ reviewTarget?.baseSummary || copy.noSummary }}</p>
          <p class="content-preview compare-content">{{ reviewTarget?.baseContent || copy.baseContentMissing }}</p>
        </section>
        <section class="compare-panel">
          <span class="cell-muted">{{ copy.currentEntry }}</span>
          <strong>{{ reviewTarget?.currentTitle || copy.currentEntryMissing }}</strong>
          <p class="cell-muted">
            {{ copy.currentVersion }}:
            {{ reviewTarget?.currentVersion ? `v${reviewTarget.currentVersion}` : "-" }}
            <span v-if="reviewTarget?.currentStatus">/ {{ statusLabel(reviewTarget.currentStatus) }}</span>
          </p>
          <p class="content-preview compare-content">{{ reviewTarget?.currentContent || copy.currentEntryMissing }}</p>
        </section>
        <section class="compare-panel">
          <span class="cell-muted">{{ copy.proposedEntry }}</span>
          <strong>{{ reviewTarget?.proposedTitle }}</strong>
          <p class="cell-muted">{{ copy.proposalBaseVersion }}: v{{ reviewTarget?.baseVersion }}</p>
          <p class="content-preview compare-content">{{ reviewTarget?.proposedContent }}</p>
        </section>
      </div>
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
import { apiRequest, type WikiEditProposal } from "../lib/api";

const copy = {
  title: "Wiki \u7f16\u8f91\u63d0\u6848\u5ba1\u6838",
  description:
    "\u5ba1\u6838\u5df2\u53d1\u5e03 Wiki \u8bcd\u6761\u7684\u7f16\u8f91\u63d0\u6848\u3002\u901a\u8fc7\u540e\u624d\u4f1a\u66f4\u65b0\u516c\u5f00\u8bcd\u6761\u548c\u7248\u672c\u5386\u53f2\u3002",
  refresh: "\u5237\u65b0",
  queue: "\u5ba1\u6838\u961f\u5217",
  proposal: "\u63d0\u6848",
  entry: "\u8bcd\u6761",
  editor: "\u7f16\u8f91\u8005",
  version: "\u7248\u672c",
  status: "\u72b6\u6001",
  content: "\u4fee\u8ba2\u5185\u5bb9",
  review: "\u5ba1\u6838\u8bb0\u5f55",
  actions: "\u64cd\u4f5c",
  approve: "\u901a\u8fc7",
  reject: "\u9a73\u56de",
  cancel: "\u53d6\u6d88",
  reviewReason: "\u5ba1\u6838\u610f\u89c1",
  noSummary: "\u672a\u586b\u5199\u4fee\u8ba2\u6458\u8981",
  noReviewReason: "\u5c1a\u65e0\u5ba1\u6838\u610f\u89c1",
  notReviewed: "\u672a\u5ba1\u6838",
  reviewHint: "\u901a\u8fc7\u5c06\u4e8b\u52a1\u6027\u66f4\u65b0\u516c\u5f00 Wiki \u8bcd\u6761\uff0c\u5e76\u5199\u5165\u7248\u672c\u5386\u53f2\u548c\u64cd\u4f5c\u65e5\u5fd7\u3002",
  approvePlaceholder: "\u53ef\u9009\uff1a\u8bb0\u5f55\u901a\u8fc7\u4f9d\u636e\u6216\u540e\u7eed\u5907\u6ce8",
  rejectPlaceholder: "\u5fc5\u586b\uff1a\u8bf4\u660e\u9700\u8981\u4fee\u6539\u7684\u539f\u56e0",
  rejectReasonRequired: "\u8bf7\u586b\u5199\u9a73\u56de\u539f\u56e0\u3002",
  baseEntry: "\u57fa\u51c6\u7248\u672c\u5185\u5bb9",
  currentEntry: "\u5f53\u524d\u516c\u5f00\u8bcd\u6761",
  proposedEntry: "\u63d0\u6848\u5185\u5bb9",
  currentVersion: "\u5f53\u524d\u7248\u672c",
  proposalBaseVersion: "\u63d0\u6848\u57fa\u51c6\u7248\u672c",
  baseContentMissing: "\u672a\u627e\u5230\u57fa\u51c6\u7248\u672c\u5386\u53f2\u5185\u5bb9",
  currentEntryMissing: "\u65e0\u6cd5\u8bfb\u53d6\u5f53\u524d\u8bcd\u6761",
  versionMismatch:
    "\u5f53\u524d\u8bcd\u6761\u7248\u672c\u5df2\u53d8\u66f4\uff0c\u670d\u52a1\u7aef\u5c06\u62d2\u7edd\u8fc7\u671f\u63d0\u6848\u901a\u8fc7\u3002",
  loadFailed: "Wiki \u7f16\u8f91\u63d0\u6848\u52a0\u8f7d\u5931\u8d25",
  reviewDone: "Wiki \u7f16\u8f91\u63d0\u6848\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  reviewFailed: "Wiki \u7f16\u8f91\u63d0\u6848\u5ba1\u6838\u5931\u8d25",
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
const proposals = ref<WikiEditProposal[]>([]);
const loading = ref(false);
const reviewing = ref(false);
const reviewOpen = ref(false);
const reviewAction = ref<"approve" | "reject">("approve");
const reviewTarget = ref<WikiEditProposal | null>(null);
const reviewReason = ref("");
const message = ref("");
const error = ref("");

const metrics = computed(() => [
  { label: copy.currentFilter, value: statusLabel(statusFilter.value) },
  { label: copy.currentResults, value: proposals.value.length },
]);

const reviewDialogTitle = computed(() => (reviewAction.value === "approve" ? copy.approve : copy.reject));
const versionMismatch = computed(() => Boolean(reviewTarget.value?.isStale));

onMounted(loadProposals);

async function loadProposals() {
  loading.value = true;
  error.value = "";
  try {
    const response = await apiRequest<{ proposals: WikiEditProposal[] }>(
      `/admin/wiki/proposals?status=${statusFilter.value}`,
    );
    proposals.value = response.data?.proposals ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function openReview(proposal: WikiEditProposal, action: "approve" | "reject") {
  reviewTarget.value = proposal;
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
      `/admin/wiki/proposals/${reviewTarget.value.id}/${reviewAction.value}`,
      { method: "POST", body: JSON.stringify({ reviewReason: reason }) },
    );
    message.value = copy.reviewDone;
    reviewOpen.value = false;
    await loadProposals();
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

<style scoped>
.proposal-compare {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  margin-top: 16px;
}

.compare-panel {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 12px;
}

.compare-content {
  max-height: 180px;
  overflow: auto;
}
</style>
