<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Courses</p>
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
      <el-form class="form-grid form-stack" label-position="top">
        <el-form-item :label="copy.school">
          <el-select v-model="form.schoolId" :placeholder="copy.selectSchool" @change="onCreateSchoolChange">
            <el-option v-for="school in schools" :key="school.id" :label="school.name" :value="school.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.college">
          <el-select v-model="form.collegeId" :placeholder="copy.selectCollege" @change="onCreateCollegeChange">
            <el-option v-for="college in filteredColleges(form.schoolId)" :key="college.id" :label="college.name" :value="college.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.major">
          <el-select v-model="form.majorId" :placeholder="copy.selectMajor">
            <el-option v-for="major in filteredMajors(form.schoolId, form.collegeId)" :key="major.id" :label="major.name" :value="major.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.grade">
          <el-input v-model="form.grade" placeholder="2023" />
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="form.name" placeholder="Discrete Math" />
        </el-form-item>
        <el-form-item label="Slug">
          <el-input v-model="form.slug" placeholder="discrete-math" />
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="form.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.courseDescription">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item class="wide" :label="copy.examScope">
          <el-input v-model="form.examScope" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="createCourse">{{ copy.createAction }}</el-button>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.listTitle }}</strong>
      </template>
      <el-table v-loading="loading" :data="courses" empty-text="No courses" style="width: 100%">
        <el-table-column :label="copy.course" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.name }}</strong>
            <p class="cell-muted">{{ row.slug }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="grade" :label="copy.grade" width="100" />
        <el-table-column :label="copy.org" min-width="220">
          <template #default="{ row }">
            <span>{{ schoolName(row.schoolId) }}</span>
            <p class="cell-muted">{{ collegeName(row.collegeId) }} / {{ majorName(row.majorId) }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.actions" min-width="180" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openEdit(row)">{{ copy.edit }}</el-button>
              <el-button v-if="row.status !== 'archived'" size="small" type="danger" plain @click="archiveCourse(row.id)">
                {{ copy.archive }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="editOpen" :title="copy.editTitle" width="min(720px, 92vw)">
      <el-form class="form-grid" label-position="top">
        <el-form-item :label="copy.school">
          <el-select v-model="editForm.schoolId" :placeholder="copy.selectSchool" @change="onEditSchoolChange">
            <el-option v-for="school in schools" :key="school.id" :label="school.name" :value="school.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.college">
          <el-select v-model="editForm.collegeId" :placeholder="copy.selectCollege" @change="onEditCollegeChange">
            <el-option v-for="college in filteredColleges(editForm.schoolId)" :key="college.id" :label="college.name" :value="college.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.major">
          <el-select v-model="editForm.majorId" :placeholder="copy.selectMajor">
            <el-option v-for="major in filteredMajors(editForm.schoolId, editForm.collegeId)" :key="major.id" :label="major.name" :value="major.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.grade">
          <el-input v-model="editForm.grade" />
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="editForm.name" />
        </el-form-item>
        <el-form-item label="Slug">
          <el-input v-model="editForm.slug" />
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="editForm.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.courseDescription">
          <el-input v-model="editForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item class="wide" :label="copy.examScope">
          <el-input v-model="editForm.examScope" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="editOpen = false">{{ copy.cancel }}</el-button>
          <el-button type="primary" :loading="saving" @click="saveCourse">{{ copy.save }}</el-button>
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
import { apiRequest, type College, type Course, type Major, type School } from "../lib/api";

type CourseForm = {
  id: string;
  schoolId: string;
  collegeId: string;
  majorId: string;
  grade: string;
  name: string;
  slug: string;
  description: string;
  examScope: string;
  status: string;
};

const copy = {
  title: "\u8bfe\u7a0b\u7ef4\u62a4",
  description:
    "\u8bfe\u7a0b\u662f\u8d44\u6599\u5e93\u7684\u4e00\u7ea7\u5165\u53e3\u3002\u540e\u53f0\u4f7f\u7528 admin-only \u63a5\u53e3\u7ef4\u62a4\u5168\u72b6\u6001\u8bfe\u7a0b\uff0c\u524d\u53f0\u53ea\u5c55\u793a published \u8bfe\u7a0b\u3002",
  refresh: "\u5237\u65b0",
  createTitle: "\u65b0\u589e\u8bfe\u7a0b",
  createDescription: "\u7ec4\u7ec7\u5b57\u6bb5\u7528\u4e8e\u5165\u5e93\u5173\u8054\uff0c\u5b66\u751f\u7aef\u4f1a\u6309\u8bfe\u7a0b\u548c\u4e13\u4e1a\u7ec4\u7ec7\u8d44\u6599\u3002",
  school: "\u5b66\u6821",
  college: "\u5b66\u9662",
  major: "\u4e13\u4e1a",
  grade: "\u5e74\u7ea7",
  name: "\u8bfe\u7a0b\u540d",
  status: "\u72b6\u6001",
  courseDescription: "\u7b80\u4ecb",
  examScope: "\u8003\u8bd5\u8303\u56f4",
  selectSchool: "\u9009\u62e9\u5b66\u6821",
  selectCollege: "\u9009\u62e9\u5b66\u9662",
  selectMajor: "\u9009\u62e9\u4e13\u4e1a",
  createAction: "\u521b\u5efa\u8bfe\u7a0b",
  listTitle: "\u8bfe\u7a0b\u5217\u8868",
  course: "\u8bfe\u7a0b",
  org: "\u7ec4\u7ec7",
  actions: "\u64cd\u4f5c",
  edit: "\u7f16\u8f91",
  archive: "\u5f52\u6863",
  editTitle: "\u7f16\u8f91\u8bfe\u7a0b",
  cancel: "\u53d6\u6d88",
  save: "\u4fdd\u5b58",
  loadFailed: "\u52a0\u8f7d\u5931\u8d25",
  createDone: "\u8bfe\u7a0b\u5df2\u521b\u5efa\u3002",
  createFailed: "\u521b\u5efa\u5931\u8d25",
  updateDone: "\u8bfe\u7a0b\u5df2\u66f4\u65b0\u3002",
  updateFailed: "\u66f4\u65b0\u5931\u8d25",
  archived: "\u8bfe\u7a0b\u5df2\u5f52\u6863\u3002",
  archiveFailed: "\u5f52\u6863\u5931\u8d25",
};

const statuses = [
  { label: "\u8349\u7a3f", value: "draft" },
  { label: "\u5df2\u53d1\u5e03", value: "published" },
  { label: "\u5df2\u5f52\u6863", value: "archived" },
];

const schools = ref<School[]>([]);
const colleges = ref<College[]>([]);
const majors = ref<Major[]>([]);
const courses = ref<Course[]>([]);
const loading = ref(false);
const saving = ref(false);
const editOpen = ref(false);
const message = ref("");
const error = ref("");

const form = reactive<CourseForm>(emptyCourseForm("published"));
const editForm = reactive<CourseForm>(emptyCourseForm("draft"));

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [schoolResponse, collegeResponse, majorResponse, courseResponse] = await Promise.all([
      apiRequest<{ schools: School[] }>("/schools"),
      apiRequest<{ colleges: College[] }>("/colleges"),
      apiRequest<{ majors: Major[] }>("/majors"),
      apiRequest<{ courses: Course[] }>("/admin/courses"),
    ]);
    schools.value = schoolResponse.data?.schools ?? [];
    colleges.value = collegeResponse.data?.colleges ?? [];
    majors.value = majorResponse.data?.majors ?? [];
    courses.value = courseResponse.data?.courses ?? [];
    if (!form.schoolId && schools.value[0]) form.schoolId = schools.value[0].id;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function createCourse() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ course: Course }>("/admin/courses", {
      method: "POST",
      body: JSON.stringify(coursePayload(form)),
    });
    message.value = copy.createDone;
    resetCreateForm();
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.createFailed;
  } finally {
    loading.value = false;
  }
}

