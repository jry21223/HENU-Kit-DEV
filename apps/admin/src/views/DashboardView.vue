<template>
  <main class="admin-shell">
    <aside class="sidebar">
      <strong>Final Review Admin</strong>
      <span>V2 Skeleton</span>
    </aside>
    <section class="content">
      <el-card shadow="never">
        <template #header>
          <strong>仪表盘</strong>
        </template>
        <p>后台骨架已启动。用户、课程、资料、审核、AI、积分会员等模块将在后续阶段接入 Go API。</p>
        <el-table :data="rows" style="width: 100%">
          <el-table-column prop="module" label="模块" />
          <el-table-column prop="status" label="状态" />
        </el-table>
      </el-card>
    </section>
  </main>
</template>

<script setup lang="ts">
const rows = [
  { module: "用户与权限", status: "待接入" },
  { module: "课程资料", status: "待接入" },
  { module: "内容审核", status: "待接入" },
  { module: "AI 工作流", status: "待接入" },
];
</script>
