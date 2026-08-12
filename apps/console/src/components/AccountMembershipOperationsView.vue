<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { accountName, accountStatusLabel } from "@/lib/account-labels";
import {
  fetchAccountMembership,
  grantAccountMembership,
  revokeAccountMembership,
  searchAccountMemberships,
  type ConsoleAccountMembership,
  type ConsoleMembershipAccountSummary,
  type ConsoleMembershipMutationRequest,
} from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type ListState = "idle" | "loading" | "ready" | "empty" | "invalid" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type PendingCommand = {
  kind: "grant" | "revoke";
  operatorID: string;
  userID: string;
  input: ConsoleMembershipMutationRequest;
  key: string;
  identity: { displayName?: string; email: string };
};
type LoadedMembership = ConsoleAccountMembership & { userID: string };

const searchQuery = ref("");
const activeQuery = ref("");
const accounts = ref<ConsoleMembershipAccountSummary[]>([]);
const account = ref<ConsoleMembershipAccountSummary>();
const membership = ref<LoadedMembership>();
const currentPage = ref(1);
const nextPage = ref<number | null>(null);
const workspaceState = ref<WorkspaceState>("loading");
const listState = ref<ListState>("idle");
const detailState = ref<DetailState>("idle");
const reason = ref("");
const feedback = ref("");
const busy = ref(false);
const confirm = ref<PendingCommand>();
const pendingStorageKey = "henukit.account-membership.pending-command";
const pending = ref<PendingCommand>();
const canWrite = computed(() => props.permissions.includes("account.membership.write"));
let listToken = 0;

function operationKey(kind: PendingCommand["kind"]) {
  return `idem_account_membership_${kind}_${crypto.randomUUID()}`;
}

function persistPending(value?: PendingCommand) {
  pending.value = value;
  if (value) sessionStorage.setItem(pendingStorageKey, JSON.stringify(value));
  else sessionStorage.removeItem(pendingStorageKey);
}

function restorePending(operatorID: string) {
  if (pending.value?.operatorID === operatorID) return;
  pending.value = undefined;
  feedback.value = "";
  try {
    const stored = JSON.parse(sessionStorage.getItem(pendingStorageKey) ?? "null") as Partial<PendingCommand> | null;
    if (!stored || (stored.kind !== "grant" && stored.kind !== "revoke") || stored.operatorID !== operatorID || typeof stored.userID !== "string" || typeof stored.key !== "string" || !stored.input || typeof stored.input !== "object" || !stored.identity || typeof stored.identity !== "object" || typeof stored.identity.email !== "string" || !stored.identity.email) {
      sessionStorage.removeItem(pendingStorageKey);
      return;
    }
    const input = stored.input as ConsoleMembershipMutationRequest;
    if (typeof input.reason === "string" && typeof input.expected_version === "number") {
      pending.value = stored as PendingCommand;
      feedback.value = "发现一项结果未确认的会员操作；可按原请求重试。";
    }
  } catch {
    sessionStorage.removeItem(pendingStorageKey);
  }
}

function resetWorkspace() {
  listToken += 1;
  accounts.value = [];
  account.value = undefined;
  membership.value = undefined;
  listState.value = "idle";
  detailState.value = "idle";
  currentPage.value = 1;
  nextPage.value = null;
  confirm.value = undefined;
}

function membershipLabel(value: ConsoleAccountMembership) {
  return value.plan === "lifetime" && value.lifetime ? "终身会员" : "免费会员";
}

function accountMembershipLabel(value: ConsoleMembershipAccountSummary) {
  return value.membership ? membershipLabel(value.membership) : "未开通会员账户";
}

function selectAccount(value: ConsoleMembershipAccountSummary) {
  account.value = value;
  membership.value = value.membership ? { ...value.membership, userID: value.id } : undefined;
  detailState.value = value.membership ? "ready" : "not_found";
  reason.value = "";
  confirm.value = undefined;
  feedback.value = "";
}

