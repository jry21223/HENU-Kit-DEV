<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Materials</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.upload }}</strong>
      </template>
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.course">
          <el-select v-model="uploadForm.courseId" :placeholder="copy.selectCourse">
            <el-option v-for="course in courses" :key="course.id" :label="`${course.name} - ${course.grade}`" :value="course.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="uploadForm.title" placeholder="Discrete math final notes" />
        </el-form-item>
        <el-form-item :label="copy.type">
          <el-select v-model="uploadForm.type">
            <el-option v-for="item in materialTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.access">
          <el-select v-model="uploadForm.accessLevel">
            <el-option v-for="item in accessLevels" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="uploadForm.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.preview">
          <el-input v-model="uploadForm.previewContent" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="copy.file">
          <input type="file" accept=".pdf,.txt,.md,.docx" @change="onFileChange" />
          <p class="muted">{{ copy.fileHint }}</p>
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="uploadMaterial">{{ copy.uploadAction }}</el-button>
        <span class="muted">{{ copy.uploadHint }}</span>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.list }}</strong>
      </template>
      <el-table v-loading="loading" :data="materials" empty-text="No materials" style="width: 100%">
        <el-table-column prop="title" :label="copy.name" min-width="180" />
        <el-table-column prop="type" :label="copy.type" width="140" />
        <el-table-column prop="accessLevel" :label="copy.access" width="130" />
        <el-table-column :label="copy.status" width="130">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.review" min-width="180">
          <template #default="{ row }">
            <p class="cell-title">{{ row.reviewReason || copy.noReviewReason }}</p>
            <p class="cell-muted">{{ formatDate(row.reviewedAt) }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="fileName" :label="copy.fileName" min-width="160" />
        <el-table-column :label="copy.actions" min-width="280" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openEdit(row)">{{ copy.edit }}</el-button>
              <el-button v-if="row.status !== 'pending'" size="small" @click="setStatus(row.id, 'pending')">{{ copy.markPending }}</el-button>
              <el-button v-if="row.status !== 'published'" size="small" type="success" plain @click="setStatus(row.id, 'published')">
                {{ copy.publish }}
              </el-button>
              <el-button v-if="row.status !== 'draft'" size="small" plain @click="setStatus(row.id, 'draft')">{{ copy.backToDraft }}</el-button>
              <el-button v-if="row.status !== 'archived'" size="small" type="danger" plain @click="archiveMaterial(row.id)">{{ copy.archive }}</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="editOpen" :title="copy.editTitle" width="min(760px, 92vw)">
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.course">
          <el-select v-model="editForm.courseId" :placeholder="copy.selectCourse">
            <el-option v-for="course in courses" :key="course.id" :label="`${course.name} - ${course.grade} - ${statusLabel(course.status)}`" :value="course.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="editForm.title" />
        </el-form-item>
        <el-form-item :label="copy.type">
          <el-select v-model="editForm.type">
            <el-option v-for="item in materialTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.access">
          <el-select v-model="editForm.accessLevel">
            <el-option v-for="item in accessLevels" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="editForm.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.descriptionLabel">
          <el-input v-model="editForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item class="wide" :label="copy.preview">
          <el-input v-model="editForm.previewContent" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="editOpen = false">{{ copy.cancel }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveMaterial">{{ copy.save }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type Course, type Material } from "../lib/api";

type MaterialForm = {
  id: string;
  courseId: string;
  title: string;
  type: string;
  description: string;
  previewContent: string;
  accessLevel: string;
  status: string;
};

const copy = {
  title: "\u0050\u0044\u0046 \u8d44\u6599",
  description:
    "\u4e0a\u4f20\u8bfe\u7a0b PDF \u8d44\u6599\u5e76\u4fdd\u6301\u7c7b\u578b\u3001\u6743\u9650\u548c\u9884\u89c8\u5185\u5bb9\u6e05\u6670\u3002\u8d44\u6599\u9ed8\u8ba4\u4ee5 draft \u5165\u5e93\uff0c\u53ea\u6709 published \u72b6\u6001\u4f1a\u5728\u524d\u53f0\u5c55\u793a\u5e76\u8fdb\u5165\u4e0b\u8f7d\u6743\u9650\u5224\u65ad\u3002",
  refresh: "\u5237\u65b0",
  upload: "\u4e0a\u4f20 PDF \u8d44\u6599",
  course: "\u8bfe\u7a0b",
  selectCourse: "\u9009\u62e9\u8bfe\u7a0b",
  name: "\u6807\u9898",
  type: "\u7c7b\u578b",
  access: "\u6743\u9650",
  status: "\u72b6\u6001",
  preview: "\u9884\u89c8\u5185\u5bb9",
  descriptionLabel: "\u8d44\u6599\u8bf4\u660e",
  file: "\u6587\u4ef6",
  fileHint: "\u4f18\u5148\u4e0a\u4f20 PDF\uff0c\u4ecd\u517c\u5bb9 TXT / MD / DOCX\u3002",
  fileName: "\u6587\u4ef6\u540d",
  review: "\u5ba1\u6838\u8bb0\u5f55",
  noReviewReason: "\u5c1a\u65e0\u5ba1\u6838\u610f\u89c1",
  notReviewed: "\u672a\u5ba1\u6838",
  uploadAction: "\u4e0a\u4f20\u5e76\u5165\u5e93",
  uploadHint: "\u672a\u9009\u72b6\u6001\u65f6\u670d\u52a1\u7aef\u4e5f\u4f1a\u9ed8\u8ba4\u4e3a draft\u3002",
  list: "\u8d44\u6599\u5217\u8868",
  actions: "\u64cd\u4f5c",
  edit: "\u7f16\u8f91",
  editTitle: "\u7f16\u8f91\u8d44\u6599",
  cancel: "\u53d6\u6d88",
  save: "\u4fdd\u5b58",
  markPending: "\u63d0\u4ea4\u5ba1\u6838",
  publish: "\u53d1\u5e03",
  backToDraft: "\u9000\u56de\u8349\u7a3f",
  archive: "\u5f52\u6863",
  chooseFile: "\u8bf7\u9009\u62e9\u6587\u4ef6\u3002",
  loadFailed: "\u52a0\u8f7d\u5931\u8d25",
  uploadDone: "\u8d44\u6599\u5df2\u4e0a\u4f20\u3002",
  uploadFailed: "\u4e0a\u4f20\u5931\u8d25",
  updateDone: "\u8d44\u6599\u5df2\u66f4\u65b0\u3002",
  updateFailed: "\u8d44\u6599\u66f4\u65b0\u5931\u8d25",
  statusUpdated: "\u8d44\u6599\u72b6\u6001\u5df2\u66f4\u65b0\u3002",
  statusFailed: "\u72b6\u6001\u66f4\u65b0\u5931\u8d25",
  archived: "\u8d44\u6599\u5df2\u5f52\u6863\u3002",
  archiveFailed: "\u5f52\u6863\u5931\u8d25",
};

const statuses = [
  { label: "\u8349\u7a3f", value: "draft" },
  { label: "\u5f85\u5ba1\u6838", value: "pending" },
  { label: "\u5df2\u53d1\u5e03", value: "published" },
  { label: "\u5df2\u9a73\u56de", value: "rejected" },
  { label: "\u5df2\u5f52\u6863", value: "archived" },
];

const materialTypes = [
  { label: "\u77e5\u8bc6\u70b9\u8bb2\u4e49", value: "knowledge_note" },
  { label: "\u6a21\u62df\u5377", value: "mock_paper" },
  { label: "\u7b54\u6848\u89e3\u6790", value: "answer" },
  { label: "\u8003\u524d\u901f\u80cc", value: "quick_review" },
  { label: "\u5386\u5e74\u771f\u9898", value: "past_exam" },
  { label: "\u5176\u4ed6", value: "other" },
];

const accessLevels = [
  { label: "\u514d\u8d39", value: "free" },
  { label: "\u767b\u5f55\u53ef\u89c1", value: "login_required" },
  { label: "\u4ed8\u8d39", value: "paid" },
  { label: "\u4f1a\u5458", value: "member_only" },
];

const courses = ref<Course[]>([]);
const materials = ref<Material[]>([]);
const file = ref<File | null>(null);
const loading = ref(false);
const saving = ref(false);
const editOpen = ref(false);
const message = ref("");
const error = ref("");

const uploadForm = reactive({
  courseId: "",
  title: "",
  type: "knowledge_note",
  accessLevel: "login_required",
  status: "draft",
  previewContent: "",
});
const editForm = reactive<MaterialForm>(emptyMaterialForm());

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [courseResponse, materialResponse] = await Promise.all([
      apiRequest<{ courses: Course[] }>("/admin/courses"),
      apiRequest<{ materials: Material[] }>("/admin/materials"),
    ]);
    courses.value = courseResponse.data?.courses ?? [];
    materials.value = materialResponse.data?.materials ?? [];
    if (!uploadForm.courseId && courses.value[0]) uploadForm.courseId = courses.value[0].id;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  file.value = input.files?.[0] ?? null;
}

async function uploadMaterial() {
  if (!file.value) {
    error.value = copy.chooseFile;
    return;
  }
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const body = new FormData();
    body.set("courseId", uploadForm.courseId);
    body.set("title", uploadForm.title);
    body.set("type", uploadForm.type);
    body.set("accessLevel", uploadForm.accessLevel);
    body.set("status", uploadForm.status);
    body.set("previewContent", uploadForm.previewContent);
    body.set("file", file.value);
    await apiRequest<{ material: Material }>("/admin/materials/upload", { method: "POST", body });
    message.value = copy.uploadDone;
    uploadForm.title = "";
    uploadForm.previewContent = "";
    uploadForm.status = "draft";
    file.value = null;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.uploadFailed;
  } finally {
    loading.value = false;
  }
}

