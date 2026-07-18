<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Memberships</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.grantTitle }}</strong>
      </template>
      <p class="muted">{{ copy.grantDescription }}</p>
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.user">
          <el-select v-model="grantForm.userId" filterable :placeholder="copy.selectUser">
            <el-option v-for="user in users" :key="user.id" :label="`${user.email} - ${user.name}`" :value="user.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.plan">
          <el-select v-model="grantForm.planCode" filterable :placeholder="copy.selectPlan">
            <el-option
              v-for="plan in plans"
              :key="plan.code"
              :label="`${plan.name} - ${formatPrice(plan.priceFen)}`"
              :value="plan.code"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.expiresAt">
          <el-input v-model="grantForm.expiresAt" clearable :placeholder="copy.expiresPlaceholder" />
        </el-form-item>
        <el-form-item :label="copy.note">
          <el-input v-model="grantForm.note" maxlength="500" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="saving" @click="grantMembership">{{ copy.grantAction }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.user">
          <el-select v-model="filters.userId" clearable filterable :placeholder="copy.allUsers">
            <el-option v-for="user in users" :key="user.id" :label="`${user.email} - ${user.name}`" :value="user.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.plan">
          <el-select v-model="filters.planCode" clearable filterable :placeholder="copy.allPlans">
            <el-option v-for="plan in plans" :key="plan.code" :label="plan.name" :value="plan.code" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="filters.status" clearable :placeholder="copy.allStatus">
            <el-option label="active" value="active" />
            <el-option label="revoked" value="revoked" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button :loading="loading" @click="loadMemberships">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.memberships }}</strong>
      </template>
      <el-table v-loading="loading" :data="memberships" empty-text="No memberships" style="width: 100%">
        <el-table-column :label="copy.user" min-width="230">
          <template #default="{ row }">
            <strong>{{ row.user?.email || row.membership.userId }}</strong>
            <p class="cell-muted">{{ row.user?.name || copy.unknown }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.plan" min-width="190">
          <template #default="{ row }">
            <strong>{{ row.plan?.name || row.membership.planCode }}</strong>
            <p class="cell-muted">{{ row.membership.planCode }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="row.active ? 'success' : row.membership.status === 'revoked' ? 'danger' : 'info'">
              {{ row.membership.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="membership.source" :label="copy.source" width="140" />
        <el-table-column :label="copy.expiresAt" width="180">
          <template #default="{ row }">{{ row.membership.expiresAt ? formatDate(row.membership.expiresAt) : copy.neverExpires }}</template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" width="180">
          <template #default="{ row }">{{ formatDate(row.membership.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="copy.actions" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain :disabled="row.membership.status === 'revoked'" @click="revokeMembership(row)">
              {{ copy.revoke }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.plans }}</strong>
      </template>
      <el-table :data="plans" empty-text="No published plans" style="width: 100%">
        <el-table-column prop="code" :label="copy.planCode" width="140" />
        <el-table-column prop="name" :label="copy.planName" min-width="180" />
        <el-table-column :label="copy.price" width="120">
          <template #default="{ row }">{{ formatPrice(row.priceFen) }}</template>
        </el-table-column>
        <el-table-column prop="status" :label="copy.status" width="120" />
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
import { apiRequest, type MembershipPlan, type MembershipRow, type User } from "../lib/api";

const copy = {
  title: "\u4f1a\u5458\u7ba1\u7406",
  description:
    "\u7ba1\u7406\u5185\u6d4b\u548c\u552e\u540e\u7684\u4f1a\u5458\u624b\u52a8\u53d1\u653e\u3002\u8fd9\u91cc\u4e0d\u521b\u5efa\u652f\u4ed8\u8ba2\u5355\uff0c\u4e5f\u4e0d\u4ee3\u66ff\u5fae\u4fe1\u56de\u8c03\u786e\u8ba4\u3002",
  refresh: "\u5237\u65b0",
  grantTitle: "\u624b\u52a8\u53d1\u653e\u4f1a\u5458",
  grantDescription:
    "\u4ec5\u9002\u7528\u4e8e\u5185\u6d4b\u3001\u552e\u540e\u6216\u4eba\u5de5\u8fd0\u8425\u573a\u666f\u3002\u91cd\u590d\u53d1\u653e\u540c\u4e00\u6709\u6548\u624b\u52a8\u4f1a\u5458\u4f1a\u66f4\u65b0\u539f\u8bb0\u5f55\uff0c\u4e0d\u4f1a\u65e0\u9650\u521b\u5efa\u3002",
  user: "\u7528\u6237",
  selectUser: "\u9009\u62e9\u7528\u6237",
  allUsers: "\u5168\u90e8\u7528\u6237",
  plan: "\u4f1a\u5458\u5957\u9910",
  selectPlan: "\u9009\u62e9\u5df2\u53d1\u5e03\u5957\u9910",
  allPlans: "\u5168\u90e8\u5957\u9910",
  expiresAt: "\u8fc7\u671f\u65f6\u95f4",
  expiresPlaceholder: "2026-07-01T00:00:00Z\uff0c\u7559\u7a7a\u8868\u793a\u957f\u671f",
  note: "\u5907\u6ce8",
  grantAction: "\u53d1\u653e\u4f1a\u5458",
  filters: "\u7b5b\u9009",
  status: "\u72b6\u6001",
  allStatus: "\u5168\u90e8\u72b6\u6001",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  memberships: "\u4f1a\u5458\u8bb0\u5f55",
  source: "\u6765\u6e90",
  neverExpires: "\u957f\u671f\u6709\u6548",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  actions: "\u64cd\u4f5c",
  revoke: "\u64a4\u9500",
  unknown: "\u672a\u5339\u914d",
  plans: "\u5df2\u53d1\u5e03\u5957\u9910",
  planCode: "\u4ee3\u7801",
  planName: "\u540d\u79f0",
  price: "\u4ef7\u683c",
  grantDone: "\u4f1a\u5458\u5df2\u53d1\u653e\u3002",
  grantUpdated: "\u5df2\u66f4\u65b0\u73b0\u6709\u4f1a\u5458\u8bb0\u5f55\u3002",
  grantFailed: "\u4f1a\u5458\u53d1\u653e\u5931\u8d25",
  revokeTitle: "\u786e\u8ba4\u64a4\u9500\u4f1a\u5458\uff1f",
  revokeConfirm: "\u64a4\u9500\u540e\uff0c\u7528\u6237\u5c06\u7acb\u5373\u5931\u53bb\u8be5\u4f1a\u5458\u6743\u76ca\u3002",
  revokeDone: "\u4f1a\u5458\u5df2\u64a4\u9500\u3002",
  revokeFailed: "\u4f1a\u5458\u64a4\u9500\u5931\u8d25",
  loadFailed: "\u4f1a\u5458\u6570\u636e\u52a0\u8f7d\u5931\u8d25",
};

const users = ref<User[]>([]);
const plans = ref<MembershipPlan[]>([]);
const memberships = ref<MembershipRow[]>([]);
const loading = ref(false);
const saving = ref(false);
const message = ref("");
const error = ref("");
const filters = reactive({
  userId: "",
  planCode: "",
  status: "active",
});
const grantForm = reactive({
  userId: "",
  planCode: "",
  expiresAt: "",
  note: "",
});

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [usersResponse, plansResponse] = await Promise.all([
      apiRequest<{ users: User[] }>("/admin/users?limit=500"),
      apiRequest<{ plans: MembershipPlan[] }>("/membership/plans"),
    ]);
    users.value = usersResponse.data?.users ?? [];
    plans.value = plansResponse.data?.plans ?? [];
    if (!grantForm.userId && users.value[0]) grantForm.userId = users.value[0].id;
    if (!grantForm.planCode && plans.value[0]) grantForm.planCode = plans.value[0].code;
    await loadMemberships(false);
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function loadMemberships(withLoading = true) {
  if (withLoading) loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.userId) params.set("userId", filters.userId);
    if (filters.planCode) params.set("planCode", filters.planCode);
    if (filters.status) params.set("status", filters.status);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const response = await apiRequest<{ memberships: MembershipRow[] }>(`/admin/memberships${suffix}`);
    memberships.value = response.data?.memberships ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    if (withLoading) loading.value = false;
  }
}

function resetFilters() {
  filters.userId = "";
  filters.planCode = "";
  filters.status = "active";
  void loadMemberships();
}

async function grantMembership() {
  saving.value = true;
  message.value = "";
  error.value = "";
  try {
    const response = await apiRequest<{ created: boolean }>("/admin/memberships/grant", {
      method: "POST",
      body: JSON.stringify({
        userId: grantForm.userId,
        planCode: grantForm.planCode,
        expiresAt: grantForm.expiresAt.trim(),
        note: grantForm.note.trim(),
      }),
    });
    message.value = response.data?.created ? copy.grantDone : copy.grantUpdated;
    grantForm.note = "";
    await loadMemberships(false);
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.grantFailed;
  } finally {
    saving.value = false;
  }
}

async function revokeMembership(row: MembershipRow) {
  message.value = "";
  error.value = "";
  try {
    await ElMessageBox.confirm(copy.revokeConfirm, copy.revokeTitle, {
      confirmButtonText: copy.revoke,
      cancelButtonText: "\u53d6\u6d88",
      type: "warning",
    });
  } catch {
    return;
  }
  loading.value = true;
  try {
    await apiRequest<{ membership: unknown }>(`/admin/memberships/${row.membership.id}/revoke`, {
      method: "POST",
      body: JSON.stringify({ reason: "admin console revoke" }),
    });
    message.value = copy.revokeDone;
    await loadMemberships(false);
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.revokeFailed;
  } finally {
    loading.value = false;
  }
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatPrice(priceFen: number) {
  if (!Number.isFinite(priceFen) || priceFen <= 0) return "\u514d\u8d39 / \u624b\u52a8";
  return `\u00a5${(priceFen / 100).toFixed(2)}`;
}
</script>