async function loadAccounts(page = 1) {
  if (busy.value || workspaceState.value !== "ready") return;
  const token = ++listToken;
  listState.value = "loading";
  account.value = undefined;
  membership.value = undefined;
  detailState.value = "idle";
  confirm.value = undefined;
  if (page === 1) activeQuery.value = searchQuery.value.trim();
  const result = await searchAccountMemberships({ query: activeQuery.value, page });
  if (token !== listToken) return;
  if (result.state === "authenticated") {
    accounts.value = result.page.accounts;
    currentPage.value = page;
    nextPage.value = result.page.next_page;
    listState.value = accounts.value.length === 0 ? "empty" : "ready";
    return;
  }
  accounts.value = [];
  nextPage.value = null;
  if (result.state === "signed_out") {
    persistPending();
    workspaceState.value = "signed_out";
    return;
  }
  if (result.state === "denied") {
    persistPending();
    workspaceState.value = "denied";
    return;
  }
  listState.value = result.state === "invalid" ? "invalid" : "unavailable";
}

async function loadMembership(userID: string) {
  const selected = account.value;
  if (!selected || selected.id !== userID) return;
  detailState.value = "loading";
  membership.value = undefined;
  const result = await fetchAccountMembership(userID);
  if (account.value?.id !== userID) return;
  if (result.state === "authenticated") {
    membership.value = { ...result.membership, userID };
    account.value = { ...selected, membership: result.membership };
    accounts.value = accounts.value.map((item) => item.id === userID ? { ...item, membership: result.membership } : item);
    detailState.value = "ready";
    return;
  }
  detailState.value = result.state === "not_found" ? "not_found" : result.state === "invalid" ? "invalid" : "unavailable";
  if (result.state === "signed_out") {
    persistPending();
    workspaceState.value = "signed_out";
  }
  if (result.state === "denied") {
    persistPending();
    workspaceState.value = "denied";
  }
}

async function finish(command: PendingCommand) {
  if (props.authState !== "authenticated" || !props.operatorID || props.operatorID !== command.operatorID || !canWrite.value) {
    persistPending();
    feedback.value = "当前登录状态或操作员身份已变化，未执行原会员操作。";
    return;
  }
  busy.value = true;
  feedback.value = command.kind === "grant" ? "正在发放终身会员权益…" : "正在撤销终身会员权益…";
  const result = command.kind === "grant"
    ? await grantAccountMembership(command.userID, command.input, command.key)
    : await revokeAccountMembership(command.userID, command.input, command.key);
  if (props.authState !== "authenticated" || props.operatorID !== command.operatorID || !canWrite.value) {
    persistPending();
    feedback.value = "当前登录状态或操作员身份已变化，未确认原会员操作结果。";
    busy.value = false;
    return;
  }
  if (result.state === "succeeded") {
    membership.value = { ...result.membership, userID: command.userID };
    if (account.value?.id === command.userID) account.value = { ...account.value, membership: result.membership };
    accounts.value = accounts.value.map((item) => item.id === command.userID ? { ...item, membership: result.membership } : item);
    detailState.value = "ready";
    reason.value = "";
    persistPending();
    feedback.value = command.kind === "grant" ? "终身会员权益已发放。" : "终身会员权益已撤销。";
  } else if (result.state === "conflict") {
    persistPending();
    await loadMembership(command.userID);
    feedback.value = "会员版本已变化，已刷新最新权益；请基于最新版本重新操作。";
  } else if (result.state === "unavailable") {
    feedback.value = "结果还没确认；可对同一已确认目标按原请求重试，系统会避免重复执行。";
  } else {
    if (result.state === "signed_out" || result.state === "denied") persistPending();
    feedback.value = result.state === "denied"
      ? "当前账户缺少会员权益操作权限。"
      : result.state === "signed_out"
        ? "登录状态已过期，请重新登录后再操作。"
        : result.state === "not_found"
          ? "该用户还未开通会员账户，无法发放权益。"
          : result.state === "invalid"
            ? "请求内容无效，请检查填写后重试。"
            : "操作没有完成，请稍后刷新页面重试。";
  }
  busy.value = false;
}