function openEdit(course: Course) {
  Object.assign(editForm, {
    id: course.id,
    schoolId: course.schoolId,
    collegeId: course.collegeId,
    majorId: course.majorId,
    grade: course.grade,
    name: course.name,
    slug: course.slug,
    description: course.description,
    examScope: course.examScope,
    status: course.status,
  });
  editOpen.value = true;
}

async function saveCourse() {
  if (!editForm.id) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ updated: boolean }>(`/admin/courses/${editForm.id}`, {
      method: "PATCH",
      body: JSON.stringify(coursePayload(editForm)),
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

async function archiveCourse(id: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ archived: boolean }>(`/admin/courses/${id}`, { method: "DELETE" });
    message.value = copy.archived;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.archiveFailed;
  }
}

function onCreateSchoolChange() {
  form.collegeId = "";
  form.majorId = "";
}

function onCreateCollegeChange() {
  form.majorId = "";
}

function onEditSchoolChange() {
  editForm.collegeId = "";
  editForm.majorId = "";
}

function onEditCollegeChange() {
  editForm.majorId = "";
}

function filteredColleges(schoolId: string) {
  return colleges.value.filter((college) => !schoolId || college.schoolId === schoolId);
}

function filteredMajors(schoolId: string, collegeId: string) {
  return majors.value.filter((major) => (!schoolId || major.schoolId === schoolId) && (!collegeId || major.collegeId === collegeId));
}

function schoolName(id: string) {
  return schools.value.find((school) => school.id === id)?.name ?? id;
}

function collegeName(id: string) {
  return colleges.value.find((college) => college.id === id)?.name ?? id;
}

function majorName(id: string) {
  return majors.value.find((major) => major.id === id)?.name ?? id;
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "published") return "success";
  if (status === "archived") return "info";
  return "warning";
}

function resetCreateForm() {
  const schoolId = form.schoolId;
  Object.assign(form, emptyCourseForm("published"), { schoolId });
}

function coursePayload(source: CourseForm) {
  return {
    schoolId: source.schoolId,
    collegeId: source.collegeId,
    majorId: source.majorId,
    grade: source.grade,
    name: source.name,
    slug: source.slug,
    description: source.description,
    examScope: source.examScope,
    status: source.status,
  };
}

function emptyCourseForm(status: string): CourseForm {
  return {
    id: "",
    schoolId: "",
    collegeId: "",
    majorId: "",
    grade: "2023",
    name: "",
    slug: "",
    description: "",
    examScope: "",
    status,
  };
}
</script>
