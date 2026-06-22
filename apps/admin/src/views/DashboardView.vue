<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Dashboard</p>
        <h1>仪表盘</h1>
        <p class="muted">当前后台聚焦课程和资料维护。审核、订单、AI、积分会员会在后续模块接入。</p>
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
        <strong>已接入模块</strong>
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
  { label: "管理入口", value: "可用" },
  { label: "课程维护", value: "基础版" },
  { label: "资料上传", value: "本地存储" },
];

const rows = [
  { module: "用户与权限", status: "已接 Go API", note: "邮箱验证码 + admin 路由守卫" },
  { module: "课程资料", status: "基础可用", note: "创建、归档、上传、下载权限" },
  { module: "内容审核", status: "待接入", note: "当前只保留 status 字段" },
  { module: "AI 工作流", status: "待接入", note: "Worker skeleton 已有" },
];
</script>
