<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Dashboard</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
    </div>

    <div class="stat-grid">
      <el-card v-for="item in stats" :key="item.label" shadow="never">
        <p class="muted">{{ item.label }}</p>
        <strong class="stat-number">{{ item.value }}</strong>
      </el-card>
    </div>

    <el-card v-if="openPaymentIncidentCount > 0" class="section-card incident-alert-card" shadow="never">
      <template #header>
        <strong>{{ copy.paymentIncidentAlert }}</strong>
      </template>
      <p class="muted">
        {{ copy.paymentIncidentAlertBody.replace("{count}", String(openPaymentIncidentCount)) }}
      </p>
      <div class="action-row incident-alert-actions">
        <RouterLink to="/payment-incidents">
          <el-button type="warning">{{ copy.viewPaymentIncidents }}</el-button>
        </RouterLink>
        <el-button :loading="loadingIncidents" @click="loadPaymentIncidentAlerts">{{ copy.refresh }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.modules }}</strong>
      </template>
      <el-table :data="rows" style="width: 100%">
        <el-table-column prop="module" :label="copy.module" />
        <el-table-column prop="status" :label="copy.status" />
        <el-table-column prop="note" :label="copy.note" />
      </el-table>
    </el-card>
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type PaymentIncidentListResponse } from "../lib/api";

const copy = {
  title: "\u8d44\u6599\u8fd0\u8425\u4eea\u8868\u76d8",
  description:
    "\u5f53\u524d\u540e\u53f0\u805a\u7126\u8bfe\u7a0b\u5165\u53e3\u3001PDF \u8d44\u6599\u4f9b\u5e94\u3001\u4e0b\u8f7d\u5ba1\u8ba1\u548c AI \u8349\u7a3f\u5ba1\u6838\uff0c\u5176\u4ed6\u8fd0\u8425\u80fd\u529b\u7ee7\u7eed\u5206\u9636\u6bb5\u63a5\u5165\u3002",
  refresh: "\u5237\u65b0",
  modules: "\u8d44\u6599\u5e93\u8fd0\u8425\u6a21\u5757",
  module: "\u6a21\u5757",
  status: "\u72b6\u6001",
  note: "\u8bf4\u660e",
  loading: "\u52a0\u8f7d\u4e2d",
  paymentIncidents: "\u652f\u4ed8\u5f02\u5e38",
  paymentIncidentAlert: "\u6709\u672a\u5904\u7406\u7684\u652f\u4ed8\u5f02\u5e38",
  paymentIncidentAlertBody:
    "\u5f53\u524d\u6709 {count} \u6761\u5fae\u4fe1\u56de\u8c03\u5f02\u5e38\u9700\u8981\u6838\u5bf9\u3002\u5904\u7406\u5f02\u5e38\u53ea\u4f1a\u5199\u5165\u8fd0\u8425\u5907\u6ce8\uff0c\u4e0d\u4f1a\u6807\u8bb0\u8ba2\u5355\u5df2\u652f\u4ed8\u6216\u53d1\u653e\u6743\u76ca\u3002",
  viewPaymentIncidents: "\u67e5\u770b\u652f\u4ed8\u5f02\u5e38",
};

const openPaymentIncidentCount = ref(0);
const loadingIncidents = ref(false);

const stats = computed(() => [
  { label: "\u8bfe\u7a0b\u5165\u53e3", value: "\u6309\u8bfe\u7a0b\u7ec4\u7ec7" },
  { label: "PDF \u8d44\u6599", value: "\u7a33\u5b9a\u4f9b\u5e94" },
  { label: "AI \u5ba1\u6838", value: "\u8349\u7a3f\u5148\u884c" },
  {
    label: copy.paymentIncidents,
    value: loadingIncidents.value ? copy.loading : `${openPaymentIncidentCount.value} \u6761\u672a\u5904\u7406`,
  },
]);

const rows = [
  {
    module: "\u8bfe\u7a0b\u8d44\u6599\u5e93",
    status: "\u4e3b\u7ebf",
    note: "\u5b66\u751f\u6309\u8bfe\u7a0b\u8fdb\u5165 PDF \u8d44\u6599\u3001\u8bfe\u7a0b\u5305\u548c\u5237\u9898\u5165\u53e3\u3002",
  },
  {
    module: "\u8d44\u6599\u8fd0\u8425",
    status: "\u5df2\u63a5\u5165",
    note: "\u652f\u6301\u8bfe\u7a0b\u7ef4\u62a4\u3001\u8d44\u6599\u4e0a\u4f20\u3001\u72b6\u6001\u6d41\u8f6c\u548c\u5f52\u6863\u3002",
  },
  {
    module: "AI \u8349\u7a3f\u5ba1\u6838",
    status: "\u5df2\u63a5\u5165",
    note: "Worker \u4ea7\u751f\u7684\u8349\u7a3f\u53ea\u80fd\u7531\u7ba1\u7406\u5458\u5ba1\u6838\uff0c\u4e0d\u4f1a\u81ea\u52a8\u53d1\u5e03\u3002",
  },
  {
    module: "\u8bfe\u7a0b\u793e\u533a",
    status: "\u9884\u7559",
    note: "\u540e\u7eed\u56f4\u7ed5\u8bfe\u7a0b\u8d44\u6599\u8ba8\u8bba\u548c\u8865\u5145\u5efa\u8bae\u63a5\u5165\u3002",
  },
];

onMounted(loadPaymentIncidentAlerts);

async function loadPaymentIncidentAlerts() {
  loadingIncidents.value = true;
  try {
    const response = await apiRequest<PaymentIncidentListResponse>("/admin/payment-incidents?status=open&limit=1");
    openPaymentIncidentCount.value = response.data?.total ?? 0;
  } catch {
    openPaymentIncidentCount.value = 0;
  } finally {
    loadingIncidents.value = false;
  }
}
</script>
