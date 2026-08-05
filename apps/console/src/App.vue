<script setup lang="ts">
import { Activity, Bell, BookOpen, Building2, LogOut, Menu, MessageSquare, ShieldCheck, Utensils, X } from "@lucide/vue";
import { AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogOverlay, AlertDialogPortal, AlertDialogRoot, AlertDialogTitle, DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from "reka-ui";
import { computed, nextTick, onMounted, ref } from "vue";

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
// The sidebar's module-summary links are `#module-<id>` anchors into the
// ModuleCard grid that only exists in the DOM on the overview page (every
// operations sub-page renders a dedicated view instead, see <main> below).
// Showing them elsewhere put two full navigation lists in the sidebar at
// once — the granted-permission shortcuts and a set of anchors with nothing
// to scroll to — which is exactly what read as a tangled, duplicated menu.
const isOverviewPage = !isPlatformOperations && !isNoticeOperations && !isLibraryOperations && !isFoodOperations && !isAccountMembershipOperations && !isAccountPointOperations && !isAccountTicketOperations;
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
  if (result.state === "authenticated" && isOverviewPage) {
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

// Signing out (or switching accounts) must survive a confirmation dialog, so
// the destructive call only runs after the operator confirms; the dialog copy
// differs between a normal sign-out and a switch-account sign-out.
const pendingSignOut = ref<"signout" | "switch_account" | null>(null);
const signOutDialogOpen = computed({
  get: () => pendingSignOut.value !== null,
  set: (open: boolean) => {
    if (!open) pendingSignOut.value = null;
  },
});

function requestSignOut(kind: "signout" | "switch_account") {
  mobileNavigationOpen.value = false;
  // Let the mobile dialog unmount before the confirmation opens so the two
  // reka-ui dialogs never fight over focus or dismissal.
  void nextTick(() => {
    pendingSignOut.value = kind;
  });
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

async function confirmSignOut() {
  await signOut();
  pendingSignOut.value = null;
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

// "可见" means the card actually shows content: denied cards are hidden behind
// a login wall and unavailable cards are blank (e.g. the whole overview being
// down renders every card "概览数据暂时不可用"), so neither counts.
const visibleCount = computed(() => summaries.value.filter((summary) => summary.status !== "denied" && summary.status !== "unavailable").length);

// One source of truth for the operations destinations. The desktop sidebar and
// the mobile dialog previously repeated this list verbatim, so a permission or
// label fix had to be made twice and the two drifted apart.
const operationsNav = computed(() =>
  [
    { href: operationsHref, label: "平台运营工作台", icon: ShieldCheck, permission: "platform.operations.read", active: isPlatformOperations },
    { href: noticesHref, label: "通知审核与分发", icon: Bell, permission: "notice.read", active: isNoticeOperations },
    { href: libraryHref, label: "资料库运营", icon: BookOpen, permission: "library.read", active: isLibraryOperations },
    { href: foodHref, label: "美食运营", icon: Utensils, permission: "food.read", active: isFoodOperations },
    { href: accountMembershipsHref, label: "会员权益运营", icon: ShieldCheck, permission: "account.membership.write", active: isAccountMembershipOperations },
    { href: accountPointsHref, label: "积分账本运营", icon: Activity, permission: "account.points.adjust", active: isAccountPointOperations },
    { href: accountTicketsHref, label: "账户工单运营", icon: MessageSquare, permission: "account.tickets.read", active: isAccountTicketOperations },
  ].filter((item) => consoleSession.value?.access_context.permissions.includes(item.permission)),
);
</script>

<template>
  <div class="min-h-screen bg-background text-foreground" data-console-shell>
    <aside
      class="fixed inset-y-0 left-0 z-20 hidden w-64 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground lg:flex"
      aria-label="Console 主导航"
    >
      <div class="flex h-16 items-center gap-2.5 border-b border-sidebar-border px-5">
        <div class="grid size-7 shrink-0 place-items-center rounded-md bg-primary text-xs font-semibold text-primary-foreground">H</div>
        <span class="truncate text-sm font-semibold tracking-tight">HENUKit Console</span>
      </div>

      <nav class="flex-1 overflow-y-auto p-3" aria-label="产品模块">
        <p class="px-2 pb-1.5 text-xs font-medium text-muted-foreground">运营</p>
        <ul class="grid gap-0.5">
          <li v-for="item in operationsNav" :key="item.href">
            <a
              :href="item.href"
              :aria-current="item.active ? 'page' : undefined"
              :class="[
                'flex min-h-9 items-center gap-2.5 rounded-md px-2 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                item.active
                  ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
                  : 'text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
              ]"
            >
              <component :is="item.icon" :size="16" aria-hidden="true" class="shrink-0" />
              <span class="truncate">{{ item.label }}</span>
            </a>
          </li>
        </ul>

        <template v-if="isOverviewPage">
          <p class="px-2 pt-4 pb-1.5 text-xs font-medium text-muted-foreground">本页模块</p>
          <ul class="grid gap-0.5">
            <li v-for="module in moduleSummaries" :key="module.id">
              <a
                :href="`#module-${module.id}`"
                class="flex min-h-9 items-center gap-2.5 rounded-md px-2 text-sm text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              >
                <component :is="icons[module.id]" :size="16" aria-hidden="true" class="shrink-0" />
                <span class="truncate">{{ module.name }}</span>
              </a>
            </li>
          </ul>
        </template>
      </nav>

      <p class="border-t border-sidebar-border px-5 py-3 text-xs leading-5 text-muted-foreground">
        学生自主运营<br />非河南大学官方项目
      </p>
    </aside>

    <div class="console-main lg:ml-64">
      <header
        class="sticky top-0 z-30 flex min-h-14 items-center gap-2 border-b border-border bg-background/85 px-4 backdrop-blur sm:px-6"
      >
        <DialogRoot v-model:open="mobileNavigationOpen">
          <DialogTrigger as-child>
            <Button variant="ghost" size="icon" class="lg:hidden" aria-label="打开产品导航">
              <Menu :size="20" />
            </Button>
          </DialogTrigger>
          <DialogPortal>
            <DialogOverlay class="fixed inset-0 z-40 bg-foreground/30 backdrop-blur-sm" />
            <DialogContent class="fixed inset-y-0 left-0 z-50 flex w-[min(84vw,18rem)] flex-col bg-background p-4 shadow-lg">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <DialogTitle class="text-sm font-semibold">产品模块</DialogTitle>
                  <DialogDescription class="mt-0.5 text-xs text-muted-foreground">专属工作台与 {{ summaries.length }} 个已确认的运营模块</DialogDescription>
                </div>
                <DialogClose as-child>
                  <Button variant="ghost" size="icon" aria-label="关闭产品导航"><X :size="18" /></Button>
                </DialogClose>
              </div>
              <nav class="mt-4 grid gap-0.5 overflow-y-auto" aria-label="移动端产品模块">
                <DialogClose v-for="item in operationsNav" :key="item.href" as-child>
                  <a
                    :href="item.href"
                    :aria-current="item.active ? 'page' : undefined"
                    :class="[
                      'flex min-h-10 items-center gap-2.5 rounded-md px-2 text-sm',
                      item.active ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                    ]"
                  >
                    <component :is="item.icon" :size="16" aria-hidden="true" class="shrink-0" />
                    <span class="truncate">{{ item.label }}</span>
                  </a>
                </DialogClose>
                <template v-if="isOverviewPage">
                  <DialogClose v-for="module in moduleSummaries" :key="module.id" as-child>
                    <a
                      :href="`#module-${module.id}`"
                      class="flex min-h-10 items-center gap-2.5 rounded-md px-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                    >
                      <component :is="icons[module.id]" :size="16" aria-hidden="true" class="shrink-0" />
                      <span class="truncate">{{ module.name }}</span>
                    </a>
                  </DialogClose>
                </template>
              </nav>
              <div v-if="authState === 'authenticated' || authState === 'denied'" class="mt-4 border-t border-border pt-3">
                <DialogClose as-child>
                  <Button
                    variant="ghost"
                    class="w-full justify-start"
                    @click="requestSignOut(authState === 'denied' ? 'switch_account' : 'signout')"
                  >
                    <LogOut :size="16" aria-hidden="true" class="shrink-0" />
                    {{ authState === "denied" ? "换个账户登录" : "退出登录" }}
                  </Button>
                </DialogClose>
              </div>
            </DialogContent>
          </DialogPortal>
        </DialogRoot>

        <div class="ml-auto flex items-center gap-2">
          <StatusBadge v-if="authState === 'loading'" status="loading">正在验证登录状态</StatusBadge>
          <template v-else-if="authState === 'authenticated'">
            <StatusBadge status="ok">权限已验证</StatusBadge>
            <Button variant="outline" size="sm" @click="requestSignOut('signout')">退出登录</Button>
          </template>
          <template v-else-if="authState === 'denied'">
            <StatusBadge status="denied">权限不足</StatusBadge>
            <Button variant="outline" size="sm" @click="requestSignOut('switch_account')">换个账户登录</Button>
          </template>
          <Button v-else-if="authState === 'unavailable'" size="sm" @click="refreshSession">重试连接</Button>
          <Button v-else as="a" size="sm" :href="consoleLoginHref()">登录 Console</Button>
        </div>
      </header>

      <main class="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 lg:px-8">
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

        <section class="mt-6 rounded-lg border border-border bg-card p-5" aria-labelledby="permission-heading">
          <h2 id="permission-heading" class="text-sm font-semibold">当前账户的权限</h2>
          <p class="mt-1 text-sm text-muted-foreground">登录状态与权限由服务端在每次请求时验证，前端不自行判断权限。</p>
          <!-- This used to render two <code> pills of filler prose ("已授予概览查看权限")
               under an aria-label promising the verified permission codes, while the real
               access_context.permissions array was never displayed at all. -->
          <div class="mt-3 flex flex-wrap gap-1.5" aria-label="服务端验证的权限代码">
            <code v-for="permission in consoleSession?.access_context.permissions ?? []" :key="permission">{{ permission }}</code>
            <span v-if="!consoleSession" class="text-sm text-muted-foreground">
              {{ authState === "denied" ? "当前账户缺少相应权限" : "登录后显示访问上下文" }}
            </span>
          </div>
        </section>
        </template>
      </main>
    </div>

    <AlertDialogRoot v-model:open="signOutDialogOpen">
      <AlertDialogPortal>
        <AlertDialogOverlay class="fixed inset-0 z-40 bg-foreground/30 backdrop-blur-sm" />
        <AlertDialogContent
          class="fixed left-1/2 top-1/2 z-50 w-[min(90vw,24rem)] -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-background p-5 shadow-lg"
        >
          <AlertDialogTitle class="text-sm font-semibold">
            {{ pendingSignOut === "switch_account" ? "换个账户登录？" : "退出登录？" }}
          </AlertDialogTitle>
          <AlertDialogDescription class="mt-1.5 text-sm leading-6 text-muted-foreground">
            {{ pendingSignOut === "switch_account" ? "将退出当前账户并返回登录页，可以选择其他账户登录。" : "将退出当前登录，未保存的更改不会保留。" }}
          </AlertDialogDescription>
          <div class="mt-4 flex justify-end gap-2">
            <AlertDialogCancel as-child>
              <Button variant="outline" size="sm">取消</Button>
            </AlertDialogCancel>
            <AlertDialogAction as-child>
              <Button size="sm" @click="confirmSignOut">
                {{ pendingSignOut === "switch_account" ? "换个账户登录" : "确认退出" }}
              </Button>
            </AlertDialogAction>
          </div>
        </AlertDialogContent>
      </AlertDialogPortal>
    </AlertDialogRoot>
  </div>
</template>
