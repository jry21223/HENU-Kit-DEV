<template>
  <main class="admin-shell">
    <aside class="sidebar">
      <div class="brand">
        <strong>{{ copy.brand }}</strong>
        <span class="muted">{{ copy.subtitle }}</span>
      </div>
      <nav class="side-nav" :aria-label="copy.navLabel">
        <RouterLink v-if="auth.isAdmin" to="/dashboard">{{ copy.dashboard }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/users">{{ copy.users }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/access-grants">{{ copy.accessGrants }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/courses">{{ copy.courses }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/materials">{{ copy.materials }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/downloads">{{ copy.downloads }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/analytics">{{ copy.analytics }}</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/operation-logs">{{ copy.operationLogs }}</RouterLink>
        <span v-if="auth.canReviewContent" class="nav-section">{{ copy.reviewSection }}</span>
        <RouterLink v-if="auth.canReviewContent" to="/material-reviews">{{ copy.materialReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/wiki-reviews">{{ copy.wikiReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/wiki-proposal-reviews">{{ copy.wikiProposalReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/blog-reviews">{{ copy.blogReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/forum-reviews">{{ copy.forumReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/forum-reply-reviews">{{ copy.forumReplyReviews }}</RouterLink>
        <RouterLink v-if="auth.canReviewAI" to="/ai/drafts">{{ copy.aiDrafts }}</RouterLink>
        <RouterLink v-if="auth.canReviewContent" to="/reports">{{ copy.reports }}</RouterLink>
        <span v-if="auth.isAdmin" class="disabled-nav" aria-disabled="true">{{ copy.community }}</span>
      </nav>
      <div class="sidebar-footer">
        <span>{{ auth.user?.email }}</span>
        <el-button size="small" @click="handleLogout">{{ copy.logout }}</el-button>
      </div>
    </aside>
    <section class="content">
      <slot />
    </section>
  </main>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const copy = {
  brand: "\u8d44\u6599\u8fd0\u8425\u5de5\u4f5c\u53f0",
  subtitle: "\u8f6f\u4ef6\u5b66\u9662\u8bfe\u7a0b\u8d44\u6599\u5e93",
  navLabel: "\u540e\u53f0\u5bfc\u822a",
  dashboard: "\u8fd0\u8425\u6982\u89c8",
  users: "\u7528\u6237\u7ba1\u7406",
  accessGrants: "\u6743\u76ca\u6388\u6743",
  courses: "\u8bfe\u7a0b\u7ef4\u62a4",
  materials: "PDF \u8d44\u6599",
  downloads: "\u4e0b\u8f7d\u5ba1\u8ba1",
  analytics: "\u8fd0\u8425\u5206\u6790",
  operationLogs: "\u64cd\u4f5c\u65e5\u5fd7",
  reviewSection: "\u5ba1\u6838\u6d41\u7a0b",
  materialReviews: "\u8d44\u6599\u5ba1\u6838",
  wikiReviews: "Wiki \u5ba1\u6838",
  wikiProposalReviews: "Wiki \u63d0\u6848\u5ba1\u6838",
  blogReviews: "\u535a\u5ba2\u5ba1\u6838",
  forumReviews: "\u5e16\u5b50\u5ba1\u6838",
  forumReplyReviews: "\u56de\u590d\u5ba1\u6838",
  aiDrafts: "AI \u8349\u7a3f",
  reports: "\u4e3e\u62a5\u5904\u7406",
  community: "\u8bfe\u7a0b\u793e\u533a",
  logout: "\u9000\u51fa",
};

const auth = useAuthStore();
const router = useRouter();

async function handleLogout() {
  await auth.logout();
  await router.push("/login");
}
</script>
