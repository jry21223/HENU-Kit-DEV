<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Material Review</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <div class="action-row">
        <el-select v-model="statusFilter" class="status-filter" @change="loadMaterials">
          <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="loadMaterials">{{ copy.refresh }}</el-button>
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
      <el-table v-loading="loading" :data="materials" empty-text="No materials" style="width: 100%">
        <el-table-column prop="title" :label="copy.name" min-width="220" />
        <el-table-column prop="type" :label="copy.type" width="140" />
        <el-table-column prop="accessLevel" :label="copy.access" width="130" />
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
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
                :disabled="row.status !== 'pending'"
                @click="openReview(row, 'approve')"
              >
                {{ copy.approve }}
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="row.status !== 'pending'"
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
      <p class="cell-title">{{ reviewTarget?.title }}</p>
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
import { apiRequest, type Material } from "../lib/api";

const copy = {
  title: "\u8d44\u6599\u5ba1\u6838",
  description:
    "\u5904\u7406\u5df2\u63d0\u4ea4\u7684\u8bfe\u7a0b\u8d44\u6599\u3002\u901a\u8fc7\u540e\u624d\u4f1a\u8fdb\u5165\u524d\u53f0 published \u72b6\u6001\uff0c\u9a73\u56de\u9700\u8981\u586b\u5199\u539f\u56e0\u3002",
  refresh: "\u5237\u65b0",
  queue: "\u5ba1\u6838\u961f\u5217",
  name: "\u6807\u9898",
  type: "\u7c7b\u578b",
  access: "\u6743\u9650",
  status: "\u72b6\u6001",
  review: "\u5ba1\u6838\u8bb0\u5f55",
  actions: "\u64cd\u4f5c",
  approve: "\u901a\u8fc7",
  reject: "\u9a73\u56de",
  cancel: "\u53d6\u6d88",
  reviewReason: "\u5ba1\u6838\u610f\u89c1",
  noReviewReason: "\u5c1a\u65e0\u5ba1\u6838\u610f\u89c1",
  notReviewed: "\u672a\u5ba1\u6838",
  reviewHint: "\u5ba1\u6838\u7ed3\u679c\u4f1a\u5199\u5165\u670d\u52a1\u7aef\u64cd\u4f5c\u65e5\u5fd7\u548c\u8d44\u6599\u5ba1\u6838\u5b57\u6bb5\u3002",
  approvePlaceholder: "\u53ef\u9009\uff1a\u8bb0\u5f55\u901a\u8fc7\u4f9d\u636e\u6216\u6ce8\u610f\u4e8b\u9879",
  rejectPlaceholder: "\u5fc5\u586b\uff1a\u8bf4\u660e\u9700\u8981\u4fee\u6539\u7684\u539f\u56e0",
  rejectReasonRequired: "\u8bf7\u586b\u5199\u9a73\u56de\u539f\u56e0\u3002",
  loadFailed: "\u5ba1\u6838\u961f\u5217\u52a0\u8f7d\u5931\u8d25",
  reviewDone: "\u8d44\u6599\u5ba1\u6838\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  reviewFailed: "\u8d44\u6599\u5ba1\u6838\u5931\u8d25",
  pending: "\u5f85\u5ba1\u6838",
  published: "\u5df2\u901a\u8fc7",
  rejected: "\u5df2\u9a73\u56de",
};

const statuses = [
  { label: copy.pending, value: "pending" },
  { label: copy.published, value: "published" },
  { label: copy.rejected, value: "rejected" },
];

const statusFilter = ref("pending");
const materials = ref<Material[]>([]);
const loading = ref(false);
const reviewing = ref(false);
const reviewOpen = ref(false);
const reviewAction = ref<"approve" | "reject">("approve");
const reviewTarget = ref<Material | null>(null);
const reviewReason = ref("");
const message = ref("");
const error = ref("");

const metrics = computed(() => [
  { label: copy.pending, value: materials.value.filter((item) => item.status === "pending").length },
  { label: copy.published, value: materials.value.filter((item) => item.status === "published").length },
  { label: copy.rejected, value: materials.value.filter((item) => item.status === "rejected").length },
]);

const reviewDialogTitle = computed(() => (reviewAction.value === "approve" ? copy.approve : copy.reject));

onMounted(loadMaterials);

async function loadMaterials() {
  loading.value = true;
  error.value = "";
  try {
    const response = await apiRequest<{ materials: Material[] }>(`/admin/material-reviews?status=${statusFilter.value}`);
    materials.value = response.data?.materials ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function openReview(material: Material, action: "approve" | "reject") {
  reviewTarget.value = material;
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
      `/admin/materials/${reviewTarget.value.id}/${reviewAction.value}`,
      { method: "POST", body: JSON.stringify({ reviewReason: reason }) },
    );
    message.value = copy.reviewDone;
    reviewOpen.value = false;
    await loadMaterials();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.reviewFailed;
  } finally {
    reviewing.value = false;
  }
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "published") return "success";
  if (status === "rejected") return "danger";
  if (status === "pending") return "warning";
  return "info";
}

function formatDate(value?: string) {
  if (!value) return copy.notReviewed;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
}
</script>
