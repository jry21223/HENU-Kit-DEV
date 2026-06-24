<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Media Assets</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAssets">{{ copy.refresh }}</el-button>
    </div>

    <el-alert class="notice" type="info" :closable="false" :title="copy.boundary" />

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.usage">
          <el-select v-model="filters.usage">
            <el-option label="moment_image" value="moment_image" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="filters.status" clearable>
            <el-option label="uploaded" value="uploaded" />
            <el-option label="attached" value="attached" />
            <el-option label="archived" value="archived" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.ownerEmail">
          <el-input v-model="filters.ownerEmail" clearable placeholder="student@stu.henu.edu.cn" />
        </el-form-item>
        <el-form-item :label="copy.momentId">
          <el-input v-model="filters.momentId" clearable placeholder="moment id" />
        </el-form-item>
        <el-form-item :label="copy.limit">
          <el-input-number v-model="filters.limit" :min="1" :max="500" controls-position="right" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadAssets">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.cleanup }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.olderThanHours">
          <el-input-number v-model="cleanupForm.olderThanHours" :min="1" :max="720" controls-position="right" />
        </el-form-item>
        <el-form-item :label="copy.cleanupLimit">
          <el-input-number v-model="cleanupForm.limit" :min="1" :max="500" controls-position="right" />
        </el-form-item>
      </el-form>
      <p class="muted">{{ copy.cleanupDescription }}</p>
      <div class="action-row mt-4">
        <el-button :loading="cleaning" @click="previewCleanup">{{ copy.preview }}</el-button>
        <el-button type="danger" :loading="cleaning" @click="runCleanup">{{ copy.runCleanup }}</el-button>
      </div>

      <section v-if="cleanupSummary" class="metric-grid mt-4">
        <el-card shadow="never" class="metric-card">
          <span>{{ copy.candidates }}</span>
          <strong>{{ cleanupSummary.candidates }}</strong>
        </el-card>
        <el-card shadow="never" class="metric-card">
          <span>{{ copy.deletedFiles }}</span>
          <strong>{{ cleanupSummary.deletedFiles }}</strong>
        </el-card>
        <el-card shadow="never" class="metric-card">
          <span>{{ copy.archivedRows }}</span>
          <strong>{{ cleanupSummary.archivedRows }}</strong>
        </el-card>
      </section>

      <el-alert
        v-if="cleanupSummary"
        class="notice"
        :type="cleanupSummary.dryRun ? 'warning' : 'success'"
        :closable="false"
        :title="cleanupSummaryTitle"
      />
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.assets }}{{ assets.length ? ` (${assets.length})` : "" }}</strong>
      </template>
      <el-table v-loading="loading" :data="assets" empty-text="No media assets" style="width: 100%">
        <el-table-column :label="copy.file" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.asset.fileName || row.asset.id }}</strong>
            <p class="cell-muted">{{ row.asset.id }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.owner" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.owner?.email ?? row.asset.ownerId }}</strong>
            <p v-if="row.owner?.name" class="cell-muted">{{ row.owner.name }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.asset.status)">{{ row.asset.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.fileState" width="120">
          <template #default="{ row }">
            <el-tag :type="row.hasFile ? 'success' : 'warning'">
              {{ row.hasFile ? copy.hasFile : copy.missingFile }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.size" width="120">
          <template #default="{ row }">
            {{ formatBytes(row.asset.fileSize) }}
          </template>
        </el-table-column>
        <el-table-column prop="asset.contentType" :label="copy.contentType" min-width="140" />
        <el-table-column :label="copy.moment" min-width="220">
          <template #default="{ row }">
            {{ row.asset.momentId ?? copy.unattached }}
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.asset.createdAt) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type MediaAssetRow, type MediaCleanupSummary } from "../lib/api";

