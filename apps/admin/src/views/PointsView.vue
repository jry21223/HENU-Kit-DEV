<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Points</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.logFilters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.user">
          <el-select v-model="filters.userId" clearable filterable :placeholder="copy.allUsers">
            <el-option v-for="user in users" :key="user.id" :label="`${user.email} - ${user.name}`" :value="user.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.reason">
          <el-input v-model="filters.reason" clearable placeholder="forum_reward_settlement" @keyup.enter="loadLogs" />
        </el-form-item>
        <el-form-item :label="copy.limit">
          <el-input-number v-model="filters.limit" :min="1" :max="500" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button :loading="loading" @click="loadLogs">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.logs }}</strong>
      </template>
      <el-table v-loading="loading" :data="logs" empty-text="No point logs" style="width: 100%">
        <el-table-column :label="copy.user" min-width="220">
          <template #default="{ row }">
            <strong>{{ userLabel(row.userId) }}</strong>
            <p class="cell-muted">{{ row.userId }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.delta" width="110">
          <template #default="{ row }">
            <el-tag :type="row.delta >= 0 ? 'success' : 'warning'">{{ row.delta >= 0 ? `+${row.delta}` : row.delta }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="balanceAfter" :label="copy.balanceAfter" width="120" />
        <el-table-column prop="reason" :label="copy.reason" min-width="180" />
        <el-table-column :label="copy.reference" min-width="220">
          <template #default="{ row }">
            <span>{{ row.referenceType || "-" }}</span>
            <p class="cell-muted">{{ row.referenceId || "-" }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" width="180">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <div class="card-header-row">
          <strong>{{ copy.rules }}</strong>
          <el-button type="primary" size="small" @click="openCreateRule">{{ copy.newRule }}</el-button>
        </div>
      </template>
      <el-table v-loading="loading" :data="rules" empty-text="No point rules" style="width: 100%">
        <el-table-column prop="code" :label="copy.code" min-width="180" />
        <el-table-column prop="description" :label="copy.ruleDescription" min-width="240" />
        <el-table-column prop="delta" :label="copy.delta" width="110" />
        <el-table-column :label="copy.enabled" width="110">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? copy.yes : copy.no }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.updatedAt" width="180">
          <template #default="{ row }">{{ formatDate(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column :label="copy.actions" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEditRule(row)">{{ copy.edit }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="ruleDialogOpen" :title="editingRuleId ? copy.editRule : copy.newRule" width="min(560px, 92vw)">
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.code">
          <el-input v-model="ruleForm.code" maxlength="100" />
        </el-form-item>
        <el-form-item :label="copy.delta">
          <el-input-number v-model="ruleForm.delta" />
        </el-form-item>
        <el-form-item :label="copy.ruleDescription">
          <el-input v-model="ruleForm.description" maxlength="500" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="copy.enabled">
          <el-switch v-model="ruleForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="ruleDialogOpen = false">{{ copy.cancel }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveRule">{{ copy.save }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type PointsLog, type PointsRule, type User } from "../lib/api";

const copy = {
  title: "\u79ef\u5206\u7ba1\u7406",
  description:
    "\u67e5\u770b\u79ef\u5206\u6d41\u6c34\u5e76\u7ef4\u62a4\u79ef\u5206\u89c4\u5219\u3002\u6240\u6709\u4f59\u989d\u53d8\u52a8\u5fc5\u987b\u6765\u81ea\u670d\u52a1\u7aef\u6d41\u6c34\uff0c\u8fd9\u91cc\u4e0d\u76f4\u63a5\u6539\u7528\u6237\u79ef\u5206\u4f59\u989d\u3002",
  refresh: "\u5237\u65b0",
  logFilters: "\u6d41\u6c34\u7b5b\u9009",
  user: "\u7528\u6237",
  allUsers: "\u5168\u90e8\u7528\u6237",
  reason: "\u539f\u56e0",
  limit: "\u6570\u91cf",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  logs: "\u79ef\u5206\u6d41\u6c34",
  delta: "\u53d8\u52a8",
  balanceAfter: "\u53d8\u52a8\u540e",
  reference: "\u5173\u8054\u8d44\u6e90",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  rules: "\u79ef\u5206\u89c4\u5219",
  newRule: "\u65b0\u589e\u89c4\u5219",
  editRule: "\u7f16\u8f91\u89c4\u5219",
  code: "\u89c4\u5219\u4ee3\u7801",
  ruleDescription: "\u8bf4\u660e",
  enabled: "\u542f\u7528",
  yes: "\u542f\u7528",
  no: "\u505c\u7528",
  updatedAt: "\u66f4\u65b0\u65f6\u95f4",
  actions: "\u64cd\u4f5c",
  edit: "\u7f16\u8f91",
  cancel: "\u53d6\u6d88",
  save: "\u4fdd\u5b58",
  loadFailed: "\u79ef\u5206\u6570\u636e\u52a0\u8f7d\u5931\u8d25",
  saveDone: "\u79ef\u5206\u89c4\u5219\u5df2\u4fdd\u5b58\u3002",
  saveFailed: "\u79ef\u5206\u89c4\u5219\u4fdd\u5b58\u5931\u8d25",
};

const users = ref<User[]>([]);
const logs = ref<PointsLog[]>([]);
const rules = ref<PointsRule[]>([]);
const loading = ref(false);
const saving = ref(false);
const message = ref("");
const error = ref("");
const ruleDialogOpen = ref(false);
const editingRuleId = ref("");
const filters = reactive({
  userId: "",
  reason: "",
  limit: 100,
});
const ruleForm = reactive({
  code: "",
  description: "",
  delta: 0,
  enabled: true,
});

const usersByID = computed(() => Object.fromEntries(users.value.map((user) => [user.id, user])));

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const usersResponse = await apiRequest<{ users: User[] }>("/admin/users?limit=500");
    users.value = usersResponse.data?.users ?? [];
    await Promise.all([loadLogs(false), loadRules(false)]);
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function loadLogs(withLoading = true) {
  if (withLoading) loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.userId) params.set("userId", filters.userId);
    if (filters.reason.trim()) params.set("reason", filters.reason.trim());
    params.set("limit", String(filters.limit));
    const response = await apiRequest<{ logs: PointsLog[] }>(`/admin/points/logs?${params.toString()}`);
    logs.value = response.data?.logs ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    if (withLoading) loading.value = false;
  }
}

