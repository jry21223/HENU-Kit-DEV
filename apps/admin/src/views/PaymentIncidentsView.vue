<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Payment Incidents</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadIncidents">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.status">
          <el-select v-model="filters.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.incidentType">
          <el-input v-model="filters.incidentType" clearable placeholder="amount_mismatch" />
        </el-form-item>
        <el-form-item :label="copy.outTradeNo">
          <el-input v-model="filters.outTradeNo" clearable placeholder="FR..." />
        </el-form-item>
        <el-form-item :label="copy.transactionId">
          <el-input v-model="filters.transactionId" clearable placeholder="wx transaction id" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadIncidents">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.incidents }}{{ total !== null ? ` (${total})` : "" }}</strong>
      </template>
      <el-table v-loading="loading" :data="incidents" empty-text="No payment incidents" style="width: 100%">
        <el-table-column :label="copy.type" min-width="210">
          <template #default="{ row }">
            <strong>{{ row.incidentType }}</strong>
            <p class="cell-muted">{{ row.provider }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.severity" width="120">
          <template #default="{ row }">
            <el-tag :type="severityTag(row.severity)">{{ row.severity }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.order" min-width="240">
          <template #default="{ row }">
            <strong>{{ row.outTradeNo || copy.empty }}</strong>
            <p class="cell-muted">{{ row.orderId || copy.noOrder }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.transactionId" min-width="220">
          <template #default="{ row }">
            <span>{{ row.transactionId || copy.empty }}</span>
            <p v-if="row.tradeState" class="cell-muted">{{ row.tradeState }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.amount" width="150">
          <template #default="{ row }">
            <strong>{{ formatFen(row.expectedAmount) }}</strong>
            <p class="cell-muted">{{ copy.actual }}: {{ formatFen(row.actualAmount) }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.message" min-width="260">
          <template #default="{ row }">
            <span>{{ row.message || copy.empty }}</span>
            <p v-if="row.handleNote" class="cell-muted">{{ copy.handleNote }}: {{ row.handleNote }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column :label="copy.handledAt" min-width="180">
          <template #default="{ row }">
            {{ row.handledAt ? formatDate(row.handledAt) : copy.empty }}
            <p v-if="row.handledBy" class="cell-muted">{{ row.handledBy }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.actions" fixed="right" width="180">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button
                v-if="row.status === 'open'"
                size="small"
                type="primary"
                :loading="handlingId === row.id"
                @click="handleIncident(row, 'resolved')"
              >
                {{ copy.resolve }}
              </el-button>
              <el-button
                v-if="row.status === 'open'"
                size="small"
                :loading="handlingId === row.id"
                @click="handleIncident(row, 'ignored')"
              >
                {{ copy.ignore }}
              </el-button>
              <span v-if="row.status !== 'open'" class="cell-muted">{{ copy.handled }}</span>
            </div>
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
import { onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type PaymentIncident, type PaymentIncidentListResponse } from "../lib/api";

const copy = {
  title: "\u652f\u4ed8\u5f02\u5e38\u53f0\u8d26",
  description:
    "\u8bb0\u5f55\u5fae\u4fe1 Native \u56de\u8c03\u7684\u9ad8\u98ce\u9669\u5f02\u5e38\uff0c\u53ea\u7528\u4e8e\u4eba\u5de5\u6838\u67e5\u548c\u6807\u8bb0\u5904\u7406\uff0c\u4e0d\u4f1a\u4fee\u6539\u8ba2\u5355\u72b6\u6001\u6216\u53d1\u653e\u6743\u76ca\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  status: "\u5904\u7406\u72b6\u6001",
  incidentType: "\u5f02\u5e38\u7c7b\u578b",
  outTradeNo: "\u5546\u6237\u8ba2\u5355\u53f7",
  transactionId: "\u5fae\u4fe1\u4ea4\u6613\u53f7",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  incidents: "\u5f02\u5e38\u5217\u8868",
  type: "\u7c7b\u578b",
  severity: "\u7ea7\u522b",
  order: "\u8ba2\u5355",
  noOrder: "\u672a\u5339\u914d\u672c\u5730\u8ba2\u5355",
  amount: "\u91d1\u989d",
  actual: "\u5b9e\u9645",
  message: "\u8bf4\u660e",
  handleNote: "\u5904\u7406\u5907\u6ce8",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  handledAt: "\u5904\u7406\u65f6\u95f4",
  actions: "\u64cd\u4f5c",
  resolve: "\u6807\u8bb0\u5df2\u5904\u7406",
  ignore: "\u5ffd\u7565",
  handled: "\u5df2\u7ed3\u675f",
  empty: "-",
  loadFailed: "\u652f\u4ed8\u5f02\u5e38\u52a0\u8f7d\u5931\u8d25",
  handleSuccess: "\u652f\u4ed8\u5f02\u5e38\u5df2\u66f4\u65b0",
  notePrompt: "\u586b\u5199\u4eba\u5de5\u5904\u7406\u5907\u6ce8",
  notePlaceholder: "\u4f8b\uff1a\u5df2\u6838\u5bf9\u5fae\u4fe1\u540e\u53f0\uff0c\u672a\u6536\u6b3e\uff1b\u4fdd\u6301\u4e0d\u53d1\u6743\u76ca\u3002",
};

const statuses = [
  { label: "Open", value: "open" },
  { label: "Resolved", value: "resolved" },
  { label: "Ignored", value: "ignored" },
  { label: "All", value: "all" },
];

const incidents = ref<PaymentIncident[]>([]);
const total = ref<number | null>(null);
const loading = ref(false);
const handlingId = ref("");
const message = ref("");
const error = ref("");
const filters = reactive({
  status: "open",
  incidentType: "",
  outTradeNo: "",
  transactionId: "",
});

onMounted(loadIncidents);

async function loadIncidents() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.status) params.set("status", filters.status);
    if (filters.incidentType.trim()) params.set("incidentType", filters.incidentType.trim());
    if (filters.outTradeNo.trim()) params.set("outTradeNo", filters.outTradeNo.trim());
    if (filters.transactionId.trim()) params.set("transactionId", filters.transactionId.trim());
    const query = params.toString();
    const response = await apiRequest<PaymentIncidentListResponse>(`/admin/payment-incidents${query ? `?${query}` : ""}`);
    incidents.value = response.data?.incidents ?? [];
    total.value = response.data?.total ?? incidents.value.length;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.status = "open";
  filters.incidentType = "";
  filters.outTradeNo = "";
  filters.transactionId = "";
  void loadIncidents();
}

async function handleIncident(row: PaymentIncident, status: "resolved" | "ignored") {
  message.value = "";
  error.value = "";
  try {
    const result = await ElMessageBox.prompt(copy.notePrompt, statusLabel(status), {
      confirmButtonText: status === "resolved" ? copy.resolve : copy.ignore,
      cancelButtonText: "\u53d6\u6d88",
      inputPlaceholder: copy.notePlaceholder,
      inputType: "textarea",
    });
    handlingId.value = row.id;
    await apiRequest<{ incident: PaymentIncident }>(`/admin/payment-incidents/${row.id}/resolve`, {
      method: "POST",
      body: JSON.stringify({ status, handleNote: result.value ?? "" }),
    });
    message.value = copy.handleSuccess;
    await loadIncidents();
  } catch (err) {
    if (isMessageBoxCancel(err)) return;
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    handlingId.value = "";
  }
}

function isMessageBoxCancel(err: unknown) {
  return err === "cancel" || err === "close";
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "open") return "danger";
  if (status === "resolved") return "success";
  if (status === "ignored") return "warning";
  return "info";
}

function severityTag(severity: string) {
  if (severity === "critical") return "danger";
  if (severity === "high") return "warning";
  return "info";
}

function formatFen(value: number) {
  return `CNY ${(value / 100).toFixed(2)}`;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
</script>
