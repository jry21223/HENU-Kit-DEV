<template>
  <AdminShell>
    <div class="page-header">
      <div>
        <p class="eyebrow">Packages</p>
        <h1>{{ copy.title }}</h1>
        <p class="muted">{{ copy.description }}</p>
      </div>
      <el-button type="primary" :loading="loading" @click="loadAll">{{ copy.refresh }}</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.createTitle }}</strong>
      </template>
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
          <el-select v-model="form.majorId" :placeholder="copy.selectMajor" @change="onCreateMajorChange">
            <el-option v-for="major in filteredMajors(form.schoolId, form.collegeId)" :key="major.id" :label="major.name" :value="major.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.course">
          <el-select v-model="form.courseId" clearable :placeholder="copy.selectCourse">
            <el-option v-for="course in filteredCourses(form)" :key="course.id" :label="courseLabel(course)" :value="course.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.grade">
          <el-input v-model="form.grade" placeholder="2023" />
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="form.title" placeholder="Discrete Math final package" />
        </el-form-item>
        <el-form-item label="Slug">
          <el-input v-model="form.slug" placeholder="discrete-math-final" />
        </el-form-item>
        <el-form-item :label="copy.price">
          <el-input-number v-model="form.priceFen" :min="0" :step="100" style="width: 100%" />
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="form.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.packageDescription">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="loading" @click="createPackage">{{ copy.createAction }}</el-button>
        <span class="muted">{{ copy.createHint }}</span>
      </div>
    </el-card>

    <el-card class="section-card" shadow="never">
      <template #header>
        <strong>{{ copy.listTitle }}</strong>
      </template>
      <el-table v-loading="loading" :data="packages" empty-text="No packages" style="width: 100%">
        <el-table-column :label="copy.package" min-width="220">
          <template #default="{ row }">
            <strong>{{ row.title }}</strong>
            <p class="cell-muted">{{ row.slug }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.price" width="120">
          <template #default="{ row }">{{ priceLabel(row) }}</template>
        </el-table-column>
        <el-table-column :label="copy.org" min-width="240">
          <template #default="{ row }">
            <span>{{ schoolName(row.schoolId) }}</span>
            <p class="cell-muted">{{ collegeName(row.collegeId) }} / {{ majorName(row.majorId) }} / {{ row.grade }}</p>
            <p class="cell-muted">{{ courseName(row.courseId) }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.status" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="copy.actions" min-width="240" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openItems(row)">{{ copy.items }}</el-button>
              <el-button size="small" @click="openEdit(row)">{{ copy.edit }}</el-button>
              <el-button v-if="row.status !== 'archived'" size="small" type="danger" plain @click="archivePackage(row.id)">
                {{ copy.archive }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="editOpen" :title="copy.editTitle" width="min(760px, 92vw)">
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
          <el-select v-model="editForm.majorId" :placeholder="copy.selectMajor" @change="onEditMajorChange">
            <el-option v-for="major in filteredMajors(editForm.schoolId, editForm.collegeId)" :key="major.id" :label="major.name" :value="major.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.course">
          <el-select v-model="editForm.courseId" clearable :placeholder="copy.selectCourse">
            <el-option v-for="course in filteredCourses(editForm)" :key="course.id" :label="courseLabel(course)" :value="course.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.grade">
          <el-input v-model="editForm.grade" />
        </el-form-item>
        <el-form-item :label="copy.name">
          <el-input v-model="editForm.title" />
        </el-form-item>
        <el-form-item label="Slug">
          <el-input v-model="editForm.slug" />
        </el-form-item>
        <el-form-item :label="copy.price">
          <el-input-number v-model="editForm.priceFen" :min="0" :step="100" style="width: 100%" />
        </el-form-item>
        <el-form-item :label="copy.status">
          <el-select v-model="editForm.status">
            <el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item class="wide" :label="copy.packageDescription">
          <el-input v-model="editForm.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="action-row">
          <el-button @click="editOpen = false">{{ copy.cancel }}</el-button>
          <el-button type="primary" :loading="saving" @click="savePackage">{{ copy.save }}</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="itemsOpen" :title="itemsTitle" width="min(920px, 94vw)">
      <el-form class="form-grid" label-position="top" @submit.prevent>
        <el-form-item :label="copy.material">
          <el-select v-model="itemForm.resourceId" filterable :placeholder="copy.selectMaterial">
            <el-option
              v-for="material in filteredMaterialsForSelectedPackage"
              :key="material.id"
              :label="materialLabel(material)"
              :value="material.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="copy.sortOrder">
          <el-input-number v-model="itemForm.sortOrder" :min="0" style="width: 100%" />
        </el-form-item>
      </el-form>
      <div class="action-row">
        <el-button type="primary" :loading="saving" @click="addItem">{{ copy.addItem }}</el-button>
        <span class="muted">{{ copy.itemHint }}</span>
      </div>
      <el-table v-loading="saving" :data="items" empty-text="No package items" style="width: 100%">
        <el-table-column :label="copy.material" min-width="240">
          <template #default="{ row }">
            <strong>{{ row.material?.title || row.item.resourceId }}</strong>
            <p class="cell-muted">{{ row.material ? `${row.material.type} / ${row.material.accessLevel}` : row.item.resourceType }}</p>
          </template>
        </el-table-column>
        <el-table-column :label="copy.materialStatus" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.material" :type="materialStatusTag(row.material.status)">{{ row.material.status }}</el-tag>
            <span v-else>{{ copy.unknown }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="item.sortOrder" :label="copy.sortOrder" width="100" />
        <el-table-column :label="copy.actions" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="removeItem(row.item.id)">{{ copy.remove }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-alert v-if="message" class="notice" type="success" :closable="false" :title="message" />
    <el-alert v-if="error" class="notice" type="error" :closable="false" :title="error" />
  </AdminShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import AdminShell from "../components/AdminShell.vue";
import {
  apiRequest,
  type College,
  type Course,
  type CoursePackage,
  type CoursePackageItemRow,
  type Major,
  type Material,
  type School,
} from "../lib/api";

type PackageForm = {
  id: string;
  schoolId: string;
  collegeId: string;
  majorId: string;
  courseId: string;
  grade: string;
  title: string;
  slug: string;
  description: string;
  priceFen: number;
  currency: string;
  status: string;
};

const copy = {
  title: "课程包管理",
  description: "维护可销售或可手动授权的课程复习包，并把已入库资料绑定到课程包。公开页面只展示 published 课程包和 published 资料。",
  refresh: "刷新",
  createTitle: "新增课程包",
  createHint: "默认建议先保存为草稿，确认包内资料后再发布。",
  school: "学校",
  college: "学院",
  major: "专业",
  course: "课程",
  grade: "年级",
  name: "课程包名",
  price: "价格(分)",
  status: "状态",
  packageDescription: "说明",
  selectSchool: "选择学校",
  selectCollege: "选择学院",
  selectMajor: "选择专业",
  selectCourse: "选择课程，可留空",
  createAction: "创建课程包",
  listTitle: "课程包列表",
  package: "课程包",
  org: "组织",
  actions: "操作",
  items: "资料",
  edit: "编辑",
  archive: "归档",
  editTitle: "编辑课程包",
  cancel: "取消",
  save: "保存",
  material: "资料",
  selectMaterial: "选择要加入课程包的资料",
  sortOrder: "排序",
  addItem: "加入课程包",
  itemHint: "可以先绑定草稿资料；公开课程包详情会自动隐藏未发布资料和对应 item。",
  materialStatus: "资料状态",
  remove: "移除",
  unknown: "未知",
  noCourse: "未绑定具体课程",
  loadFailed: "加载失败",
  createDone: "课程包已创建。",
  createFailed: "课程包创建失败",
  updateDone: "课程包已更新。",
  updateFailed: "课程包更新失败",
  archived: "课程包已归档。",
  archiveFailed: "课程包归档失败",
  itemAdded: "资料已加入课程包。",
  itemRemoved: "资料已从课程包移除。",
  itemFailed: "包内资料操作失败",
};

const statuses = [
  { label: "草稿", value: "draft" },
  { label: "已发布", value: "published" },
  { label: "已归档", value: "archived" },
];

const schools = ref<School[]>([]);
const colleges = ref<College[]>([]);
const majors = ref<Major[]>([]);
const courses = ref<Course[]>([]);
const materials = ref<Material[]>([]);
const packages = ref<CoursePackage[]>([]);
const items = ref<CoursePackageItemRow[]>([]);
const selectedPackage = ref<CoursePackage | null>(null);
const loading = ref(false);
const saving = ref(false);
const editOpen = ref(false);
const itemsOpen = ref(false);
const message = ref("");
const error = ref("");

const form = reactive<PackageForm>(emptyPackageForm());
const editForm = reactive<PackageForm>(emptyPackageForm());
const itemForm = reactive({ resourceId: "", sortOrder: 0 });

const itemsTitle = computed(() => (selectedPackage.value ? `${selectedPackage.value.title} - ${copy.items}` : copy.items));

const filteredMaterialsForSelectedPackage = computed(() => {
  const pkg = selectedPackage.value;
  if (!pkg) return materials.value;
  return materials.value.filter((material) => {
    if (pkg.courseId && material.courseId !== pkg.courseId) return false;
    const course = courses.value.find((item) => item.id === material.courseId);
    if (!course) return true;
    return course.schoolId === pkg.schoolId && course.collegeId === pkg.collegeId && course.majorId === pkg.majorId && course.grade === pkg.grade;
  });
});

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [schoolResponse, collegeResponse, majorResponse, courseResponse, materialResponse, packageResponse] = await Promise.all([
      apiRequest<{ schools: School[] }>("/schools"),
      apiRequest<{ colleges: College[] }>("/colleges"),
      apiRequest<{ majors: Major[] }>("/majors"),
      apiRequest<{ courses: Course[] }>("/admin/courses"),
      apiRequest<{ materials: Material[] }>("/admin/materials"),
      apiRequest<{ packages: CoursePackage[] }>("/admin/packages"),
    ]);
    schools.value = schoolResponse.data?.schools ?? [];
    colleges.value = collegeResponse.data?.colleges ?? [];
    majors.value = majorResponse.data?.majors ?? [];
    courses.value = courseResponse.data?.courses ?? [];
    materials.value = materialResponse.data?.materials ?? [];
    packages.value = packageResponse.data?.packages ?? [];
    if (!form.schoolId && schools.value[0]) form.schoolId = schools.value[0].id;
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.loadFailed;
  } finally {
    loading.value = false;
  }
}

async function createPackage() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ package: CoursePackage }>("/admin/packages", {
      method: "POST",
      body: JSON.stringify(packagePayload(form)),
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

function openEdit(coursePackage: CoursePackage) {
  Object.assign(editForm, {
    id: coursePackage.id,
    schoolId: coursePackage.schoolId,
    collegeId: coursePackage.collegeId,
    majorId: coursePackage.majorId,
    courseId: coursePackage.courseId ?? "",
    grade: coursePackage.grade,
    title: coursePackage.title,
    slug: coursePackage.slug,
    description: coursePackage.description,
    priceFen: coursePackage.priceFen,
    currency: coursePackage.currency,
    status: coursePackage.status,
  });
  editOpen.value = true;
}

async function savePackage() {
  if (!editForm.id) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ package: CoursePackage }>(`/admin/packages/${editForm.id}`, {
      method: "PATCH",
      body: JSON.stringify(packagePayload(editForm)),
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

async function archivePackage(id: string) {
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ archived: boolean }>(`/admin/packages/${id}`, { method: "DELETE" });
    message.value = copy.archived;
    await loadAll();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.archiveFailed;
  }
}

async function openItems(coursePackage: CoursePackage) {
  selectedPackage.value = coursePackage;
  itemForm.resourceId = "";
  itemForm.sortOrder = 0;
  itemsOpen.value = true;
  await loadItems();
}

async function loadItems() {
  if (!selectedPackage.value) return;
  saving.value = true;
  error.value = "";
  try {
    const response = await apiRequest<{ package: CoursePackage; items: CoursePackageItemRow[] }>(
      `/admin/packages/${selectedPackage.value.id}/items`,
    );
    items.value = response.data?.items ?? [];
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.itemFailed;
  } finally {
    saving.value = false;
  }
}

async function addItem() {
  if (!selectedPackage.value || !itemForm.resourceId) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ item: unknown; alreadyExists: boolean }>(`/admin/packages/${selectedPackage.value.id}/items`, {
      method: "POST",
      body: JSON.stringify({ resourceType: "material", resourceId: itemForm.resourceId, sortOrder: itemForm.sortOrder }),
    });
    message.value = copy.itemAdded;
    itemForm.resourceId = "";
    await loadItems();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.itemFailed;
  } finally {
    saving.value = false;
  }
}

async function removeItem(itemId: string) {
  if (!selectedPackage.value) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await apiRequest<{ deleted: boolean }>(`/admin/packages/${selectedPackage.value.id}/items/${itemId}`, { method: "DELETE" });
    message.value = copy.itemRemoved;
    await loadItems();
  } catch (err) {
    error.value = err instanceof Error ? err.message : copy.itemFailed;
  } finally {
    saving.value = false;
  }
}

function onCreateSchoolChange() {
  form.collegeId = "";
  form.majorId = "";
  form.courseId = "";
}

function onCreateCollegeChange() {
  form.majorId = "";
  form.courseId = "";
}

function onCreateMajorChange() {
  form.courseId = "";
}

function onEditSchoolChange() {
  editForm.collegeId = "";
  editForm.majorId = "";
  editForm.courseId = "";
}

function onEditCollegeChange() {
  editForm.majorId = "";
  editForm.courseId = "";
}

function onEditMajorChange() {
  editForm.courseId = "";
}

function filteredColleges(schoolId: string) {
  return colleges.value.filter((college) => !schoolId || college.schoolId === schoolId);
}

function filteredMajors(schoolId: string, collegeId: string) {
  return majors.value.filter((major) => (!schoolId || major.schoolId === schoolId) && (!collegeId || major.collegeId === collegeId));
}

function filteredCourses(source: PackageForm) {
  return courses.value.filter((course) => {
    return (
      (!source.schoolId || course.schoolId === source.schoolId) &&
      (!source.collegeId || course.collegeId === source.collegeId) &&
      (!source.majorId || course.majorId === source.majorId) &&
      (!source.grade || course.grade === source.grade)
    );
  });
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

function courseName(id?: string) {
  if (!id) return copy.noCourse;
  return courses.value.find((course) => course.id === id)?.name ?? id;
}

function courseLabel(course: Course) {
  return `${course.name} - ${course.grade} - ${statusLabel(course.status)}`;
}

function materialLabel(material: Material) {
  return `${material.title} - ${material.status} - ${material.accessLevel}`;
}

function priceLabel(coursePackage: CoursePackage) {
  return `${coursePackage.currency || "CNY"} ${(coursePackage.priceFen / 100).toFixed(2)}`;
}

function statusLabel(status: string) {
  return statuses.find((item) => item.value === status)?.label ?? status;
}

function statusTag(status: string) {
  if (status === "published") return "success";
  if (status === "archived") return "info";
  return "warning";
}

function materialStatusTag(status: string) {
  if (status === "published") return "success";
  if (status === "archived") return "info";
  if (status === "rejected") return "danger";
  return "warning";
}

function resetCreateForm() {
  const schoolId = form.schoolId;
  Object.assign(form, emptyPackageForm(), { schoolId });
}

function packagePayload(source: PackageForm) {
  return {
    schoolId: source.schoolId,
    collegeId: source.collegeId,
    majorId: source.majorId,
    courseId: source.courseId,
    grade: source.grade,
    title: source.title,
    slug: source.slug,
    description: source.description,
    priceFen: source.priceFen,
    currency: source.currency,
    status: source.status,
  };
}

function emptyPackageForm(): PackageForm {
  return {
    id: "",
    schoolId: "",
    collegeId: "",
    majorId: "",
    courseId: "",
    grade: "2023",
    title: "",
    slug: "",
    description: "",
    priceFen: 1990,
    currency: "CNY",
    status: "draft",
  };
}
</script>