async function loadRules(withLoading = true) {
  if (withLoading) loading.value = true;
  error.value = "";
  try {
    const response = await apiRequest<{ rules: PointsRule[] }>("/admin/points/rules");
    rules.value = response.data?.rules ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    if (withLoading) loading.value = false;
  }
}

function resetFilters() {
  filters.userId = "";
  filters.reason = "";
  filters.limit = 100;
  void loadLogs();
}

function openCreateRule() {
  editingRuleId.value = "";
  ruleForm.code = "";
  ruleForm.description = "";
  ruleForm.delta = 0;
  ruleForm.enabled = true;
  ruleDialogOpen.value = true;
}

function openEditRule(rule: PointsRule) {
  editingRuleId.value = rule.id;
  ruleForm.code = rule.code;
  ruleForm.description = rule.description;
  ruleForm.delta = rule.delta;
  ruleForm.enabled = rule.enabled;
  ruleDialogOpen.value = true;
}

async function saveRule() {
  saving.value = true;
  message.value = "";
  error.value = "";
  try {
    const body = {
      code: ruleForm.code.trim(),
      description: ruleForm.description.trim(),
      delta: ruleForm.delta,
      enabled: ruleForm.enabled,
    };
    if (editingRuleId.value) {
      await apiRequest<{ rule: PointsRule }>(`/admin/points/rules/${editingRuleId.value}`, {
        method: "PATCH",
        body: JSON.stringify(body),
      });
    } else {
      await apiRequest<{ rule: PointsRule }>("/admin/points/rules", {
        method: "POST",
        body: JSON.stringify(body),
      });
    }
    message.value = copy.saveDone;
    ruleDialogOpen.value = false;
    await loadRules();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.saveFailed;
  } finally {
    saving.value = false;
  }
}

function userLabel(userId: string) {
  const user = usersByID.value[userId];
  return user ? `${user.email} - ${user.name}` : userId;
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
</script>