const copy = {
  title: "\u5a92\u4f53\u8d44\u4ea7",
  description:
    "\u5ba1\u8ba1\u5b66\u4e60\u52a8\u6001\u56fe\u7247\u8d44\u4ea7\uff0c\u5e76\u6e05\u7406\u8d85\u8fc7\u6307\u5b9a\u65f6\u95f4\u4ecd\u672a\u7ed1\u5b9a\u52a8\u6001\u7684\u4e34\u65f6\u4e0a\u4f20\u3002",
  boundary:
    "\u6e05\u7406\u53ea\u5904\u7406 status=uploaded\u3001moment_id \u4e3a\u7a7a\u7684 moment_image\uff1b\u5df2\u7ed1\u5b9a\u52a8\u6001\u7684\u56fe\u7247\u4e0d\u4f1a\u88ab\u5220\u9664\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  usage: "\u7528\u9014",
  status: "\u72b6\u6001",
  ownerEmail: "\u6240\u6709\u8005\u90ae\u7bb1",
  momentId: "\u52a8\u6001 ID",
  limit: "\u8fd4\u56de\u6761\u6570",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  cleanup: "\u672a\u7ed1\u5b9a\u56fe\u7247\u6e05\u7406",
  olderThanHours: "\u8d85\u8fc7\u5c0f\u65f6",
  cleanupLimit: "\u5355\u6b21\u4e0a\u9650",
  cleanupDescription:
    "\u5148\u6267\u884c dry-run \u67e5\u770b\u5019\u9009\u8d44\u4ea7\u3002\u771f\u5b9e\u6e05\u7406\u4f1a\u5220\u9664\u672c\u5730\u6587\u4ef6\uff0c\u7136\u540e\u5c06\u5bf9\u5e94\u8bb0\u5f55\u5f52\u6863\u3002",
  preview: "\u9884\u68c0\u6e05\u7406",
  runCleanup: "\u6267\u884c\u6e05\u7406",
  cleanupConfirmTitle: "\u786e\u8ba4\u6e05\u7406",
  cleanupConfirm:
    "\u8fd9\u4f1a\u5220\u9664\u672c\u5730\u672a\u7ed1\u5b9a\u56fe\u7247\u6587\u4ef6\u5e76\u5f52\u6863\u8bb0\u5f55\u3002\u5df2\u7ed1\u5b9a\u52a8\u6001\u7684\u56fe\u7247\u4e0d\u4f1a\u88ab\u5904\u7406\u3002",
  confirm: "\u786e\u8ba4\u6267\u884c",
  cancel: "\u53d6\u6d88",
  candidates: "\u5019\u9009\u8d44\u4ea7",
  deletedFiles: "\u5df2\u5220\u6587\u4ef6",
  archivedRows: "\u5f52\u6863\u8bb0\u5f55",
  missingFiles: "\u6587\u4ef6\u7f3a\u5931",
  dryRun: "Dry-run",
  realRun: "\u5df2\u6267\u884c",
  assets: "\u8d44\u4ea7\u5217\u8868",
  file: "\u6587\u4ef6",
  owner: "\u6240\u6709\u8005",
  fileState: "\u6587\u4ef6",
  hasFile: "\u5b58\u5728",
  missingFile: "\u7f3a\u5931",
  size: "\u5927\u5c0f",
  contentType: "\u7c7b\u578b",
  moment: "\u5173\u8054\u52a8\u6001",
  unattached: "\u672a\u7ed1\u5b9a",
  createdAt: "\u4e0a\u4f20\u65f6\u95f4",
  loadFailed: "\u5a92\u4f53\u8d44\u4ea7\u52a0\u8f7d\u5931\u8d25",
  cleanupFailed: "\u5a92\u4f53\u8d44\u4ea7\u6e05\u7406\u5931\u8d25",
};

const assets = ref<MediaAssetRow[]>([]);
const cleanupSummary = ref<MediaCleanupSummary | null>(null);
const loading = ref(false);
const cleaning = ref(false);
const message = ref("");
const error = ref("");

const filters = reactive({
  usage: "moment_image",
  status: "",
  ownerEmail: "",
  momentId: "",
  limit: 200,
});

const cleanupForm = reactive({
  olderThanHours: 24,
  limit: 200,
});

const cleanupSummaryTitle = computed(() => {
  if (!cleanupSummary.value) return "";
  const summary = cleanupSummary.value;
  const prefix = summary.dryRun ? copy.dryRun : copy.realRun;
  return `${prefix}: ${copy.candidates} ${summary.candidates}, ${copy.deletedFiles} ${summary.deletedFiles}, ${copy.missingFiles} ${summary.missingFiles}, ${copy.archivedRows} ${summary.archivedRows}`;
});

onMounted(loadAssets);

async function loadAssets() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.usage) params.set("usage", filters.usage);
    if (filters.status) params.set("status", filters.status);
    if (filters.ownerEmail.trim()) params.set("ownerEmail", filters.ownerEmail.trim());
    if (filters.momentId.trim()) params.set("momentId", filters.momentId.trim());
    params.set("limit", String(filters.limit));
    const response = await apiRequest<{ assets: MediaAssetRow[] }>(`/admin/media-assets?${params.toString()}`);
    assets.value = response.data?.assets ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.usage = "moment_image";
  filters.status = "";
  filters.ownerEmail = "";
  filters.momentId = "";
  filters.limit = 200;
  void loadAssets();
}

async function previewCleanup() {
  await cleanup(true);
}

async function runCleanup() {
  try {
    await ElMessageBox.confirm(copy.cleanupConfirm, copy.cleanupConfirmTitle, {
      confirmButtonText: copy.confirm,
      cancelButtonText: copy.cancel,
      type: "warning",
    });
  } catch (err) {
    if (err === "cancel" || err === "close") return;
    throw err;
  }
  await cleanup(false);
}

async function cleanup(dryRun: boolean) {
  cleaning.value = true;
  message.value = "";
  error.value = "";
  try {
    const response = await apiRequest<{ cleanup: MediaCleanupSummary }>("/admin/media-assets/cleanup", {
      method: "POST",
      body: JSON.stringify({
        olderThanHours: cleanupForm.olderThanHours,
        dryRun,
        limit: cleanupForm.limit,
      }),
    });
    cleanupSummary.value = response.data?.cleanup ?? null;
    if (!dryRun) {
      message.value = cleanupSummaryTitle.value;
      await loadAssets();
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.cleanupFailed;
  } finally {
    cleaning.value = false;
  }
}

function statusTag(status: string) {
  if (status === "attached") return "success";
  if (status === "uploaded") return "warning";
  if (status === "archived") return "info";
  return "info";
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
</script>
