<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Reports</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <div class="action-row">
        <el-select v-model="statusFilter" class="status-filter" @change="loadReports">
          <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-select v-model="targetFilter" class="status-filter" @change="loadReports">
          <el-option v-for="item in targetTypes" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="loadReports">{{ copy.refresh }}</el-button>
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
      <el-table v-loading="loading" :data="reports" empty-text="No reports" style="width: 100%">
        <el-table-column :label="copy.target" min-width="220">
          <template #default="{ row }">
            <p class="cell-title">{{ targetLabel(row.targetType) }}</p>
            <p class="cell-muted">{{ row.targetId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.reason" min-width="220">
          <template #default="{ row }">
            <p class="cell-title">{{ row.reason }}</p>
            <p class="cell-muted">{{ row.description || copy.noDescription }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="reporterId" :label="copy.reporter" min-width="220" />
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.review" min-width="220">
          <template #default="{ row }">
            <p class="cell-title">{{ row.reviewReason || copy.noReviewReason }}</p>
            <p class="cell-muted">{{ formatDate(row.reviewedAt || row.createdAt) }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.actions" min-width="190" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button
                size="small"
                type="success"
                plain
                :disabled="row.status !== 'pending'"
                :data-testid="`report-review-resolve-${row.id}`"
                @click="openReview(row, 'resolve')"
              >
                {{ copy.resolve }}
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="row.status !== 'pending'"
                :data-testid="`report-review-reject-${row.id}`"
                @click="openReview(row, 'reject')"
              >
                {{ copy.reject }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="reviewOpen" :title="reviewDialogTitle" width="min(540px, 92vw)">
      <p class="cell-title">{{ reviewTarget ? targetLabel(reviewTarget.targetType) : "" }}</p>
      <p class="cell-muted">{{ reviewTarget?.targetId }}</p>
      <p class="cell-title mt-4">{{ reviewTarget?.reason }}</p>
      <p class="cell-muted">{{ reviewTarget?.description || copy.noDescription }}</p>
      <el-form label-position="top" class="mt-4">
        <el-form-item :label="copy.reviewReason">
          <el-input
            v-model="reviewReason"
            type="textarea"
            :rows="4"
            :placeholder="reviewAction === 'reject' ? copy.rejectPlaceholder : copy.resolvePlaceholder"
            maxlength="1000"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="reviewOpen = false">{{ copy.cancel }}</el-button>
          <el-button
            :type="reviewAction === 'resolve' ? 'success' : 'danger'"
            :loading="reviewing"
            data-testid="report-review-submit"
            @click="submitReview"
          >
            {{ reviewAction === "resolve" ? copy.resolve : copy.reject }}
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
import { apiRequest, type Report } from "../lib/api";

const copy = {
  title: "\u4e3e\u62a5\u5904\u7406",
  description:
    "\u5904\u7406\u7528\u6237\u63d0\u4ea4\u7684\u516c\u5f00\u5185\u5bb9\u548c\u7528\u6237\u4e3e\u62a5\u3002\u5904\u7406\u7ed3\u679c\u4f1a\u5199\u5165\u64cd\u4f5c\u65e5\u5fd7\u5e76\u901a\u77e5\u4e3e\u62a5\u4eba\u3002",
  refresh: "\u5237\u65b0",
  queue: "\u4e3e\u62a5\u961f\u5217",
  target: "\u76ee\u6807",
  reason: "\u4e3e\u62a5\u7406\u7531",
  reporter: "\u4e3e\u62a5\u4eba",
  status: "\u72b6\u6001",
  review: "\u5904\u7406\u8bb0\u5f55",
  actions: "\u64cd\u4f5c",
  resolve: "\u5904\u7406",
  reject: "\u9a73\u56de",
  cancel: "\u53d6\u6d88",
  reviewReason: "\u5904\u7406\u8bf4\u660e",
  noDescription: "\u672a\u586b\u5199\u8be6\u7ec6\u8bf4\u660e",
  noReviewReason: "\u5c1a\u65e0\u5904\u7406\u8bb0\u5f55",
  resolvePlaceholder: "\u53ef\u9009\uff1a\u8bb0\u5f55\u5df2\u7ecf\u91c7\u53d6\u7684\u5904\u7406\u63aa\u65bd",
  rejectPlaceholder: "\u5fc5\u586b\uff1a\u8bf4\u660e\u672a\u91c7\u7eb3\u7684\u539f\u56e0",
  rejectReasonRequired: "\u8bf7\u586b\u5199\u9a73\u56de\u539f\u56e0\u3002",
  loadFailed: "\u4e3e\u62a5\u961f\u5217\u52a0\u8f7d\u5931\u8d25",
  reviewDone: "\u4e3e\u62a5\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  reviewFailed: "\u4e3e\u62a5\u5904\u7406\u5931\u8d25",
  pending: "\u5f85\u5904\u7406",
  approved: "\u5df2\u5904\u7406",
  rejected: "\u672a\u91c7\u7eb3",
  all: "\u5168\u90e8",
  material: "\u8d44\u6599",
  wikiEntry: "Wiki \u8bcd\u6761",
  blogPost: "\u535a\u5ba2",
  forumPost: "\u5e16\u5b50",
  forumReply: "\u56de\u590d",
  user: "\u7528\u6237",
};

const statuses = [
  { label: copy.pending, value: "pending" },
  { label: copy.approved, value: "approved" },
  { label: copy.rejected, value: "rejected" },
  { label: copy.all, value: "all" },
];

const targetTypes = [
  { label: copy.all, value: "all" },
  { label: copy.material, value: "material" },
  { label: copy.wikiEntry, value: "wiki_entry" },
  { label: copy.blogPost, value: "blog_post" },
  { label: copy.forumPost, value: "forum_post" },
  { label: copy.forumReply, value: "forum_reply" },
  { label: copy.user, value: "user" },
];

const statusFilter = ref("pending");
const targetFilter = ref("all");
const reports = ref<Report[]>([]);
const loading = ref(false);
const reviewing = ref(false);
const reviewOpen = ref(false);
const reviewAction = ref<"resolve" | "reject">("resolve");
const reviewTarget = ref<Report | null>(null);
const reviewReason = ref("");
const message = ref("");
const error = ref("");

const metrics = computed(() => [
  { label: copy.pending, value: reports.value.filter((item) => item.status === "pending").length },
  { label: copy.approved, value: reports.value.filter((item) => item.status === "approved").length },
  { label: copy.rejected, value: reports.value.filter((item) => item.status === "rejected").length },
]);

const reviewDialogTitle = computed(() => (reviewAction.value === "resolve" ? copy.resolve : copy.reject));

onMounted(loadReports);

async function loadReports() {
  loading.value = true;
  error.value = "";
  const params = new URLSearchParams();
  params.set("status", statusFilter.value);
  if (targetFilter.value !== "all") {
    params.set("targetType", targetFilter.value);
  }
  try {
    const response = await apiRequest<{ reports: Report[] }>(`/admin/reports?${params.toString()}`);
    reports.value = response.data?.reports ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function openReview(report: Report, action: "resolve" | "reject") {
  reviewTarget.value = report;
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
      `/admin/reports/${reviewTarget.value.id}/${reviewAction.value}`,
      { method: "POST", body: JSON.stringify({ reviewReason: reason }) },
    );
    message.value = copy.reviewDone;
    reviewOpen.value = false;
    await loadReports();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.reviewFailed;
  } finally {
    reviewing.value = false;
  }
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function targetLabel(targetType: string) {
  return targetTypes.find((item) => item.value === targetType)?.label ?? targetType;
}

function statusTag(status: string) {
  if (status === "approved") return "success";
  if (status === "rejected") return "danger";
  if (status === "pending") return "warning";
  return "info";
}

function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
}
</script>
