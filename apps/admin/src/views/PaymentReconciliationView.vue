<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Payment Reconciliation</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadIssues">{{ copy.refresh }}</el-button>
    </div>

    <div class="stats-grid">
      <el-card class="section-card stat-card" shadow="never">
        <span class="muted">{{ copy.total }}</span>
        <strong>{{ summary?.total ?? 0 }}</strong>
      </el-card>
      <el-card class="section-card stat-card" shadow="never">
        <span class="muted">{{ copy.critical }}</span>
        <strong class="danger-text">{{ summary?.critical ?? 0 }}</strong>
      </el-card>
      <el-card class="section-card stat-card" shadow="never">
        <span class="muted">{{ copy.high }}</span>
        <strong class="warning-text">{{ summary?.high ?? 0 }}</strong>
      </el-card>
      <el-card class="section-card stat-card" shadow="never">
        <span class="muted">{{ copy.mediumLow }}</span>
        <strong>{{ (summary?.medium ?? 0) + (summary?.low ?? 0) }}</strong>
      </el-card>
    </div>

    <el-alert class="notice" type="warning" :closable="false" :title="copy.readOnlyNotice" />

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.issueType">
          <el-input v-model="filters.issueType" clearable placeholder="paid_order_missing_entitlement" />
        </el-form-item>
        <el-form-item :label="copy.severity">
          <el-select v-model="filters.severity" clearable>
            <el-option v-for="item in severities" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="loadIssues">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.issues }}{{ total !== null ? ` (${total})` : "" }}</strong>
      </template>
      <el-table v-loading="loading" :data="issues" empty-text="No reconciliation issues" style="width: 100%">
        <el-table-column :label="copy.issue" min-width="260">
          <template #default="{ row }">
            <strong>{{ row.issueType }}</strong>
            <p class="cell-muted">{{ row.message || copy.empty }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.severity" width="120">
          <template #default="{ row }">
            <el-tag :type="severityTag(row.severity)">{{ row.severity }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.order" min-width="250">
          <template #default="{ row }">
            <strong>{{ row.outTradeNo || copy.empty }}</strong>
            <p class="cell-muted">{{ row.orderId || copy.noOrder }}</p>
            <p v-if="row.orderStatus" class="cell-muted">{{ row.orderStatus }} / {{ row.paymentProvider || copy.empty }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.buyer" min-width="220">
          <template #default="{ row }">
            <span>{{ row.userEmail || row.userId || copy.empty }}</span>
            <p v-if="row.packageTitle" class="cell-muted">{{ row.packageTitle }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.amount" width="130">
          <template #default="{ row }">
            {{ typeof row.amountTotal === "number" ? formatFen(row.amountTotal) : copy.empty }}
          </template>
        </el-table-column>
        <el-table-column :label="copy.evidence" min-width="260">
          <template #default="{ row }">
            <p v-if="row.riskFlag" class="cell-muted">{{ copy.riskFlag }}: {{ row.riskFlag }}</p>
            <p v-if="row.transactionId" class="cell-muted">tx: {{ row.transactionId }}</p>
            <p v-if="row.paymentRecordId" class="cell-muted">record: {{ row.paymentRecordId }}</p>
            <p v-if="row.grantId" class="cell-muted">grant: {{ row.grantId }}</p>
            <p v-if="row.incidentId" class="cell-muted">incident: {{ row.incidentId }}</p>
            <span v-if="!row.riskFlag && !row.transactionId && !row.paymentRecordId && !row.grantId && !row.incidentId">{{ copy.empty }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="180">
          <template #default="{ row }">
            {{ row.createdAt ? formatDate(row.createdAt) : copy.empty }}
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
import { apiRequest, type PaymentReconciliationIssue, type PaymentReconciliationResponse, type PaymentReconciliationSummary } from "../lib/api";

const copy = {
  title: "支付对账报告",
  description: "只读检查本地订单、支付记录、支付异常和权益授权之间的不一致，用于上线前人工核查。",
  refresh: "刷新",
  total: "异常总数",
  critical: "Critical",
  high: "High",
  mediumLow: "Medium / Low",
  readOnlyNotice: "该页面不会修改订单、支付记录或权益授权；发现异常后请结合微信商户后台与支付异常台账人工处理。",
  filters: "筛选",
  issueType: "异常类型",
  severity: "级别",
  apply: "应用筛选",
  reset: "重置",
  issues: "异常列表",
  issue: "异常",
  order: "订单",
  buyer: "用户 / 课程包",
  amount: "金额",
  evidence: "证据",
  riskFlag: "风险标记",
  createdAt: "发现时间",
  empty: "-",
  noOrder: "无本地订单",
  loadFailed: "支付对账报告加载失败",
};

const severities = [
  { label: "Critical", value: "critical" },
  { label: "High", value: "high" },
  { label: "Medium", value: "medium" },
  { label: "Low", value: "low" },
];

const issues = ref<PaymentReconciliationIssue[]>([]);
const summary = ref<PaymentReconciliationSummary | null>(null);
const total = ref<number | null>(null);
const loading = ref(false);
const error = ref("");
const filters = reactive({
  issueType: "",
  severity: "",
});

onMounted(loadIssues);

async function loadIssues() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.issueType.trim()) params.set("issueType", filters.issueType.trim());
    if (filters.severity.trim()) params.set("severity", filters.severity.trim());
    const query = params.toString();
    const response = await apiRequest<PaymentReconciliationResponse>(`/admin/payment-reconciliation${query ? `?${query}` : ""}`);
    issues.value = response.data?.issues ?? [];
    summary.value = response.data?.summary ?? null;
    total.value = response.data?.total ?? issues.value.length;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.issueType = "";
  filters.severity = "";
  void loadIssues();
}

function severityTag(severity: string) {
  if (severity === "critical") return "danger";
  if (severity === "high") return "warning";
  if (severity === "medium") return "info";
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

<style scoped>
.stats-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.stat-card :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.stat-card strong {
  font-size: 26px;
  line-height: 1;
}

.danger-text {
  color: #c2410c;
}

.warning-text {
  color: #b45309;
}
</style>
