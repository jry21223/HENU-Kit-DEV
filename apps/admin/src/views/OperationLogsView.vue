<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Operation Logs</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadLogs">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.operatorId">
          <el-input v-model="filters.operatorId" clearable placeholder="operator id" />
        </el-form-item>
        <el-form-item :label="copy.action">
          <el-input v-model="filters.action" clearable placeholder="material.update" />
        </el-form-item>
        <el-form-item :label="copy.targetType">
          <el-select v-model="filters.targetType" clearable filterable placeholder="target type">
            <el-option label="course" value="course" />
            <el-option label="material" value="material" />
            <el-option label="school" value="school" />
            <el-option label="college" value="college" />
            <el-option label="major" value="major" />
            <el-option label="ai_draft" value="ai_draft" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.targetId">
          <el-input v-model="filters.targetId" clearable placeholder="target id" />
        </el-form-item>
        <el-form-item :label="copy.limit">
          <el-input-number v-model="filters.limit" :min="1" :max="500" controls-position="right" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadLogs">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.records }}</strong>
      </template>
      <el-table v-loading="loading" :data="logs" empty-text="No operation logs" style="width: 100%">
        <el-table-column prop="action" :label="copy.action" min-width="180" />
        <el-table-column :label="copy.target" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.targetType || copy.unknown }}</strong>
            <p class="cell-muted">{{ row.targetId || copy.empty }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.operator" min-width="220">
          <template #default="{ row }">
            {{ row.operatorId }}
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" min-width="130" />
        <el-table-column prop="userAgent" label="User-Agent" min-width="220" show-overflow-tooltip />
        <el-table-column :label="copy.metadata" min-width="260">
          <template #default="{ row }">
            <code class="json-cell">{{ formatMetadata(row.metadata) }}</code>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.createdAt) }}
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
import { apiRequest, type OperationLog } from "../lib/api";

const copy = {
  title: "\u64cd\u4f5c\u65e5\u5fd7",
  description:
    "\u67e5\u770b\u7ba1\u7406\u5458\u5728\u670d\u52a1\u7aef\u7559\u4e0b\u7684\u9ad8\u98ce\u9669\u53d8\u66f4\u5ba1\u8ba1\u8bb0\u5f55\u3002\u8fd9\u4e2a\u9875\u9762\u53ea\u8bfb\uff0c\u4e0d\u63d0\u4f9b\u4fee\u6539\u6216\u5220\u9664\u65e5\u5fd7\u7684\u80fd\u529b\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  operatorId: "\u64cd\u4f5c\u4eba ID",
  action: "\u52a8\u4f5c",
  targetType: "\u76ee\u6807\u7c7b\u578b",
  targetId: "\u76ee\u6807 ID",
  limit: "\u8fd4\u56de\u6761\u6570",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  records: "\u5ba1\u8ba1\u8bb0\u5f55",
  target: "\u76ee\u6807",
  operator: "\u64cd\u4f5c\u4eba",
  metadata: "\u5143\u6570\u636e",
  createdAt: "\u8bb0\u5f55\u65f6\u95f4",
  unknown: "\u672a\u77e5",
  empty: "\u65e0",
  loadFailed: "\u64cd\u4f5c\u65e5\u5fd7\u52a0\u8f7d\u5931\u8d25",
};

const logs = ref<OperationLog[]>([]);
const loading = ref(false);
const error = ref("");
const filters = reactive({
  operatorId: "",
  action: "",
  targetType: "",
  targetId: "",
  limit: 200,
});

onMounted(loadLogs);

async function loadLogs() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.operatorId.trim()) params.set("operatorId", filters.operatorId.trim());
    if (filters.action.trim()) params.set("action", filters.action.trim());
    if (filters.targetType.trim()) params.set("targetType", filters.targetType.trim());
    if (filters.targetId.trim()) params.set("targetId", filters.targetId.trim());
    params.set("limit", String(filters.limit));
    const response = await apiRequest<{ operationLogs: OperationLog[] }>(`/admin/operation-logs?${params.toString()}`);
    logs.value = response.data?.operationLogs ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.operatorId = "";
  filters.action = "";
  filters.targetType = "";
  filters.targetId = "";
  filters.limit = 200;
  void loadLogs();
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatMetadata(value: unknown) {
  if (value == null || value === "") return "-";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
</script>

<style scoped>
.json-cell {
  display: inline-block;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #475569;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  padding: 2px 6px;
}
</style>
