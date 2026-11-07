<script setup lang="ts">
import { Activity, Bell, BookOpen, LayoutDashboard, LogOut, Menu, MessageSquare, ShieldCheck, Utensils, X } from "@lucide/vue";
import { AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogOverlay, AlertDialogPortal, AlertDialogRoot, AlertDialogTitle, DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from "reka-ui";
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";

import OverviewWorkspace from "@/components/OverviewWorkspace.vue";
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

import { beginConsolePrincipal, isCurrentConsolePrincipal, onConsoleSessionExpired } from "@/lib/console-session";

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
const overviewHref = consolePath("/");
const isOverviewPage = !isPlatformOperations && !isNoticeOperations && !isLibraryOperations && !isFoodOperations && !isAccountMembershipOperations && !isAccountPointOperations && !isAccountTicketOperations;
const loading = query.get("scenario") === "loading";
const mobileNavigationOpen = ref(false);
const authState = ref<"loading" | "authenticated" | "signed_out" | "denied" | "unavailable">("loading");
const principalGeneration = ref(0);
const sessionExpired = ref(false);
const consoleSession = ref<ConsoleSession>();
const consoleOverview = ref<ConsoleOverview>();
const overviewState = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
let overviewController: AbortController | undefined;
async function readOverview() {
  overviewController?.abort();
  const controller = new AbortController();
  overviewController = controller;
  return new Promise<Awaited<ReturnType<typeof fetchConsoleOverview>>>((resolve) => {
    let settled = false;
    const finish = (result: Awaited<ReturnType<typeof fetchConsoleOverview>>) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      controller.signal.removeEventListener("abort", expired);
      resolve(result);
    };
    const expired = () => finish({ state: "unavailable" });
    const timer = setTimeout(() => controller.abort(), 3000);
    controller.signal.addEventListener("abort", expired, { once: true });
    void fetchConsoleOverview({ signal: controller.signal }).then(finish, () => finish({ state: "unavailable" }));
  });
}

