<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import {
  fetchAccountMembership,
  grantAccountMembership,
  lookupConsoleAccount,
  revokeAccountMembership,
  type ConsoleAccountMembership,
  type ConsoleLookedUpAccount,
  type ConsoleMembershipMutationRequest,
} from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type LookupState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type PendingCommand = {
  kind: "grant" | "revoke";
  operatorID: string;
  userID: string;
  input: ConsoleMembershipMutationRequest;
  key: string;
};
type LoadedMembership = ConsoleAccountMembership & { userID: string };

const targetEmail = ref("");
const account = ref<ConsoleLookedUpAccount>();
const membership = ref<LoadedMembership>();
const workspaceState = ref<WorkspaceState>("loading");
const lookupState = ref<LookupState>("idle");
const detailState = ref<DetailState>("idle");
const reason = ref("");
const feedback = ref("");
const busy = ref(false);
const confirm = ref<PendingCommand>();
const pendingStorageKey = "henukit.account-membership.pending-command";
const pending = ref<PendingCommand>();
const canWrite = computed(() => props.permissions.includes("account.membership.write"));
let lookupToken = 0;

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
    if (!stored || (stored.kind !== "grant" && stored.kind !== "revoke") || stored.operatorID !== operatorID || typeof stored.userID !== "string" || typeof stored.key !== "string" || !stored.input || typeof stored.input !== "object") {
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

function resetLookup() {
  lookupToken += 1;
  account.value = undefined;
  membership.value = undefined;
  lookupState.value = "idle";
  detailState.value = "idle";
  confirm.value = undefined;
}

function accountStatusLabel(status: string) {
  if (status === "active") return "正常";
  if (status === "suspended") return "已停用";
  if (status === "deleted") return "已删除";
  return status;
}

// accountName echoes the resolved display name; legacy rows may carry an empty
// label, which must never render as a blank confirmation target.
function accountName(value?: string) {
  return value && value.trim().length > 0 ? value : "（未设置姓名）";
}

function membershipLabel(value: ConsoleAccountMembership) {
  return value.plan === "lifetime" && value.lifetime ? "终身会员" : "免费会员";
}

async function lookupAccount() {
  const email = targetEmail.value.trim();
  if (!email || busy.value) return;
  const token = ++lookupToken;
  lookupState.value = "loading";
  account.value = undefined;
  membership.value = undefined;
  detailState.value = "idle";
  confirm.value = undefined;
  feedback.value = "";
  const result = await lookupConsoleAccount(email);
  if (token !== lookupToken || targetEmail.value.trim() !== email) return;
  if (result.state === "authenticated") {
    if (!result.account) {
      lookupState.value = "not_found";
      return;
    }
    account.value = result.account;
    lookupState.value = "ready";
    await loadMembership(result.account.id);
    return;
  }
  if (result.state === "signed_out") {
    persistPending();
    workspaceState.value = "signed_out";
    lookupState.value = "idle";
    return;
  }
  if (result.state === "denied") {
    persistPending();
    workspaceState.value = "denied";
    lookupState.value = "idle";
    return;
  }
  lookupState.value = result.state === "invalid" ? "invalid" : "unavailable";
}

async function loadMembership(userID: string) {
  const token = lookupToken;
  detailState.value = "loading";
  membership.value = undefined;
  const result = await fetchAccountMembership(userID);
  if (token !== lookupToken || account.value?.id !== userID) return;
  if (result.state === "authenticated") {
    membership.value = { ...result.membership, userID };
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
    detailState.value = "ready";
    reason.value = "";
    persistPending();
    feedback.value = command.kind === "grant" ? "终身会员权益已发放。" : "终身会员权益已撤销。";
  } else if (result.state === "conflict") {
    persistPending();
    await loadMembership(command.userID);
    feedback.value = "会员版本已变化，已刷新最新权益；请基于最新版本重新操作。";
  } else if (result.state === "unavailable") {
    feedback.value = "结果还没确认，可点下方按钮按原请求重试。";
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
// only after confirmation, and it stores the resolved account id, never the
// email used to look the account up.
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
      workspaceState.value = "ready";
      restorePending(operatorID);
      return;
    }
    if (authState === "loading") {
      workspaceState.value = "loading";
      resetLookup();
      return;
    }
    persistPending();
    workspaceState.value = authState === "signed_out" ? "signed_out" : authState === "denied" || !hasPermission ? "denied" : "unavailable";
    resetLookup();
  },
  { immediate: true },
);
</script>

