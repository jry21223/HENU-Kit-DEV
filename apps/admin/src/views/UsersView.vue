<template>
  <AdminShellV2 title="用户与会话" environment="runtime">
    <div class="space-y-6">
      <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"><div><p class="text-xs font-semibold uppercase tracking-[.16em] text-primary">users</p><h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ copy.title }}</h1><p class="mt-2 text-sm text-muted-foreground">{{ copy.description }}</p></div><Button variant="outline" :disabled="loading" @click="loadUsers">{{ copy.refresh }}</Button></header>
      <Alert v-if="message" class="border-emerald-200 bg-emerald-50 text-emerald-800">{{ message }}</Alert><Alert v-if="error" variant="destructive">{{ error }}</Alert>
      <Card><CardHeader><CardTitle>{{ copy.filters }}</CardTitle><CardDescription>筛选条件会写入 URL 查询对应的服务端过滤。</CardDescription></CardHeader><CardContent><form class="grid gap-3 md:grid-cols-3" @submit.prevent="loadUsers"><label class="grid gap-1.5 text-xs font-medium">{{ copy.email }}<Input v-model="filters.email" placeholder="student@stu.henu.edu.cn" /></label><label class="grid gap-1.5 text-xs font-medium">{{ copy.role }}<select v-model="filters.role" class="h-10 rounded-md border border-input bg-background px-3 text-sm"><option value="">{{ copy.allRoles }}</option><option v-for="item in roleOptions" :key="item.value" :disabled="item.value === 'super_admin' && auth.user?.role !== 'super_admin'" :value="item.value">{{ item.label }}</option></select></label><label class="grid gap-1.5 text-xs font-medium">{{ copy.status }}<select v-model="filters.status" class="h-10 rounded-md border border-input bg-background px-3 text-sm"><option value="">{{ copy.allStatuses }}</option><option v-for="item in statusOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label><div class="flex gap-2 md:col-span-3"><Button type="submit" :disabled="loading">{{ copy.apply }}</Button><Button type="button" variant="outline" @click="resetFilters">{{ copy.reset }}</Button></div></form></CardContent></Card>
      <Card><CardHeader><CardTitle>{{ copy.listTitle }}</CardTitle><CardDescription>共 {{ users.length }} 名用户；角色、冻结与 Session 撤销均由服务端权限和乐观锁校验。</CardDescription></CardHeader><CardContent class="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>{{ copy.user }}</TableHead><TableHead>{{ copy.role }}</TableHead><TableHead>{{ copy.status }}</TableHead><TableHead>{{ copy.profile }}</TableHead><TableHead>{{ copy.points }}</TableHead><TableHead>{{ copy.verified }}</TableHead><TableHead>{{ copy.createdAt }}</TableHead><TableHead>{{ copy.actions }}</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="row in users" :key="row.id"><TableCell><strong>{{ row.name }}</strong><p class="text-xs text-muted-foreground">{{ row.email }}</p></TableCell><TableCell><Badge :variant="row.role.includes('admin') ? 'destructive' : 'secondary'">{{ roleLabel(row.role) }}</Badge></TableCell><TableCell><Badge :variant="row.status === 'frozen' ? 'destructive' : 'success'">{{ statusLabel(row.status) }}</Badge></TableCell><TableCell>{{ row.grade || copy.empty }}<p class="text-xs text-muted-foreground">{{ row.schoolId || copy.unbound }}</p></TableCell><TableCell>{{ row.pointsBalance }}</TableCell><TableCell>{{ row.emailVerified ? copy.yes : copy.no }}</TableCell><TableCell>{{ formatDate(row.createdAt) }}</TableCell><TableCell><div class="flex gap-2"><Button size="sm" variant="outline" @click="openEdit(row)">{{ copy.edit }}</Button><Button size="sm" variant="destructive" :disabled="row.id === auth.user?.id" @click="revokeSessions(row)">{{ copy.revokeSessions }}</Button></div></TableCell></TableRow><TableRow v-if="!users.length"><TableCell colspan="8" class="py-10 text-center text-muted-foreground">暂无符合条件的用户</TableCell></TableRow></TableBody></Table></CardContent></Card>
      <Card v-if="editOpen"><CardHeader><CardTitle>{{ copy.editTitle }}</CardTitle><CardDescription>{{ editForm.email }}</CardDescription></CardHeader><CardContent class="grid gap-4"><Alert v-if="sensitiveNotice" class="border-amber-200 bg-amber-50 text-amber-800">{{ sensitiveNotice }}</Alert><div class="grid gap-3 md:grid-cols-3"><label class="grid gap-1.5 text-xs font-medium">{{ copy.name }}<Input v-model="editForm.name" maxlength="80" /></label><label class="grid gap-1.5 text-xs font-medium">{{ copy.role }}<select v-model="editForm.role" :disabled="disableSensitiveEdit" class="h-10 rounded-md border border-input bg-background px-3 text-sm"><option v-for="item in roleOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label><label class="grid gap-1.5 text-xs font-medium">{{ copy.status }}<select v-model="editForm.status" :disabled="disableSensitiveEdit" class="h-10 rounded-md border border-input bg-background px-3 text-sm"><option v-for="item in statusOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label></div><div class="flex justify-end gap-2"><Button variant="outline" @click="editOpen=false">{{ copy.cancel }}</Button><Button :disabled="saving" @click="saveUser">{{ copy.save }}</Button></div></CardContent></Card>
    </div>
  </AdminShellV2>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import AdminShellV2 from "../components/AdminShellV2.vue";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
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
	revokeSessions: "\u64a4\u9500 Session",
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
	revokeDone: "\u8be5\u7528\u6237\u7684\u5df2\u7b7e\u53d1 Session \u5df2\u5168\u90e8\u64a4\u9500\u3002",
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

async function revokeSessions(user: User) {
	loading.value = true;
	error.value = "";
	message.value = "";
	try {
		const response = await apiRequest<{ user_id: string; version: number; sessions_revoked: boolean }>(`/admin/users/${user.id}/sessions/revoke`, {
			method: "POST",
			headers: { "Idempotency-Key": crypto.randomUUID() },
			body: JSON.stringify({ expected_version: user.version }),
		});
		users.value = users.value.map((item) => item.id === user.id ? { ...item, version: response.data?.version ?? item.version } : item);
		message.value = copy.revokeDone;
	} catch (err) {
		error.value = err instanceof Error ? err.message : copy.updateFailed;
	} finally {
		loading.value = false;
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
