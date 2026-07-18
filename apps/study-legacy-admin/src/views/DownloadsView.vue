<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Downloads</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadDownloads">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.materialId">
          <el-input v-model="filters.materialId" clearable placeholder="material id" />
        </el-form-item>
        <el-form-item :label="copy.userId">
          <el-input v-model="filters.userId" clearable placeholder="user id" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadDownloads">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.records }}</strong>
      </template>
      <el-table v-loading="loading" :data="downloads" empty-text="No download records" style="width: 100%">
        <el-table-column :label="copy.material" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.material?.title ?? copy.archived }}</strong>
            <p class="cell-muted">{{ row.materialId }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="accessLevel" :label="copy.access" width="130" />
        <el-table-column :label="copy.user" min-width="210">
          <template #default="{ row }">
            {{ row.userId ?? copy.anonymous }}
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" min-width="140" />
        <el-table-column prop="userAgent" label="User-Agent" min-width="220" show-overflow-tooltip />
        <el-table-column :label="copy.downloadedAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.downloadedAt) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type DownloadRecord } from "../lib/api";

const copy = {
  title: "\u4e0b\u8f7d\u5ba1\u8ba1",
  description:
    "\u67e5\u770b\u6210\u529f\u4e0b\u8f7d\u7684\u670d\u52a1\u7aef\u5ba1\u8ba1\u65e5\u5fd7\u3002\u5931\u8d25\u9274\u6743\u3001\u5371\u9669\u8def\u5f84\u548c\u7f3a\u5931\u6587\u4ef6\u4e0d\u4f1a\u8bb0\u4e3a\u6210\u529f\u4e0b\u8f7d\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  materialId: "\u8d44\u6599 ID",
  userId: "\u7528\u6237 ID",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  records: "\u5ba1\u8ba1\u8bb0\u5f55",
  material: "\u8d44\u6599",
  access: "\u6743\u9650",
  user: "\u7528\u6237",
  anonymous: "\u672a\u767b\u5f55",
  archived: "\u8d44\u6599\u5df2\u5f52\u6863\u6216\u4e0d\u53ef\u89c1",
  downloadedAt: "\u4e0b\u8f7d\u65f6\u95f4",
  loadFailed: "\u4e0b\u8f7d\u5ba1\u8ba1\u52a0\u8f7d\u5931\u8d25",
};

const downloads = ref<DownloadRecord[]>([]);
const loading = ref(false);
const error = ref("");
const filters = reactive({
  materialId: "",
  userId: "",
});

onMounted(loadDownloads);

async function loadDownloads() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.materialId.trim()) params.set("materialId", filters.materialId.trim());
    if (filters.userId.trim()) params.set("userId", filters.userId.trim());
    const query = params.toString();
    const response = await apiRequest<{ downloads: DownloadRecord[] }>(`/admin/downloads${query ? `?${query}` : ""}`);
    downloads.value = response.data?.downloads ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.materialId = "";
  filters.userId = "";
  void loadDownloads();
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
</script>
