<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Materials</p>
        <h1>资料管理</h1>
        <p class="muted">资料文件必须通过服务端上传或安全 storageKey 入库，前台下载仍走权限检查。</p>
      </div>
      <el-button type="primary" @click="loadAll">刷新</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>上传资料</strong>
      </template>
      <el-form class="form-grid" label-position="top">
        <el-form-item label="课程">
          <el-select v-model="uploadForm.courseId" placeholder="选择课程">
            <el-option v-for="course in courses" :key="course.id" :label="`${course.name} · ${course.grade}`" :value="course.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="uploadForm.title" placeholder="离散数学重点讲义" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="uploadForm.type">
            <el-option label="知识点讲义" value="knowledge_note" />
            <el-option label="模拟卷" value="mock_paper" />
            <el-option label="答案解析" value="answer" />
            <el-option label="考前速背" value="quick_review" />
            <el-option label="历年真题" value="past_exam" />
            <el-option label="其他" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item label="权限">
          <el-select v-model="uploadForm.accessLevel">
            <el-option label="免费" value="free" />
            <el-option label="登录可见" value="login_required" />
            <el-option label="付费" value="paid" />
            <el-option label="会员" value="member_only" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" label="预览内容">
          <el-input v-model="uploadForm.previewContent" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="文件">
          <input type="file" accept=".pdf,.txt,.md,.docx" @change="onFileChange" />
        </el-form-item>
      </el-form>
      <el-button type="primary" :loading="loading" @click="uploadMaterial">上传并入库</el-button>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>资料列表</strong>
      </template>
      <el-table :data="materials" style="width: 100%">
        <el-table-column prop="title" label="标题" min-width="180" />
        <el-table-column prop="type" label="类型" width="130" />
        <el-table-column prop="accessLevel" label="权限" width="130" />
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="fileName" label="文件名" min-width="160" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="archiveMaterial(row.id)">归档</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type Course, type Material } from "../lib/api";

const courses = ref<Course[]>([]);
const materials = ref<Material[]>([]);
const file = ref<File | null>(null);
const loading = ref(false);
const message = ref("");
const error = ref("");

const uploadForm = reactive({
  courseId: "",
  title: "",
  type: "knowledge_note",
  accessLevel: "login_required",
  status: "published",
  previewContent: "",
});

onMounted(loadAll);

async function loadAll() {
  error.value = "";
  try {
    const [courseResponse, materialResponse] = await Promise.all([
      apiRequest<{ courses: Course[] }>("/courses"),
      apiRequest<{ materials: Material[] }>("/materials"),
    ]);
    courses.value = courseResponse.data?.courses ?? [];
    materials.value = materialResponse.data?.materials ?? [];
    if (!uploadForm.courseId && courses.value[0]) uploadForm.courseId = courses.value[0].id;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载失败";
  }
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  file.value = input.files?.[0] ?? null;
}

async function uploadMaterial() {
  if (!file.value) {
    error.value = "请选择文件。";
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
    message.value = "资料已上传。";
    uploadForm.title = "";
    uploadForm.previewContent = "";
    file.value = null;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "上传失败";
  } finally {
    loading.value = false;
  }
}

async function archiveMaterial(id: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ archived: boolean }>(`/admin/materials/${id}`, { method: "DELETE" });
    message.value = "资料已归档。";
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "归档失败";
  }
}
</script>