// requestMutation opens the single confirmation step. Nothing is persisted or
// written until the operator confirms; the pending retry record is created
// only after confirmation, together with the exact identity snapshot the
// operator confirmed, so an unknown result can be retried without ambiguity.
function requestMutation() {
  const current = membership.value;
  const trimmedReason = reason.value.trim();
  if (!current || !trimmedReason || busy.value || !props.operatorID || account.value?.id !== current.userID) return;
  const kind: PendingCommand["kind"] = current.plan === "free" ? "grant" : "revoke";
  confirm.value = {
    kind,
    operatorID: props.operatorID,
    userID: current.userID,
    input: { reason: trimmedReason, expected_version: current.version },
    key: operationKey(kind),
    identity: { displayName: account.value.display_name, email: account.value.email },
  };
}

function confirmMutation() {
  const command = confirm.value;
  if (!command) return;
  confirm.value = undefined;
  persistPending(command);
  void finish(command);
}

function cancelMutation() {
  confirm.value = undefined;
  feedback.value = "已取消，未执行任何操作。";
}

watch(
  () => [props.authState, props.operatorID, canWrite.value] as const,
  ([authState, operatorID, hasPermission]) => {
    if (authState === "authenticated" && hasPermission && operatorID) {
      const shouldLoad = workspaceState.value !== "ready";
      workspaceState.value = "ready";
      restorePending(operatorID);
      if (shouldLoad) void loadAccounts(1);
      return;
    }
    if (authState === "loading") {
      workspaceState.value = "loading";
      resetWorkspace();
      return;
    }
    persistPending();
    workspaceState.value = authState === "signed_out" ? "signed_out" : authState === "denied" || !hasPermission ? "denied" : "unavailable";
    resetWorkspace();
  },
  { immediate: true },
);
</script>