function openEdit(material: Material) {
  Object.assign(editForm, {
    id: material.id,
    courseId: material.courseId,
    title: material.title,
    type: material.type,
    description: material.description,
    previewContent: material.previewContent,
    accessLevel: material.accessLevel,
    status: material.status,
  });
  editOpen.value = true;
}

async function saveMaterial() {
  if (!editForm.id) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ updated: boolean }>(`/admin/materials/${editForm.id}`, {
      method: "PATCH",
      body: JSON.stringify(materialPayload(editForm)),
    });
    message.value = copy.updateDone;
    editOpen.value = false;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.updateFailed;
  } finally {
    saving.value = false;
  }
}

async function setStatus(id: string, status: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ updated: boolean; status: string }>(`/admin/materials/${id}/status`, {
      method: "PATCH",
      body: JSON.stringify({ status }),
    });
    message.value = copy.statusUpdated;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.statusFailed;
  }
}

async function archiveMaterial(id: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ archived: boolean }>(`/admin/materials/${id}`, { method: "DELETE" });
    message.value = copy.archived;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.archiveFailed;
  }
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "published") return "success";
  if (status === "pending") return "warning";
  if (status === "rejected") return "danger";
  if (status === "archived") return "info";
  return "";
}

function formatDate(value?: string) {
  if (!value) return copy.notReviewed;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
}

function materialPayload(source: MaterialForm) {
  return {
    courseId: source.courseId,
    title: source.title,
    type: source.type,
    description: source.description,
    previewContent: source.previewContent,
    accessLevel: source.accessLevel,
    status: source.status,
  };
}

function emptyMaterialForm(): MaterialForm {
  return {
    id: "",
    courseId: "",
    title: "",
    type: "knowledge_note",
    description: "",
    previewContent: "",
    accessLevel: "login_required",
    status: "draft",
  };
}
</script>
