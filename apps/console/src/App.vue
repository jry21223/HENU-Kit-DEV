<script setup lang="ts">
import { Activity, Bell, BookOpen, Building2, Menu, MessageSquare, Search, ShieldCheck, Utensils, X } from "@lucide/vue";
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from "reka-ui";
import { computed, onMounted, ref } from "vue";

import ModuleCard from "@/components/ModuleCard.vue";
import { Button, PageHeader } from "@/components/ui";
import LibraryOperationsView from "@/components/LibraryOperationsView.vue";
import FoodOperationsView from "@/components/FoodOperationsView.vue";
import AccountMembershipOperationsView from "@/components/AccountMembershipOperationsView.vue";
import AccountPointOperationsView from "@/components/AccountPointOperationsView.vue";
import AccountTicketOperationsView from "@/components/AccountTicketOperationsView.vue";
import NoticeOperationsView from "@/components/NoticeOperationsView.vue";
import PlatformOperationsView from "@/components/PlatformOperationsView.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import { moduleSummaries, type ModuleSummary } from "@/data/modules";
import { consolePath, isConsolePath } from "@/lib/base-path";
import { consoleLoginHref, fetchConsoleOverview, fetchConsoleSession, logoutConsoleSession, type ConsoleOverview, type ConsoleSession } from "@/lib/console-gateway";

const icons = {
  portal: Building2,
  platform: ShieldCheck,
  notice: Bell,
  library: BookOpen,
  quizcraft: Activity,
  food: Utensils,
};

const query = new URLSearchParams(window.location.search);
const isPlatformOperations = isConsolePath("/operations");
const isNoticeOperations = isConsolePath("/notices");
const isLibraryOperations = isConsolePath("/library");
const isFoodOperations = isConsolePath("/food");
const isAccountMembershipOperations = isConsolePath("/account/memberships");
const isAccountPointOperations = isConsolePath("/account/points");
const isAccountTicketOperations = isConsolePath("/account");
const operationsHref = consolePath("/operations");
const noticesHref = consolePath("/notices");
const libraryHref = consolePath("/library");
const foodHref = consolePath("/food");
const accountMembershipsHref = consolePath("/account/memberships");
const accountPointsHref = consolePath("/account/points");
const accountTicketsHref = consolePath("/account");
const loading = query.get("scenario") === "loading";
const mobileNavigationOpen = ref(false);
const authState = ref<"loading" | "authenticated" | "signed_out" | "denied" | "unavailable">("loading");
const consoleSession = ref<ConsoleSession>();
const consoleOverview = ref<ConsoleOverview>();
const overviewState = ref<"loading" | "ready" | "unavailable">("loading");

async function refreshSession() {
  authState.value = "loading";
  const result = await fetchConsoleSession();
  authState.value = result.state;
  consoleSession.value = result.state === "authenticated" ? result.session : undefined;
  consoleOverview.value = undefined;
  if (result.state === "authenticated" && !isPlatformOperations && !isNoticeOperations && !isLibraryOperations && !isFoodOperations && !isAccountMembershipOperations && !isAccountPointOperations && !isAccountTicketOperations) {
    overviewState.value = "loading";
    const overviewResult = await fetchConsoleOverview();
    if (overviewResult.state === "authenticated") {
      consoleOverview.value = overviewResult.overview;
      overviewState.value = "ready";
    } else if (overviewResult.state === "signed_out" || overviewResult.state === "denied") {
      authState.value = overviewResult.state;
      consoleSession.value = undefined;
      overviewState.value = "unavailable";
    } else {
      overviewState.value = "unavailable";
    }
  }
}

async function signOut() {
  try {
    await logoutConsoleSession();
    authState.value = "signed_out";
    consoleSession.value = undefined;
    consoleOverview.value = undefined;
  } catch {
    authState.value = "unavailable";
  }
}

onMounted(refreshSession);

const summaries = computed<ModuleSummary[]>(() =>
  loading || authState.value === "loading" || (authState.value === "authenticated" && overviewState.value === "loading")
    ? moduleSummaries.map((summary) => ({ ...summary, status: "loading", metrics: [], trend: undefined }))
    : authState.value === "authenticated"
      ? overviewState.value === "ready" && consoleOverview.value
        ? moduleSummaries.map((presentation) => {
            const live = consoleOverview.value?.modules.find((module) => module.id === presentation.id);
            return live
              ? {
                  ...presentation,
                  status: live.status,
                  metrics: live.metrics,
                  statusMessage: live.status_message,
                  asOf: live.as_of,
                  lastSuccessAt: live.last_success_at,
                  requestId: live.request_id,
                  trend: undefined,
                }
              : { ...presentation, status: "unavailable", metrics: [], statusMessage: "此模块暂无数据。", trend: undefined };
          })
        : moduleSummaries.map((summary) => ({ ...summary, status: "unavailable", metrics: [], statusMessage: "概览数据暂时不可用，请稍后刷新页面。", trend: undefined }))
      : moduleSummaries.map((summary) => ({ ...summary, status: "denied", metrics: [], statusMessage: "登录后即可查看，或联系管理员开通权限。" })),
);

