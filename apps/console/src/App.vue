<script setup lang="ts">
import { Activity, Bell, BookOpen, Building2, Menu, Search, ShieldCheck, Utensils, X } from "@lucide/vue";
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from "reka-ui";
import { computed, ref } from "vue";

import ModuleCard from "@/components/ModuleCard.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import UiButton from "@/components/ui/UiButton.vue";
import { moduleSummaries, type ModuleSummary } from "@/data/modules";

const icons = {
  portal: Building2,
  platform: ShieldCheck,
  notice: Bell,
  library: BookOpen,
  quizcraft: Activity,
  food: Utensils,
};

const query = new URLSearchParams(window.location.search);
const loading = query.get("scenario") === "loading";
const mobileNavigationOpen = ref(false);

const summaries = computed<ModuleSummary[]>(() =>
  loading
    ? moduleSummaries.map((summary) => ({ ...summary, status: "loading", metrics: [] }))
    : moduleSummaries,
);

const visibleCount = computed(() => summaries.value.filter((summary) => summary.status !== "denied").length);
</script>

<template>
  <div class="min-h-screen bg-[var(--hk-paper)] text-[var(--hk-ink)]" data-console-shell>
    <aside class="console-sidebar" aria-label="Console 主导航">
      <div class="flex items-center gap-3 px-4 py-5">
        <div class="grid size-10 place-items-center rounded-xl bg-[var(--hk-wheat-gold)] font-black text-[var(--hk-ink-green-deep)]">H</div>
        <div>
          <strong class="block text-base tracking-tight text-white">HENUKit Console</strong>
          <span class="text-sm text-white/75">学生自主运营</span>
        </div>
      </div>

      <nav class="mt-4 grid gap-1 px-3" aria-label="产品模块">
        <a
          v-for="module in moduleSummaries"
          :key="module.id"
          :href="`#module-${module.id}`"
          class="flex min-h-11 items-center gap-3 rounded-xl px-3 text-base font-medium text-white/75 transition hover:bg-white/10 hover:text-white focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--hk-wheat-gold)]"
        >
          <component :is="icons[module.id]" :size="17" aria-hidden="true" />
          {{ module.name }}
        </a>
      </nav>

      <div class="mt-auto p-4 text-sm leading-6 text-white/85">非河南大学官方项目<br />Console V1 · Mock data</div>
    </aside>

    <div class="console-main">
      <header class="console-topbar">
        <DialogRoot v-model:open="mobileNavigationOpen">
          <DialogTrigger as-child>
            <UiButton variant="ghost" size="icon" class="lg:hidden" aria-label="打开产品导航">
              <Menu :size="20" />
            </UiButton>
          </DialogTrigger>
          <DialogPortal>
            <DialogOverlay class="fixed inset-0 z-40 bg-black/35 backdrop-blur-sm" />
            <DialogContent class="fixed inset-y-0 left-0 z-50 w-[min(84vw,20rem)] bg-[var(--hk-ink-green-deep)] p-5 text-white shadow-2xl">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <DialogTitle class="font-semibold">产品模块</DialogTitle>
                  <DialogDescription class="mt-1 text-sm text-white/75">六个已确认的运营模块</DialogDescription>
                </div>
                <DialogClose as-child>
                  <UiButton variant="ghost-inverse" size="icon" aria-label="关闭产品导航"><X :size="20" /></UiButton>
                </DialogClose>
              </div>
              <nav class="mt-6 grid gap-2" aria-label="移动端产品模块">
                <DialogClose v-for="module in moduleSummaries" :key="module.id" as-child>
                  <a :href="`#module-${module.id}`" class="flex min-h-11 items-center gap-3 rounded-xl px-3 text-base text-white/85 hover:bg-white/10">
                    <component :is="icons[module.id]" :size="18" aria-hidden="true" />{{ module.name }}
                  </a>
                </DialogClose>
              </nav>
            </DialogContent>
          </DialogPortal>
        </DialogRoot>

        <label class="search-control">
          <Search :size="17" aria-hidden="true" />
          <span class="sr-only">搜索模块</span>
          <input type="search" placeholder="搜索模块或状态" disabled aria-describedby="search-note" />
        </label>
        <span id="search-note" class="sr-only">Mock 阶段暂不提供搜索</span>

        <div class="ml-auto flex items-center gap-3">
          <StatusBadge status="loading">Mock 权限态</StatusBadge>
          <div class="operator-avatar" aria-label="当前操作员：Console Operator">CO</div>
        </div>
      </header>

      <main class="mx-auto w-full max-w-[var(--hk-content-max)] px-4 py-6 sm:px-6 lg:px-8 lg:py-9">
        <section class="overview-hero" aria-labelledby="overview-heading">
          <div>
            <p class="eyebrow">Operations overview</p>
            <h1 id="overview-heading" class="mt-2 text-2xl font-bold tracking-[-0.03em] sm:text-3xl">产品运行概览</h1>
            <p class="mt-2 max-w-2xl text-base leading-7 text-[var(--hk-ink-muted)]">
              六个产品模块保持各自的数据所有权。此页面仅验证 Console 的结构、权限表达与降级状态，不连接生产服务。
            </p>
          </div>
          <div class="access-context" aria-label="Mock 访问上下文">
            <span>console.overview.read</span>
            <strong>{{ visibleCount }}/6 可见</strong>
          </div>
        </section>

        <section class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3" :aria-busy="loading">
          <ModuleCard
            v-for="module in summaries"
            :id="`module-${module.id}`"
            :key="module.id"
            :summary="module"
            :icon="icons[module.id]"
          />
        </section>

        <section class="mt-6 rounded-[var(--hk-radius-feature)] border border-[var(--hk-paper-line)] bg-white p-5 shadow-[var(--hk-shadow-card)]" aria-labelledby="permission-heading">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p class="eyebrow">Access context</p>
              <h2 id="permission-heading" class="mt-1 text-lg font-semibold">权限按代码与 Scope 表达</h2>
              <p class="mt-2 max-w-2xl text-base leading-7 text-[var(--hk-ink-muted)]">当前仅模拟未来由服务端返回的访问上下文；正式接入后，前端不信任浏览器角色，也不使用旧的单一 isAdmin 判断。</p>
            </div>
            <div class="flex flex-wrap gap-2" aria-label="Mock 权限代码">
              <code>console.overview.read</code><code>platform.health.read</code><code>scope:all-products</code>
            </div>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>
