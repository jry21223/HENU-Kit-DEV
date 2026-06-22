<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Dashboard</p>
        <h1>资料运营仪表盘</h1>
        <p class="muted">当前后台聚焦课程入口、PDF 资料供应和资料保障，审核与社区能力作为后续扩展预留。</p>
      </div>
    </div>

    <div class="stat-grid">
      <el-card v-for="item in stats" :key="item.label" shadow="never">
        <p class="muted">{{ item.label }}</p>
        <strong class="stat-number">{{ item.value }}</strong>
      </el-card>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>资料库运营模块</strong>
      </template>
      <el-table :data="rows" style="width: 100%">
        <el-table-column prop="module" label="模块" />
        <el-table-column prop="status" label="状态" />
        <el-table-column prop="note" label="说明" />
      </el-table>
    </el-card>
  </AdminShell>
</template>

<script setup lang="ts">
import AdminShell from "../components/AdminShell.vue";

const stats = [
  { label: "课程入口", value: "按课程组织" },
  { label: "PDF 资料", value: "稳定供应" },
  { label: "资料保障", value: "轻水印" },
];

const rows = [
  { module: "课程资料库", status: "主线", note: "学生按课程进入 PDF 资料。" },
  { module: "资料运营", status: "已接入", note: "支持课程维护、资料上传和归档。" },
  { module: "内容审核", status: "预留", note: "后续用于 AI 草稿和社区内容审核。" },
  { module: "课程社区", status: "预留", note: "后续围绕课程资料讨论和补充建议。" },
];
</script>
