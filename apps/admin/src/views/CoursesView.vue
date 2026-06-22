<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Courses</p>
        <h1>课程维护</h1>
        <p class="muted">课程是资料库的一级入口。当前只服务软件学院，不把院校筛选作为前台主路径。</p>
      </div>
      <el-button type="primary" @click="loadAll">刷新</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>新增课程</strong>
      </template>
      <p class="muted">组织字段用于入库关联，学生端展示时会弱化院校筛选。</p>
      <el-form class="form-grid" label-position="top">
        <el-form-item label="学校">
          <el-select v-model="form.schoolId" placeholder="选择学校" @change="form.collegeId = ''; form.majorId = ''">
            <el-option v-for="school in schools" :key="school.id" :label="school.name" :value="school.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="学院">
          <el-select v-model="form.collegeId" placeholder="选择学院" @change="form.majorId = ''">
            <el-option v-for="college in filteredColleges" :key="college.id" :label="college.name" :value="college.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="专业">
          <el-select v-model="form.majorId" placeholder="选择专业">
            <el-option v-for="major in filteredMajors" :key="major.id" :label="major.name" :value="major.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="年级">
          <el-input v-model="form.grade" placeholder="2023" />
        </el-form-item>
        <el-form-item label="课程名">
          <el-input v-model="form.name" placeholder="离散数学" />
        </el-form-item>
        <el-form-item label="Slug">
          <el-input v-model="form.slug" placeholder="discrete-math" />
        </el-form-item>
        <el-form-item class="wide" label="简介">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item class="wide" label="考试范围">
          <el-input v-model="form.examScope" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <el-button type="primary" :loading="loading" @click="createCourse">创建课程</el-button>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>课程列表</strong>
      </template>
      <el-table :data="courses" style="width: 100%">
        <el-table-column prop="name" label="课程" min-width="160" />
        <el-table-column prop="grade" label="年级" width="100" />
        <el-table-column prop="slug" label="Slug" min-width="160" />
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="archiveCourse(row.id)">归档</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import { apiRequest, type College, type Course, type Major, type School } from "../lib/api";

const schools = ref<School[]>([]);
const colleges = ref<College[]>([]);
const majors = ref<Major[]>([]);
const courses = ref<Course[]>([]);
const loading = ref(false);
const message = ref("");
const error = ref("");

const form = reactive({
  schoolId: "",
  collegeId: "",
  majorId: "",
  grade: "2023",
  name: "",
  slug: "",
  description: "",
  examScope: "",
  status: "published",
});

const filteredColleges = computed(() => colleges.value.filter((college) => !form.schoolId || college.schoolId === form.schoolId));
const filteredMajors = computed(() => majors.value.filter((major) => (!form.schoolId || major.schoolId === form.schoolId) && (!form.collegeId || major.collegeId === form.collegeId)));

onMounted(loadAll);

async function loadAll() {
  error.value = "";
  try {
    const [schoolResponse, collegeResponse, majorResponse, courseResponse] = await Promise.all([
      apiRequest<{ schools: School[] }>("/schools"),
      apiRequest<{ colleges: College[] }>("/colleges"),
      apiRequest<{ majors: Major[] }>("/majors"),
      apiRequest<{ courses: Course[] }>("/courses"),
    ]);
    schools.value = schoolResponse.data?.schools ?? [];
    colleges.value = collegeResponse.data?.colleges ?? [];
    majors.value = majorResponse.data?.majors ?? [];
    courses.value = courseResponse.data?.courses ?? [];
    if (!form.schoolId && schools.value[0]) form.schoolId = schools.value[0].id;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载失败";
  }
}

async function createCourse() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ course: Course }>("/admin/courses", {
      method: "POST",
      body: JSON.stringify(form),
    });
    message.value = "课程已创建。";
    form.name = "";
    form.slug = "";
    form.description = "";
    form.examScope = "";
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "创建失败";
  } finally {
    loading.value = false;
  }
}

async function archiveCourse(id: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ archived: boolean }>(`/admin/courses/${id}`, { method: "DELETE" });
    message.value = "课程已归档。";
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "归档失败";
  }
}
</script>
