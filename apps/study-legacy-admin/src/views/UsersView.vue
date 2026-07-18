<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Users</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadUsers">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.filters }}</strong>
      </template>
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.email">
          <el-input v-model="filters.email" clearable placeholder="student@stu.henu.edu.cn" @keyup.enter="loadUsers" />
        </el-form-item>
        <el-form-item :label="copy.role">
          <el-select v-model="filters.role" clearable :placeholder="copy.allRoles">
            <el-option
              v-for="item in roleOptions"
              :key="item.value"
              :disabled="item.value === 'super_admin' && auth.user?.role !== 'super_admin'"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="filters.status" clearable :placeholder="copy.allStatuses">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button :loading="loading" @click="loadUsers">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.listTitle }}</strong>
      </template>
      <el-table v-loading="loading" :data="users" empty-text="No users" style="width: 100%">
        <el-table-column :label="copy.user" min-width="240">
          <template #default="{ row }">
            <strong>{{ row.name }}</strong>
            <p class="cell-muted">{{ row.email }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.role" width="140">
          <template #default="{ row }">
            <el-tag :type="roleTag(row.role)">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.profile" min-width="180">
          <template #default="{ row }">
            <span>{{ row.grade || copy.empty }}</span>
            <p class="cell-muted">{{ row.schoolId || copy.unbound }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="pointsBalance" :label="copy.points" width="100" />
        <el-table-column :label="copy.verified" width="110">
          <template #default="{ row }">
            <el-tag :type="row.emailVerified ? 'success' : 'warning'">
              {{ row.emailVerified ? copy.yes : copy.no }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" min-width="160">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="copy.actions" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">{{ copy.edit }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="editOpen" :title="copy.editTitle" width="min(560px, 92vw)">
      <el-alert v-if="sensitiveNotice" class="notice" type="warning" :closable="false" :title="sensitiveNotice" />
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.email">
          <el-input v-model="editForm.email" disabled />
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="editForm.name" maxlength="80" />
        </el-form-item>
        <el-form-item :label="copy.role">
          <el-select v-model="editForm.role" :disabled="disableSensitiveEdit">
            <el-option v-for="item in roleOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="editForm.status" :disabled="disableSensitiveEdit">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="editOpen = false">{{ copy.cancel }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveUser">{{ copy.save }}</el-button>
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
import { apiRequest, type User } from "../lib/api";
import { useAuthStore } from "../stores/auth";

type UserEditForm = {
  id: string;
  email: string;
  name: string;
  role: string;
  originalRole: string;
  status: string;
};

const copy = {
  title: "\u7528\u6237\u7ba1\u7406",
  description:
    "\u7ba1\u7406\u5b66\u751f\u3001\u5ba1\u6838\u5458\u548c\u7ba1\u7406\u5458\u7684\u57fa\u7840\u72b6\u6001\u3002\u89d2\u8272\u548c\u51bb\u7ed3\u64cd\u4f5c\u5747\u7531 Go API \u670d\u52a1\u7aef\u6821\u9a8c\u3002",
  refresh: "\u5237\u65b0",
  filters: "\u7b5b\u9009",
  email: "\u90ae\u7bb1",
  role: "\u89d2\u8272",
  status: "\u72b6\u6001",
  allRoles: "\u5168\u90e8\u89d2\u8272",
  allStatuses: "\u5168\u90e8\u72b6\u6001",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  listTitle: "\u7528\u6237\u5217\u8868",
  user: "\u7528\u6237",
  profile: "\u7ed1\u5b9a\u4fe1\u606f",
  points: "\u79ef\u5206",
  verified: "\u90ae\u7bb1\u9a8c\u8bc1",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  actions: "\u64cd\u4f5c",
  edit: "\u7f16\u8f91",
  editTitle: "\u7f16\u8f91\u7528\u6237",
  name: "\u6635\u79f0",
  cancel: "\u53d6\u6d88",
  save: "\u4fdd\u5b58",
  empty: "-",
  unbound: "\u672a\u7ed1\u5b9a\u5b66\u6821",
  yes: "\u5df2\u9a8c\u8bc1",
  no: "\u672a\u9a8c\u8bc1",
  loadFailed: "\u7528\u6237\u52a0\u8f7d\u5931\u8d25",
  updateDone: "\u7528\u6237\u5df2\u66f4\u65b0\u3002",
  updateFailed: "\u7528\u6237\u66f4\u65b0\u5931\u8d25",
  selfNotice: "\u5f53\u524d\u767b\u5f55\u8d26\u53f7\u4e0d\u80fd\u5728\u6b64\u4fee\u6539\u81ea\u5df1\u7684\u89d2\u8272\u6216\u72b6\u6001\u3002",
  superNotice: "\u975e super_admin \u4e0d\u80fd\u4fee\u6539 super_admin \u8d26\u53f7\u6216\u6388\u4e88 super_admin\u3002",
};

const roleOptions = [
  { label: "user", value: "user" },
  { label: "creator", value: "creator" },
  { label: "reviewer", value: "reviewer" },
  { label: "operator", value: "operator" },
  { label: "admin", value: "admin" },
  { label: "super_admin", value: "super_admin" },
];

const statusOptions = [
  { label: "\u6b63\u5e38", value: "active" },
  { label: "\u51bb\u7ed3", value: "frozen" },
];

const auth = useAuthStore();
const users = ref<User[]>([]);
const loading = ref(false);
const saving = ref(false);
const editOpen = ref(false);
const message = ref("");
const error = ref("");
const filters = reactive({
  email: "",
  role: "",
  status: "",
});
const editForm = reactive<UserEditForm>({
  id: "",
  email: "",
  name: "",
  role: "user",
  originalRole: "user",
  status: "active",
});

const editingSelf = computed(() => editForm.id !== "" && editForm.id === auth.user?.id);
const editingSuperAdmin = computed(() => editForm.originalRole === "super_admin" && auth.user?.role !== "super_admin");
const disableSensitiveEdit = computed(() => editingSelf.value || editingSuperAdmin.value);
const sensitiveNotice = computed(() => {
  if (editingSelf.value) return copy.selfNotice;
  if (editingSuperAdmin.value) return copy.superNotice;
  return "";
});

onMounted(loadUsers);

async function loadUsers() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.email.trim()) params.set("email", filters.email.trim());
    if (filters.role) params.set("role", filters.role);
    if (filters.status) params.set("status", filters.status);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const response = await apiRequest<{ users: User[] }>(`/admin/users${suffix}`);
    users.value = response.data?.users ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.email = "";
  filters.role = "";
  filters.status = "";
  void loadUsers();
}

function openEdit(user: User) {
  editForm.id = user.id;
  editForm.email = user.email;
  editForm.name = user.name;
  editForm.role = user.role;
  editForm.originalRole = user.role;
  editForm.status = user.status || "active";
  editOpen.value = true;
  message.value = "";
  error.value = "";
}

async function saveUser() {
  if (!editForm.id) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    const body: Record<string, string> = {
      name: editForm.name,
    };
    if (!disableSensitiveEdit.value) {
      body.role = editForm.role;
      body.status = editForm.status;
    }
    const response = await apiRequest<{ user: User }>(`/admin/users/${editForm.id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
    const updated = response.data?.user;
    if (updated) {
      users.value = users.value.map((item) => (item.id === updated.id ? updated : item));
      if (auth.user?.id === updated.id) {
        auth.setUser(updated);
      }
    }
    message.value = copy.updateDone;
    editOpen.value = false;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.updateFailed;
  } finally {
    saving.value = false;
  }
}

function roleLabel(role: string) {
  return roleOptions.find((item) => item.value === role)?.label ?? role;
}

function statusLabel(status: string) {
  return statusOptions.find((item) => item.value === status)?.label ?? status;
}

function roleTag(role: string) {
  if (role === "super_admin" || role === "admin") return "danger";
  if (role === "reviewer" || role === "operator") return "warning";
  if (role === "creator") return "success";
  return "info";
}

function statusTag(status: string) {
  return status === "frozen" ? "danger" : "success";
}

function formatDate(value?: string) {
  if (!value) return copy.empty;
  return new Date(value).toLocaleString();
}
</script>