async function refreshSession() {
  overviewController?.abort();
  const generation = beginConsolePrincipal();
  principalGeneration.value = generation;
  authState.value = "loading";
  consoleSession.value = undefined;
  consoleOverview.value = undefined;
  const result = await fetchConsoleSession();
  if (!isCurrentConsolePrincipal(generation)) return;
  sessionExpired.value = false;
  authState.value = result.state;
  consoleSession.value = result.state === "authenticated" ? result.session : undefined;
  consoleOverview.value = undefined;
  if (result.state === "authenticated" && isOverviewPage) {
    if (!result.session.access_context.permissions.includes("console.overview.read") || !result.session.access_context.scopes.some((scope) => scope.kind === "platform")) {
      overviewState.value = "denied";
      return;
    }
    overviewState.value = "loading";
    const overviewResult = await readOverview();
    if (!isCurrentConsolePrincipal(generation)) return;
    if (overviewResult.state === "authenticated") {
      consoleOverview.value = overviewResult.overview;
      overviewState.value = "ready";
    } else if (overviewResult.state === "signed_out") {
      authState.value = overviewResult.state;
      consoleSession.value = undefined;
      overviewState.value = "unavailable";
    } else if (overviewResult.state === "denied") {
      overviewState.value = "denied";
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
  overviewController?.abort();
  const generation = beginConsolePrincipal();
  principalGeneration.value = generation;
  authState.value = "loading";
  consoleSession.value = undefined;
  consoleOverview.value = undefined;
  sessionExpired.value = false;
  try {
    await logoutConsoleSession();
    if (!isCurrentConsolePrincipal(generation)) return;
    authState.value = "signed_out";
    consoleSession.value = undefined;
    consoleOverview.value = undefined;
  } catch {
    if (isCurrentConsolePrincipal(generation)) authState.value = "unavailable";
  }
}

async function confirmSignOut() {
  await signOut();
  pendingSignOut.value = null;
}

const stopExpiryListener = onConsoleSessionExpired((generation) => {
  overviewController?.abort();
  principalGeneration.value = generation;
  sessionExpired.value = authState.value === "authenticated";
  authState.value = "signed_out";
  consoleSession.value = undefined;
  consoleOverview.value = undefined;
  mobileNavigationOpen.value = false;
  pendingSignOut.value = null;
});
onMounted(refreshSession);
onUnmounted(() => {
  overviewController?.abort();
  stopExpiryListener();
  if (isCurrentConsolePrincipal(principalGeneration.value)) beginConsolePrincipal();
});

function publicModuleStatusMessage(
  moduleId: ModuleSummary["id"],
  status: ModuleSummary["status"],
  unavailableReason: ModuleSummary["unavailableReason"],
  message: string,
): string {
  if (moduleId !== "quizcraft") return message;
  if (unavailableReason === "not_onboarded") return "练习服务摘要尚未接入；管理入口已停用";
  if (unavailableReason === "operator_disabled") return "练习服务摘要已停用";
  return {
    ok: "练习服务摘要可用",
    empty: "当前暂无练习摘要",
    partial: "练习服务仅部分可用",
    stale: "展示最近一次练习摘要",
    unavailable: "练习服务摘要暂不可用",
    loading: "正在读取练习摘要",
    denied: "无权查看练习摘要",
  }[status];
}

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
                  statusMessage: publicModuleStatusMessage(presentation.id, live.status, live.unavailable_reason, live.status_message),
                  unavailableReason: live.unavailable_reason,
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

// Canonical Console destinations, also reused by the Overview evidence panel.
// Each remains hidden unless the server-verified access context grants it.
const consoleNav = computed(() =>
  [
    { href: overviewHref, label: "全局平台概览", icon: LayoutDashboard, permission: "console.overview.read", active: isOverviewPage },
    { href: operationsHref, label: "平台运营工作台", icon: ShieldCheck, permission: "platform.operations.read", active: isPlatformOperations },
    { href: noticesHref, label: "校园通知审核与分发", icon: Bell, permission: "notice.read", product: "notice", active: isNoticeOperations },
    { href: libraryHref, label: "资料库运营", icon: BookOpen, permission: "library.read", product: "library", active: isLibraryOperations },
    { href: foodHref, label: "美食运营", icon: Utensils, permission: "food.read", product: "food", active: isFoodOperations },
    { href: accountMembershipsHref, label: "会员权益运营", icon: ShieldCheck, permission: "account.membership.write", product: "account-portfolio", active: isAccountMembershipOperations },
    { href: accountPointsHref, label: "积分调整", icon: Activity, permission: "account.points.adjust", product: "account-portfolio", active: isAccountPointOperations },
    { href: accountTicketsHref, label: "账户工单运营", icon: MessageSquare, permission: "account.tickets.read", product: "account-portfolio", active: isAccountTicketOperations },
  ].filter((item) => consoleSession.value?.access_context.permissions.includes(item.permission)
    && consoleSession.value.access_context.scopes.some((scope) => item.product ? scope.kind === "product" && scope.product_code === item.product : scope.kind === "platform")),
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

      <nav class="flex-1 overflow-y-auto p-3" aria-label="运营导航">
        <p class="px-2 pb-1.5 text-xs font-medium text-muted-foreground">运营</p>
        <ul class="grid gap-0.5">
          <li v-for="item in consoleNav" :key="item.href">
            <a
              :href="item.href"
              :aria-current="item.active ? 'page' : undefined"
              :class="[
                'flex min-h-11 items-center gap-2.5 rounded-md px-2 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
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
            <Button variant="ghost" size="icon" class="lg:hidden" aria-label="打开运营导航">
              <Menu :size="20" />
            </Button>
          </DialogTrigger>
          <DialogPortal>
            <DialogOverlay class="fixed inset-0 z-40 bg-foreground/30 backdrop-blur-sm" />
            <DialogContent class="fixed inset-y-0 left-0 z-50 flex w-[min(84vw,18rem)] flex-col bg-background p-4 shadow-lg">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <DialogTitle class="text-sm font-semibold">运营导航</DialogTitle>
                  <DialogDescription class="mt-0.5 text-xs text-muted-foreground">已获授权的概览与运营工作台</DialogDescription>
                </div>
                <DialogClose as-child>
                  <Button variant="ghost" size="icon" aria-label="关闭运营导航"><X :size="18" /></Button>
                </DialogClose>
              </div>
              <nav class="mt-4 grid gap-0.5 overflow-y-auto" aria-label="移动端运营导航">
                <DialogClose v-for="item in consoleNav" :key="item.href" as-child>
                  <a
                    :href="item.href"
                    :aria-current="item.active ? 'page' : undefined"
                    :class="[
                      'flex min-h-11 items-center gap-2.5 rounded-md px-2 text-sm',
                      item.active ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                    ]"
                  >
                    <component :is="item.icon" :size="16" aria-hidden="true" class="shrink-0" />
                    <span class="truncate">{{ item.label }}</span>
                  </a>
                </DialogClose>
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

      <main :key="principalGeneration" class="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 lg:px-8" :class="isOverviewPage && 'console-overview-main'">
        <PageHeader v-if="isOverviewPage && authState !== 'authenticated'" title="全局总览" title-id="overview-heading" />
        <p v-if="authState === 'signed_out'" role="status" class="mb-4 text-sm text-muted-foreground">{{ sessionExpired ? "登录已过期，请重新登录后继续。" : "请登录后继续，登录成功后将回到当前页面。" }}</p>
        <template v-if="authState === 'authenticated'">
        <PlatformOperationsView v-if="isPlatformOperations" :key="`platform-${consoleSession?.user.id ?? 'signed-out'}`" :auth-state="authState" :operator-i-d="consoleSession?.user.id" />
        <NoticeOperationsView v-else-if="isNoticeOperations" :key="`notice-${consoleSession?.user.id ?? 'signed-out'}`" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <LibraryOperationsView v-else-if="isLibraryOperations" :auth-state="authState" :permissions="consoleSession?.access_context.permissions ?? []" />
        <FoodOperationsView v-else-if="isFoodOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountMembershipOperationsView v-else-if="isAccountMembershipOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountPointOperationsView v-else-if="isAccountPointOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <AccountTicketOperationsView v-else-if="isAccountTicketOperations" :auth-state="authState" :operator-i-d="consoleSession?.user.id" :permissions="consoleSession?.access_context.permissions ?? []" />
        <template v-else>
        <section v-if="overviewState === 'denied'" aria-labelledby="overview-heading">
          <PageHeader title="全局总览" title-id="overview-heading" />
          <p class="mt-4 text-sm text-muted-foreground" role="status">当前账户没有总览查看权限。你仍可进入已获授权的工作台。</p>
          <nav class="mt-4 flex flex-wrap gap-3" aria-label="可用工作台"><Button v-for="item in consoleNav.filter((item) => item.permission !== 'console.overview.read')" :key="item.href" as="a" variant="outline" :href="item.href">{{ item.label }}</Button></nav>
        </section>
        <OverviewWorkspace v-else :summaries="summaries" :overview-state="overviewState" :permissions="consoleSession?.access_context.permissions ?? []" :scopes="consoleSession?.access_context.scopes ?? []" :navigation="consoleNav" @refresh="refreshSession" />
        </template>
        </template>
        <p v-else-if="authState === 'loading'" role="status" aria-busy="true" class="text-sm text-muted-foreground">正在验证登录状态…</p>
        <p v-else-if="authState === 'denied'" role="status" class="text-sm text-muted-foreground">当前账户没有此工作台的访问权限，请联系管理员或换个账户登录。</p>
        <p v-else-if="authState === 'unavailable'" role="status" class="text-sm text-muted-foreground">暂时无法验证访问权限，请重试连接。</p>
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