<template>
  <section aria-labelledby="account-membership-heading">
    <PageHeader
      eyebrow="会员权益操作"
      title="会员权益运营"
      description="所有操作均以当前登录身份记录。"
      title-id="account-membership-heading"
    >
      <div class="access-context"><span>已记录审计的权益操作</span></div>
    </PageHeader>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">待确认操作：{{ pending.kind === "grant" ? "向" : "从" }}用户 <code>{{ pending.userID }}</code>{{ pending.kind === "grant" ? "发放" : "撤销" }}终身会员权益。</span>
      <Button v-if="pending && !busy" class="mt-3" @click="finish(pending)">确认并按原请求重试</Button>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证会员操作权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有会员权益运营权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>会员权益服务暂时不可用，请稍后再试。</p></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <form class="operation-panel !mt-0" @submit.prevent="lookupAccount">
        <h2>按邮箱查找账户</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">输入完整邮箱查找账户，核对姓名后再执行发放或撤销。</p>
        <Label class="mt-5 grid gap-2">
          完整邮箱
          <Input v-model="targetEmail" required type="email" inputmode="email" autocomplete="off" placeholder="student@stu.henu.edu.cn" :disabled="busy || lookupState === 'loading'" @input="resetLookup" />
        </Label>
        <Button class="mt-4" type="submit" :disabled="busy || lookupState === 'loading' || !targetEmail.trim()">查找账户</Button>
      </form>

      <Card class="!mt-0 p-4" :data-account-lookup-state="lookupState" :data-account-membership-detail-state="detailState" aria-labelledby="account-membership-detail-heading">
        <div v-if="lookupState === 'idle'" class="text-muted-foreground">输入完整邮箱后，这里会先回显账户姓名供核对。</div>
        <div v-else-if="lookupState === 'loading'" aria-busy="true">正在查找账户…</div>
        <div v-else-if="lookupState === 'not_found'" class="text-muted-foreground">没有找到该邮箱对应的账户。请核对邮箱后重试；这不是服务不可用。</div>
        <div v-else-if="lookupState === 'invalid'" class="text-muted-foreground">邮箱格式不对，请检查后重试。</div>
        <div v-else-if="lookupState === 'unavailable'" class="text-muted-foreground"><p>账户查找服务暂时不可用，请稍后再试。</p><Button class="mt-3" @click="lookupAccount">重新查找</Button></div>
        <template v-else-if="account">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="eyebrow">账户核对</p>
              <h2 id="account-membership-detail-heading" class="mt-1 text-xl font-bold">{{ accountName(account.display_name) }}</h2>
              <p class="mt-1 text-sm text-muted-foreground">账户状态：{{ accountStatusLabel(account.status) }}。请核对姓名后再操作。</p>
            </div>
          </div>

          <div v-if="detailState === 'loading'" class="mt-4" aria-busy="true">正在读取持久化会员权益…</div>
          <div v-else-if="detailState === 'not_found'" class="mt-4 text-muted-foreground">该账户还未开通会员账户，无法发放权益。</div>
          <div v-else-if="detailState === 'invalid'" class="mt-4 text-muted-foreground">账户信息无效，请重新查找。</div>
          <div v-else-if="detailState === 'unavailable'" class="mt-4 text-muted-foreground"><p>会员权益暂不可用。</p><Button class="mt-3" @click="account && loadMembership(account.id)">重新加载</Button></div>
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
                将向「{{ accountName(account.display_name) }}」{{ confirm.kind === "grant" ? "发放" : "撤销" }}终身会员权益，写入不可变审计事件并向该用户创建持久化通知；此操作不可撤销。
              </p>
              <div class="mt-3 flex gap-3">
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
