<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Orders</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadOrders">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.status">
          <el-select v-model="filters.status" clearable :placeholder="copy.allStatuses">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.userEmail">
          <el-input v-model="filters.userEmail" clearable placeholder="buyer@stu.henu.edu.cn" />
        </el-form-item>
        <el-form-item :label="copy.outTradeNo">
          <el-input v-model="filters.outTradeNo" clearable placeholder="FR..." />
        </el-form-item>
        <el-form-item :label="copy.packageId">
          <el-input v-model="filters.packageId" clearable placeholder="package id" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadOrders">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.orders }}</strong>
      </template>
      <el-table v-loading="loading" :data="orders" empty-text="No orders" style="width: 100%">
        <el-table-column :label="copy.order" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.order.outTradeNo }}</strong>
            <p class="cell-muted">{{ row.order.id }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.buyer" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.user?.email ?? row.order.userId }}</strong>
            <p v-if="row.user?.name" class="cell-muted">{{ row.user.name }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.product" min-width="240">
          <template #default="{ row }">
            <strong>{{ row.package?.title ?? row.order.productId }}</strong>
            <p class="cell-muted">{{ row.order.productType }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.amount" width="120">
          <template #default="{ row }">
            {{ priceLabel(row) }}
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.order.status)">{{ statusLabel(row.order.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.provider" width="150">
          <template #default="{ row }">
            {{ row.order.paymentProvider }}
          </template>
        </el-table-column>
        <el-table-column :label="copy.entitlement" width="150">
          <template #default="{ row }">
            <el-tag :type="row.entitlementGranted ? 'success' : 'info'">
              {{ row.entitlementGranted ? copy.granted : copy.notGranted }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.order.createdAt) }}
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
import { apiRequest, type OrderRow } from "../lib/api";

const copy = {
  title: "\u8ba2\u5355\u67e5\u8be2",
  description:
    "\u53ea\u8bfb\u67e5\u770b\u8bfe\u7a0b\u5305\u8ba2\u5355\u72b6\u6001\u3001\u4e70\u5bb6\u548c\u6743\u76ca\u53d1\u653e\u60c5\u51b5\u3002\u6b64\u9875\u4e0d\u4f1a\u4fee\u6539\u652f\u4ed8\u72b6\u6001\u6216\u53d1\u653e\u6743\u76ca\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  status: "\u72b6\u6001",
  allStatuses: "\u5168\u90e8\u72b6\u6001",
  userEmail: "\u4e70\u5bb6\u90ae\u7bb1",
  outTradeNo: "\u5546\u6237\u8ba2\u5355\u53f7",
  packageId: "\u8bfe\u7a0b\u5305 ID",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  orders: "\u8ba2\u5355\u5217\u8868",
  order: "\u8ba2\u5355",
  buyer: "\u4e70\u5bb6",
  product: "\u5546\u54c1",
  amount: "\u91d1\u989d",
  provider: "\u652f\u4ed8\u65b9\u5f0f",
  entitlement: "\u6743\u76ca",
  granted: "\u5df2\u6709\u6743\u76ca",
  notGranted: "\u672a\u53d1\u653e",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  loadFailed: "\u8ba2\u5355\u52a0\u8f7d\u5931\u8d25",
};

const statuses = [
  { label: "Pending", value: "pending" },
  { label: "Paying", value: "paying" },
  { label: "Paid", value: "paid" },
  { label: "Closed", value: "closed" },
  { label: "Expired", value: "expired" },
  { label: "Failed", value: "failed" },
  { label: "Cancelled", value: "cancelled" },
  { label: "Refunded", value: "refunded" },
];

const orders = ref<OrderRow[]>([]);
const loading = ref(false);
const error = ref("");
const filters = reactive({
  status: "",
  userEmail: "",
  outTradeNo: "",
  packageId: "",
});

onMounted(loadOrders);

async function loadOrders() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.status) params.set("status", filters.status);
    if (filters.userEmail.trim()) params.set("userEmail", filters.userEmail.trim());
    if (filters.outTradeNo.trim()) params.set("outTradeNo", filters.outTradeNo.trim());
    if (filters.packageId.trim()) params.set("packageId", filters.packageId.trim());
    const query = params.toString();
    const response = await apiRequest<{ orders: OrderRow[] }>(`/admin/orders${query ? `?${query}` : ""}`);
    orders.value = response.data?.orders ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.status = "";
  filters.userEmail = "";
  filters.outTradeNo = "";
  filters.packageId = "";
  void loadOrders();
}

function priceLabel(row: OrderRow) {
  return `${row.order.currency || "CNY"} ${(row.order.amountTotal / 100).toFixed(2)}`;
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "paid") return "success";
  if (status === "failed" || status === "cancelled") return "danger";
  if (status === "refunded" || status === "expired") return "warning";
  return "info";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
</script>
