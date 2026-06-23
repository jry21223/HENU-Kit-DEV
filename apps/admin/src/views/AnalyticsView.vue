<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Analytics</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadOverview">{{ copy.refresh }}</el-button>
    </div>

    <div class="stat-grid">
      <el-card v-for="item in stats" :key="item.label" shadow="never">
        <p class="muted">{{ item.label }}</p>
        <strong class="stat-number">{{ item.value }}</strong>
      </el-card>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.trend }}</strong>
      </template>
      <el-table v-loading="loading" :data="overview?.downloadTrend ?? []" empty-text="No trend data" style="width: 100%">
        <el-table-column prop="date" :label="copy.date" min-width="160" />
        <el-table-column prop="count" :label="copy.downloads" min-width="140" />
      </el-table>
    </el-card>

    <div class="analytics-grid">
      <el-card class="section-card" shadow="never">
        <template #header>
          <strong>{{ copy.topMaterials }}</strong>
        </template>
        <el-table v-loading="loading" :data="overview?.topMaterials ?? []" empty-text="No download records" style="width: 100%">
          <el-table-column :label="copy.material" min-width="220">
            <template #default="{ row }">
              <strong>{{ row.title }}</strong>
              <p class="cell-muted">{{ row.materialId }}</p>
            </template>
          </el-table-column>
          <el-table-column prop="accessLevel" :label="copy.access" width="130" />
          <el-table-column prop="downloads" :label="copy.downloads" width="120" />
        </el-table>
      </el-card>

      <el-card class="section-card" shadow="never">
        <template #header>
          <strong>{{ copy.accessBreakdown }}</strong>
        </template>
        <el-table v-loading="loading" :data="overview?.accessBreakdown ?? []" empty-text="No access data" style="width: 100%">
          <el-table-column prop="accessLevel" :label="copy.access" min-width="160" />
          <el-table-column prop="downloads" :label="copy.downloads" min-width="120" />
        </el-table>
      </el-card>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.courseDemand }}</strong>
      </template>
      <el-table v-loading="loading" :data="overview?.courseDemand ?? []" empty-text="No course data" style="width: 100%">
        <el-table-column :label="copy.course" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.courseName }}</strong>
            <p class="cell-muted">{{ row.courseId }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="grade" :label="copy.grade" width="110" />
        <el-table-column prop="publishedMaterialCount" :label="copy.publishedMaterials" min-width="150" />
        <el-table-column prop="materialCount" :label="copy.materials" min-width="130" />
        <el-table-column prop="downloadCount" :label="copy.downloads" min-width="130" />
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="row.status === 'published' ? 'success' : 'info'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type AnalyticsOverview } from "../lib/api";

const copy = {
  title: "\u8fd0\u8425\u5206\u6790",
  description:
    "\u57fa\u4e8e\u670d\u52a1\u7aef\u6210\u529f\u4e0b\u8f7d\u65e5\u5fd7\u548c\u8bfe\u7a0b\u8d44\u6599\u4f9b\u7ed9\u60c5\u51b5\uff0c\u67e5\u770b\u8fd1\u671f\u8d44\u6599\u9700\u6c42\u3002\u8be5\u9875\u53ea\u8bfb\uff0c\u4e0d\u4f1a\u53d1\u653e\u6743\u9650\u6216\u4fee\u6539\u8d44\u6599\u72b6\u6001\u3002",
  refresh: "\u5237\u65b0",
  users: "\u7528\u6237",
  courses: "\u8bfe\u7a0b",
  materials: "\u8d44\u6599",
  publishedMaterials: "\u5df2\u53d1\u5e03\u8d44\u6599",
  downloads: "\u4e0b\u8f7d",
  packages: "\u8bfe\u7a0b\u5305",
  trend: "14 \u5929\u4e0b\u8f7d\u8d8b\u52bf",
  date: "\u65e5\u671f",
  topMaterials: "\u70ed\u95e8\u8d44\u6599",
  accessBreakdown: "\u6743\u9650\u5206\u5e03",
  material: "\u8d44\u6599",
  access: "\u6743\u9650",
  courseDemand: "\u8bfe\u7a0b\u9700\u6c42",
  course: "\u8bfe\u7a0b",
  grade: "\u5e74\u7ea7",
  status: "\u72b6\u6001",
  loadFailed: "\u8fd0\u8425\u5206\u6790\u52a0\u8f7d\u5931\u8d25",
};

const overview = ref<AnalyticsOverview | null>(null);
const loading = ref(false);
const error = ref("");

const stats = computed(() => [
  { label: copy.users, value: overview.value?.totals.users ?? 0 },
  { label: copy.courses, value: overview.value?.totals.courses ?? 0 },
  { label: copy.publishedMaterials, value: overview.value?.totals.publishedMaterials ?? 0 },
  { label: copy.downloads, value: overview.value?.totals.downloads ?? 0 },
  { label: copy.packages, value: overview.value?.totals.packages ?? 0 },
  { label: copy.materials, value: overview.value?.totals.materials ?? 0 },
]);

onMounted(loadOverview);

async function loadOverview() {
  loading.value = true;
  error.value = "";
  try {
    const response = await apiRequest<AnalyticsOverview>("/admin/analytics/overview");
    overview.value = response.data ?? null;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}
</script>
