<template>
  <AdminShellV2 :title="title" environment="runtime">
    <div class="admin-v2-page-heading">
      <div><p>{{ domain.toUpperCase() }}</p><h1>{{ title }}</h1><span>{{ description }}</span></div>
      <RouterLink to="/dashboard">返回总览</RouterLink>
    </div>
    <Alert variant="info">该业务域已经进入统一导航；详细列表和写操作按契约分 PR 接入。总览卡片只展示当前真实可用的数据。</Alert>
  </AdminShellV2>
</template>
<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import AdminShellV2 from "../components/AdminShellV2.vue";
import Alert from "../components/ui/Alert.vue";
const route = useRoute();
const title = computed(() => String(route.meta.title ?? "业务运营"));
const domain = computed(() => String(route.meta.domain ?? "operations"));
const description = computed(() => String(route.meta.description ?? "统一管理后台业务域"));
</script>