const visibleCount = computed(() => summaries.value.filter((summary) => summary.status !== "denied").length);
</script>

<template>
  <div class="min-h-screen bg-[var(--hk-paper)] text-[var(--hk-ink)]" data-console-shell>
    <aside class="console-sidebar" aria-label="Console 主导航">
      <div class="flex items-center gap-3 px-4 py-5">
        <div class="grid size-10 place-items-center rounded-[var(--hk-radius-control)] bg-[var(--hk-accent)] font-black text-[var(--hk-paper)]">H</div>
        <div>
          <strong class="block text-base tracking-tight text-white">HENUKit Console</strong>
          <span class="text-sm text-white/75">学生自主运营</span>
        </div>
      </div>

      <nav class="mt-4 grid gap-1 px-3" aria-label="产品模块">
        <a v-if="consoleSession?.access_context.permissions.includes('platform.operations.read')" :href="operationsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10">
          <ShieldCheck :size="17" aria-hidden="true" />平台运营工作台
        </a>
        <a v-if="consoleSession?.access_context.permissions.includes('notice.read')" :href="noticesHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><Bell :size="17" aria-hidden="true" />通知审核与分发</a>
        <a v-if="consoleSession?.access_context.permissions.includes('library.read')" :href="libraryHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><BookOpen :size="17" aria-hidden="true" />资料库运营</a>
        <a v-if="consoleSession?.access_context.permissions.includes('food.read')" :href="foodHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><Utensils :size="17" aria-hidden="true" />美食运营</a>
        <a v-if="consoleSession?.access_context.permissions.includes('account.membership.write')" :href="accountMembershipsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><ShieldCheck :size="17" aria-hidden="true" />会员权益运营</a>
        <a v-if="consoleSession?.access_context.permissions.includes('account.points.adjust')" :href="accountPointsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><Activity :size="17" aria-hidden="true" />积分账本运营</a>
        <a v-if="consoleSession?.access_context.permissions.includes('account.tickets.read')" :href="accountTicketsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-semibold text-white hover:bg-white/10"><MessageSquare :size="17" aria-hidden="true" />账户工单运营</a>
        <a
          v-for="module in moduleSummaries"
          :key="module.id"
          :href="`#module-${module.id}`"
          class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base font-medium text-white/75 transition hover:bg-white/10 hover:text-white focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--hk-accent)]"
        >
          <component :is="icons[module.id]" :size="17" aria-hidden="true" />
          {{ module.name }}
        </a>
      </nav>

      <div class="mt-auto p-4 text-sm leading-6 text-white/85">非河南大学官方项目<br />运营管理后台</div>
    </aside>

    <div class="console-main">
      <header class="console-topbar">
        <DialogRoot v-model:open="mobileNavigationOpen">
          <DialogTrigger as-child>
            <Button variant="ghost" size="icon" class="lg:hidden" aria-label="打开产品导航">
              <Menu :size="20" />
            </Button>
          </DialogTrigger>
          <DialogPortal>
            <DialogOverlay class="fixed inset-0 z-40 bg-black/35 backdrop-blur-sm" />
            <DialogContent class="fixed inset-y-0 left-0 z-50 w-[min(84vw,20rem)] bg-[var(--hk-ink)] p-5 text-white shadow-2xl">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <DialogTitle class="font-semibold">产品模块</DialogTitle>
                  <DialogDescription class="mt-1 text-sm text-white/75">专属工作台与 {{ summaries.length }} 个已确认的运营模块</DialogDescription>
                </div>
                <DialogClose as-child>
                  <Button variant="ghost-inverse" size="icon" aria-label="关闭产品导航"><X :size="20" /></Button>
                </DialogClose>
              </div>
              <nav class="mt-6 grid gap-2" aria-label="移动端产品模块">
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('platform.operations.read')" as-child><a :href="operationsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><ShieldCheck :size="18" />平台运营工作台</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('notice.read')" as-child><a :href="noticesHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><Bell :size="18" />通知审核与分发</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('library.read')" as-child><a :href="libraryHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><BookOpen :size="18" />资料库运营</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('food.read')" as-child><a :href="foodHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><Utensils :size="18" />美食运营</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('account.membership.write')" as-child><a :href="accountMembershipsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><ShieldCheck :size="18" />会员权益运营</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('account.points.adjust')" as-child><a :href="accountPointsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><Activity :size="18" />积分账本运营</a></DialogClose>
                <DialogClose v-if="consoleSession?.access_context.permissions.includes('account.tickets.read')" as-child><a :href="accountTicketsHref" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white"><MessageSquare :size="18" />账户工单运营</a></DialogClose>
                <DialogClose v-for="module in moduleSummaries" :key="module.id" as-child>
                  <a :href="`#module-${module.id}`" class="flex min-h-11 items-center gap-3 rounded-[var(--hk-radius-control)] px-3 text-base text-white/85 hover:bg-white/10">
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
          <input type="search" placeholder="搜索模块" disabled aria-describedby="search-note" />
        </label>
        <span id="search-note" class="sr-only">搜索即将开放</span>

        <div class="ml-auto flex items-center gap-3">
          <StatusBadge v-if="authState === 'loading'" status="loading">正在验证登录状态</StatusBadge>
          <template v-else-if="authState === 'authenticated'">
            <StatusBadge status="ok">权限已验证</StatusBadge>
            <button type="button" class="operator-avatar" aria-label="退出 Console" @click="signOut">CO</button>
          </template>
          <StatusBadge v-else-if="authState === 'denied'" status="denied">权限不足</StatusBadge>
          <Button v-else-if="authState === 'unavailable'" @click="refreshSession">重试连接</Button>
          <Button v-else as="a" :href="consoleLoginHref()">登录 Console</Button>
        </div>
      </header>

      <main class="mx-auto w-full max-w-[var(--hk-content-max)] px-4 py-6 sm:px-6 lg:px-8 lg:py-9">
        <PlatformOperationsView v-if="isPlatformOperations" :auth-state="authState" />
        <NoticeOperationsView v-else-if="isNoticeOperations" :auth-state="authState" :permissions="consoleSession?.access_context.permissions ?? []" />
        <LibraryOperationsView v-else-if="isLibraryOperations" :auth-state="authState" :permissions="consoleSession?.access_context.permissions ?? []" />
        <FoodOperationsView v-else-if="isFoodOperations" :auth-state="authState" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountMembershipOperationsView v-else-if="isAccountMembershipOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountPointOperationsView v-else-if="isAccountPointOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountTicketOperationsView v-else-if="isAccountTicketOperations" :auth-state="authState" :permissions="consoleSession?.access_context.permissions ?? []" />
        <template v-else>
        <PageHeader
          eyebrow="运营概览"
          title="产品运行概览"
          :description="`${summaries.length} 个运营模块的运行状态与关键指标总览，供运营人员快速了解全站情况。`"
          title-id="overview-heading"
        >
          <div class="access-context" aria-label="服务端验证的访问上下文">
            <span>{{ authState === "authenticated" ? "已授予概览查看权限" : "需要服务端权限" }}</span>
            <strong>{{ authState === "authenticated" ? visibleCount : 0 }}/{{ summaries.length }} 可见</strong>
          </div>
        </PageHeader>

        <section class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3" :aria-busy="loading">
          <ModuleCard
            v-for="module in summaries"
            :id="`module-${module.id}`"
            :key="module.id"
            :summary="module"
            :icon="icons[module.id]"
          />
        </section>

        <section class="mt-6 rounded-[var(--hk-radius-feature)] border border-[var(--hk-line)] bg-white p-5 shadow-[var(--hk-shadow-card)]" aria-labelledby="permission-heading">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p class="eyebrow">权限信息</p>
              <h2 id="permission-heading" class="mt-1 text-lg font-semibold">权限说明</h2>
              <p class="mt-2 max-w-2xl text-base leading-7 text-[var(--hk-ink-muted)]">登录状态与权限由服务端在每次请求时验证，前端不自行判断权限。</p>
            </div>
            <div class="flex flex-wrap gap-2" aria-label="服务端验证的权限代码">
              <template v-if="consoleSession">
                <code>已授予概览查看权限</code>
                <code>已授予的平台权限</code>
              </template>
              <span v-else>{{ authState === "denied" ? "当前账户缺少相应权限" : "登录后显示访问上下文" }}</span>
            </div>
          </div>
        </section>
        </template>
      </main>
    </div>
  </div>
</template>