<template>
  <section aria-labelledby="account-membership-heading">
    <PageHeader
      eyebrow="会员权益操作"
      title="会员权益运营"
      description="从用户列表选择一个账户，所有操作均以当前登录身份记录。"
      title-id="account-membership-heading"
    >
      <div class="access-context"><span>已记录审计的权益操作</span></div>
    </PageHeader>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block break-all text-sm">待重试操作：为「{{ accountName(pending.identity.displayName) }} · {{ pending.identity.email }}」{{ pending.kind === "grant" ? "发放" : "撤销" }}终身会员权益。这是刚才已确认的同一目标，重试会沿用原操作请求，系统会避免重复执行。</span>
      <Button v-if="pending && !busy" class="mt-3" @click="finish(pending)">按原请求重试</Button>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证会员操作权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有会员权益运营权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>会员权益服务暂时不可用，请稍后再试。</p></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(20rem,.9fr)_minmax(0,1.1fr)]">
      <section class="operation-panel !mt-0" aria-labelledby="membership-account-list-heading">
        <h2 id="membership-account-list-heading">选择用户</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">默认显示一页用户；可按显示名称（Display Name）或完整邮箱缩小结果。</p>
        <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="loadAccounts(1)">
          <Label class="min-w-0 flex-1 grid gap-2">
            显示名称或邮箱
            <Input v-model="searchQuery" autocomplete="off" placeholder="姓名或 student@henu.edu.cn" :disabled="busy" />
          </Label>
          <Button class="sm:self-end" type="submit" :disabled="busy || listState === 'loading'">搜索</Button>
        </form>

        <div class="mt-4" :data-membership-list-state="listState">
          <p v-if="listState === 'loading'" aria-busy="true">正在加载用户…</p>
          <p v-else-if="listState === 'empty'" class="text-muted-foreground">没有匹配的用户。可以清空搜索条件后重试。</p>
          <p v-else-if="listState === 'invalid'" class="text-muted-foreground">搜索内容无效，请检查后重试。</p>
          <div v-else-if="listState === 'unavailable'" class="text-muted-foreground"><p>用户与会员状态暂时不可用，请稍后再试。</p><Button class="mt-3" @click="loadAccounts(currentPage)">重新加载</Button></div>
          <div v-else-if="listState === 'ready'" class="grid gap-2">
            <button
              v-for="item in accounts"
              :key="item.id"
              type="button"
              class="w-full min-w-0 rounded-lg border border-border p-3 text-left transition hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :class="account?.id === item.id ? 'bg-muted ring-1 ring-ring' : ''"
              @click="selectAccount(item)"
            >
              <span class="block font-semibold">{{ accountName(item.display_name) }}</span>
              <span class="mt-1 block break-all text-sm text-muted-foreground">{{ item.email }}</span>
              <span class="mt-2 block text-sm">{{ accountMembershipLabel(item) }}</span>
            </button>
          </div>
        </div>

        <div v-if="listState === 'ready'" class="mt-4 flex flex-wrap items-center justify-between gap-3" aria-label="用户分页">
          <Button variant="outline" :disabled="currentPage <= 1" @click="loadAccounts(currentPage - 1)">上一页</Button>
          <span class="text-sm text-muted-foreground">第 {{ currentPage }} 页</span>
          <Button variant="outline" :disabled="nextPage === null" @click="nextPage && loadAccounts(nextPage)">下一页</Button>
        </div>
      </section>

      <Card class="!mt-0 min-w-0 p-4" :data-account-membership-detail-state="detailState" aria-labelledby="account-membership-detail-heading">
        <div v-if="!account" class="text-muted-foreground">从左侧列表选择一位用户后，可查看并编辑会员权益。</div>
        <template v-else>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="eyebrow">已选用户</p>
              <h2 id="account-membership-detail-heading" class="mt-1 text-xl font-bold">{{ accountName(account.display_name) }}</h2>
              <p class="mt-1 break-all text-sm text-muted-foreground">{{ account.email }} · 账户状态：{{ accountStatusLabel(account.status) }}。请核对姓名和邮箱后再操作。</p>
            </div>
          </div>

          <div v-if="detailState === 'loading'" class="mt-4" aria-busy="true">正在读取持久化会员权益…</div>
          <div v-else-if="detailState === 'not_found'" class="mt-4 text-muted-foreground">该用户尚未建立会员账户，暂时不能发放权益；请让用户先登录一次账户中心后重试。</div>
          <div v-else-if="detailState === 'invalid'" class="mt-4 text-muted-foreground">账户信息无效，请重新选择。</div>
          <div v-else-if="detailState === 'unavailable'" class="mt-4 text-muted-foreground"><p>会员权益暂不可用。</p><Button class="mt-3" @click="loadMembership(account.id)">重新加载</Button></div>
          <template v-else-if="membership && account.id === membership.userID">
            <div class="mt-4 flex flex-wrap items-start justify-between gap-3">
              <div>
                <p class="eyebrow">当前权益</p>
                <h3 class="mt-1 text-lg font-bold">{{ membershipLabel(membership) }}</h3>
              </div>
              <span class="rounded-full bg-muted px-3 py-1 text-sm">版本 {{ membership.version }}</span>
            </div>
            <p class="mt-3 text-sm leading-6 text-muted-foreground">每次授权或撤销都写入不可变审计事件，并向用户创建一条持久化通知。</p>

            <form v-if="!confirm" class="mt-5 border-t border-border pt-5" @submit.prevent="requestMutation">
              <Label class="grid gap-2">
                操作原因
                <Textarea v-model="reason" required maxlength="1000" rows="4" :placeholder="membership.plan === 'free' ? '说明为何发放终身权益。' : '说明为何撤销终身权益。'"></Textarea>
              </Label>
              <Button class="mt-3" type="submit" :disabled="busy || !reason.trim()">
                {{ membership.plan === 'free' ? '发放终身会员' : '撤销终身会员' }}
              </Button>
            </form>

            <div v-else class="mt-5 border-t border-border pt-5" data-membership-confirm-step>
              <p class="font-medium">确认{{ confirm.kind === "grant" ? "发放" : "撤销" }}终身会员权益？</p>
              <p class="mt-2 text-sm leading-6 text-muted-foreground">
                将向「{{ accountName(account.display_name) }} · {{ account.email }}」{{ confirm.kind === "grant" ? "发放" : "撤销" }}终身会员权益，写入不可变审计事件并向该用户创建持久化通知；提交后立即生效，之后可通过相反操作调整。
              </p>
              <div class="mt-3 flex flex-wrap gap-3">
                <Button :disabled="busy" @click="confirmMutation">确认执行</Button>
                <Button variant="outline" :disabled="busy" @click="cancelMutation">取消</Button>
              </div>
            </div>
          </template>
        </template>
      </Card>
    </div>
  </section>
</template>
