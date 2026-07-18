<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Access Grants</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.createTitle }}</strong>
      </template>
      <p class="muted">{{ copy.createDescription }}</p>
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.user">
          <el-select v-model="form.userId" filterable :placeholder="copy.selectUser">
            <el-option v-for="user in users" :key="user.id" :label="`${user.email} - ${user.name}`" :value="user.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.resourceType">
          <el-segmented v-model="form.resourceType" :options="resourceTypeOptions" />
        </el-form-item>
        <el-form-item v-if="form.resourceType === 'material'" :label="copy.material">
          <el-select v-model="form.materialId" filterable :placeholder="copy.selectMaterial">
            <el-option
              v-for="material in grantableMaterials"
              :key="material.id"
              :label="`${material.title} - ${material.accessLevel}`"
              :value="material.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-else :label="copy.package">
          <el-select v-model="form.packageId" filterable :placeholder="copy.selectPackage">
            <el-option
              v-for="coursePackage in packages"
              :key="coursePackage.id"
              :label="`${coursePackage.title} - ${formatPrice(coursePackage.priceFen, coursePackage.currency)}`"
              :value="coursePackage.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.expiresAt">
          <el-input v-model="form.expiresAt" clearable :placeholder="copy.expiresPlaceholder" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="saving" @click="createGrant">{{ copy.createAction }}</el-button>
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
        <el-form-item :label="copy.active">
          <el-select v-model="filters.active" clearable :placeholder="copy.allStatus">
            <el-option :label="copy.activeOnly" value="true" />
            <el-option :label="copy.expiredOnly" value="false" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button :loading="loading" @click="loadGrants">{{ copy.apply }}</el-button>
        <el-button @click="resetFilters">{{ copy.reset }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.listTitle }}</strong>
      </template>
      <el-table v-loading="loading" :data="grants" empty-text="No grants" style="width: 100%">
        <el-table-column :label="copy.user" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.user?.email || row.grant.userId }}</strong>
            <p class="cell-muted">{{ row.user?.name || copy.unknown }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.resource" min-width="260">
          <template #default="{ row }">
            <strong>{{ resourceTitle(row) }}</strong>
            <p class="cell-muted">{{ resourceMeta(row) }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="grant.source" :label="copy.source" width="130" />
        <el-table-column :label="copy.active" width="110">
          <template #default="{ row }">
            <el-tag :type="row.active ? 'success' : 'info'">{{ row.active ? copy.activeYes : copy.activeNo }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.expiresAt" width="180">
          <template #default="{ row }">{{ row.grant.expiresAt ? formatDate(row.grant.expiresAt) : copy.neverExpires }}</template>
        </el-table-column>
        <el-table-column :label="copy.createdAt" width="180">
          <template #default="{ row }">{{ formatDate(row.grant.createdAt) }}</template>
        </el-table-column>
        <el-table-column :label="copy.actions" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="revokeGrant(row)">{{ copy.revoke }}</el-button>
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
import { computed, onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type AccessGrantRow, type CoursePackage, type Material, type User } from "../lib/api";

const copy = {
  title: "\u6743\u76ca\u6388\u6743",
  description:
    "\u7528\u4e8e\u5185\u6d4b\u548c\u552e\u540e\u7684\u624b\u52a8\u8d44\u6599\u4ea4\u4ed8\u3002\u8fd9\u91cc\u4e0d\u521b\u5efa\u652f\u4ed8\u8ba2\u5355\uff0c\u4e5f\u4e0d\u4f2a\u9020\u652f\u4ed8\u6210\u529f\u3002",
  refresh: "\u5237\u65b0",
  createTitle: "\u65b0\u589e\u624b\u52a8\u6388\u6743",
  createDescription:
    "\u53ea\u80fd\u6388\u6743 published \u8bfe\u7a0b\u5305\uff0c\u6216 published \u4e14 access_level \u4e3a paid/member_only \u7684\u8d44\u6599\u3002\u91cd\u590d\u6388\u6743\u4f1a\u8fd4\u56de\u73b0\u6709\u6709\u6548\u8bb0\u5f55\u3002",
  user: "\u7528\u6237",
  selectUser: "\u9009\u62e9\u7528\u6237",
  resourceType: "\u8d44\u6e90\u7c7b\u578b",
  material: "\u8d44\u6599",
  package: "\u8bfe\u7a0b\u5305",
  selectMaterial: "\u9009\u62e9 paid/member_only \u8d44\u6599",
  selectPackage: "\u9009\u62e9\u8bfe\u7a0b\u5305",
  expiresAt: "\u8fc7\u671f\u65f6\u95f4",
  expiresPlaceholder: "2026-07-01 or 2026-07-01T00:00:00Z",
  createAction: "\u53d1\u653e\u6388\u6743",
  filters: "\u7b5b\u9009",
  allUsers: "\u5168\u90e8\u7528\u6237",
  active: "\u6709\u6548\u72b6\u6001",
  allStatus: "\u5168\u90e8",
  activeOnly: "\u4ec5\u6709\u6548",
  expiredOnly: "\u4ec5\u8fc7\u671f",
  apply: "\u5e94\u7528\u7b5b\u9009",
  reset: "\u91cd\u7f6e",
  listTitle: "\u6388\u6743\u5217\u8868",
  resource: "\u8d44\u6e90",
  source: "\u6765\u6e90",
  activeYes: "\u6709\u6548",
  activeNo: "\u8fc7\u671f",
  neverExpires: "\u6c38\u4e0d\u8fc7\u671f",
  createdAt: "\u521b\u5efa\u65f6\u95f4",
  actions: "\u64cd\u4f5c",
  revoke: "\u64a4\u9500",
  unknown: "\u672a\u5339\u914d",
  materialResource: "\u76f4\u63a5\u8d44\u6599",
  packageResource: "\u8bfe\u7a0b\u5305",
  createDone: "\u6388\u6743\u5df2\u53d1\u653e\u3002",
  alreadyGranted: "\u5df2\u5b58\u5728\u6709\u6548\u6388\u6743\uff0c\u672a\u91cd\u590d\u521b\u5efa\u3002",
  createFailed: "\u6388\u6743\u5931\u8d25",
  revokeConfirm: "\u64a4\u9500\u540e\uff0c\u7528\u6237\u5c06\u5931\u53bb\u8be5\u8d44\u6e90\u7684 paid \u8bbf\u95ee\u6743\u3002",
  revokeTitle: "\u786e\u8ba4\u64a4\u9500\u6388\u6743\uff1f",
  revokeDone: "\u6388\u6743\u5df2\u64a4\u9500\u3002",
  revokeFailed: "\u64a4\u9500\u5931\u8d25",
  loadFailed: "\u6388\u6743\u6570\u636e\u52a0\u8f7d\u5931\u8d25",
};

const resourceTypeOptions = [
  { label: copy.material, value: "material" },
  { label: copy.package, value: "package" },
];

const users = ref<User[]>([]);
const materials = ref<Material[]>([]);
const packages = ref<CoursePackage[]>([]);
const grants = ref<AccessGrantRow[]>([]);
const loading = ref(false);
const saving = ref(false);
const message = ref("");
const error = ref("");
const filters = reactive({
  userId: "",
  active: "true",
});
const form = reactive({
  userId: "",
  resourceType: "material",
  materialId: "",
  packageId: "",
  expiresAt: "",
});

const grantableMaterials = computed(() =>
  materials.value.filter((item) => item.status === "published" && (item.accessLevel === "paid" || item.accessLevel === "member_only")),
);

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [usersResponse, materialsResponse, packagesResponse] = await Promise.all([
      apiRequest<{ users: User[] }>("/admin/users?limit=500"),
      apiRequest<{ materials: Material[] }>("/admin/materials?status=published"),
      apiRequest<{ packages: CoursePackage[] }>("/packages"),
    ]);
    users.value = usersResponse.data?.users ?? [];
    materials.value = materialsResponse.data?.materials ?? [];
    packages.value = packagesResponse.data?.packages ?? [];
    if (!form.userId && users.value[0]) form.userId = users.value[0].id;
    await loadGrants();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function loadGrants() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (filters.userId) params.set("userId", filters.userId);
    if (filters.active) params.set("active", filters.active);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const response = await apiRequest<{ grants: AccessGrantRow[] }>(`/admin/access-grants${suffix}`);
    grants.value = response.data?.grants ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function createGrant() {
  saving.value = true;
  message.value = "";
  error.value = "";
  try {
    const body: Record<string, string> = {
      userId: form.userId,
    };
    if (form.resourceType === "material") {
      body.materialId = form.materialId;
    } else {
      body.packageId = form.packageId;
    }
    if (form.expiresAt.trim()) {
      body.expiresAt = form.expiresAt.trim();
    }
    const response = await apiRequest<{ grant: unknown; alreadyGranted: boolean }>("/admin/access-grants", {
      method: "POST",
      body: JSON.stringify(body),
    });
    message.value = response.data?.alreadyGranted ? copy.alreadyGranted : copy.createDone;
    await loadGrants();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.createFailed;
  } finally {
    saving.value = false;
  }
}

function resetFilters() {
  filters.userId = "";
  filters.active = "true";
  void loadGrants();
}

async function revokeGrant(row: AccessGrantRow) {
  try {
    await ElMessageBox.confirm(copy.revokeConfirm, copy.revokeTitle, {
      confirmButtonText: copy.revoke,
      cancelButtonText: "\u53d6\u6d88",
      type: "warning",
    });
  } catch {
    return;
  }
  message.value = "";
  error.value = "";
  try {
    await apiRequest<{ revoked: boolean }>(`/admin/access-grants/${row.grant.id}`, { method: "DELETE" });
    message.value = copy.revokeDone;
    await loadGrants();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.revokeFailed;
  }
}

function resourceTitle(row: AccessGrantRow) {
  if (row.material) return row.material.title;
  if (row.package) return row.package.title;
  return row.grant.materialId || row.grant.packageId || copy.unknown;
}

function resourceMeta(row: AccessGrantRow) {
  if (row.material) return `${copy.materialResource} - ${row.material.accessLevel} - ${row.material.id}`;
  if (row.package) return `${copy.packageResource} - ${formatPrice(row.package.priceFen, row.package.currency)} - ${row.package.id}`;
  return copy.unknown;
}

function formatPrice(priceFen: number, currency = "CNY") {
  return `${currency} ${(priceFen / 100).toFixed(2)}`;
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
</script>
