<script setup lang="ts">
import { computed, ref, watch } from "vue";

import UiButton from "@/components/ui/UiButton.vue";
import {
  fetchAccountMembership,
  grantAccountMembership,
  revokeAccountMembership,
  type ConsoleAccountMembership,
  type ConsoleMembershipMutationRequest,
} from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type PendingCommand = {
  kind: "grant" | "revoke";
  operatorID: string;
  userID: string;
  input: ConsoleMembershipMutationRequest;
  key: string;
};
type LoadedMembership = ConsoleAccountMembership & { userID: string };

const targetUserID = ref("");
const membership = ref<LoadedMembership>();
const workspaceState = ref<WorkspaceState>("loading");
const detailState = ref<DetailState>("idle");
const reason = ref("");
const feedback = ref("");
const busy = ref(false);
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
  membership.value = undefined;
  detailState.value = "idle";
}

function membershipLabel(value: ConsoleAccountMembership) {
  return value.plan === "lifetime" && value.lifetime ? "终身会员" : "免费会员";
}

async function lookupMembership(options: { allowWhileBusy?: boolean } = {}) {
  const userID = targetUserID.value.trim();
  if (!userID || (busy.value && !options.allowWhileBusy)) return;
  const token = ++lookupToken;
  detailState.value = "loading";
  membership.value = undefined;
  feedback.value = "";
  const result = await fetchAccountMembership(userID);
  if (token !== lookupToken || targetUserID.value.trim() !== userID) return;
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
  if (targetUserID.value.trim() !== command.userID) {
    targetUserID.value = command.userID;
    resetLookup();
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
    await lookupMembership({ allowWhileBusy: true });
    feedback.value = "会员版本已变化，已刷新最新权益；请基于最新版本重新操作。";
  } else if (result.state === "unavailable") {
    feedback.value = "结果尚未确认，已保留幂等键；可按原请求重试。";
  } else {
    if (result.state === "signed_out" || result.state === "denied") persistPending();
    feedback.value = result.state === "denied"
      ? "当前账户缺少会员权益操作权限。"
      : result.state === "signed_out"
        ? "登录状态已过期，请重新登录后再操作。"
      : result.state === "not_found"
        ? "该用户尚未初始化 Account Portfolio，不能创建虚构账户。"
        : result.state === "invalid"
          ? "请求内容无效。"
          : "操作未完成。";
  }
  busy.value = false;
}

async function submitMutation() {
  const current = membership.value;
  const trimmedReason = reason.value.trim();
  if (!current || !trimmedReason || busy.value || !props.operatorID || targetUserID.value.trim() !== current.userID) return;
  const kind: PendingCommand["kind"] = current.plan === "free" ? "grant" : "revoke";
  const command: PendingCommand = {
    kind,
    operatorID: props.operatorID,
    userID: current.userID,
    input: { reason: trimmedReason, expected_version: current.version },
    key: operationKey(kind),
  };
  persistPending(command);
  await finish(command);
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
    <div class="overview-hero">
      <div>
        <p class="eyebrow">Account Portfolio operations</p>
        <h1 id="account-membership-heading" class="mt-2 text-2xl font-bold sm:text-3xl">会员权益运营</h1>
        <p class="mt-2 max-w-3xl leading-7 text-[var(--hk-ink-muted)]">仅通过经验证的 Console Session 操作已初始化账户的真实会员权益；浏览器不能指定操作员身份。</p>
      </div>
      <div class="access-context"><span>scope:product/account-portfolio</span><strong>audited entitlement</strong></div>
    </div>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">待确认操作：{{ pending.kind === "grant" ? "向" : "从" }}用户 <code>{{ pending.userID }}</code>{{ pending.kind === "grant" ? "发放" : "撤销" }}终身会员权益。</span>
      <UiButton v-if="pending && !busy" class="mt-3" @click="finish(pending)">确认并按原请求重试</UiButton>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证 Account Portfolio 会员操作权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户缺少 Account Portfolio 产品 Scope 或 `account.membership.write` 权限。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>会员权益操作暂不可用。</p></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <form class="operation-panel !mt-0" @submit.prevent="() => lookupMembership()">
        <h2>查找已初始化账户</h2>
        <p class="mt-2 text-sm leading-6 text-[var(--hk-ink-muted)]">目标用户必须先通过其认证后的 Account Portfolio 读取完成初始化；Console 不会为任意 UUID 创建账户。</p>
        <label class="mt-5 grid gap-2 text-sm font-semibold">
          用户 ID
          <input v-model="targetUserID" required inputmode="text" autocomplete="off" placeholder="已初始化的 UUID" :disabled="busy" class="rounded-[var(--hk-radius-control)] border border-[var(--hk-line)] bg-white px-3 py-2 font-mono text-sm font-normal" @input="resetLookup">
        </label>
        <UiButton class="mt-4" type="submit" :disabled="busy || detailState === 'loading' || !targetUserID.trim()">查询会员权益</UiButton>
      </form>

      <section class="operation-panel !mt-0" :data-account-membership-detail-state="detailState" aria-labelledby="account-membership-detail-heading">
        <div v-if="detailState === 'idle'" class="text-[var(--hk-ink-muted)]">输入已初始化账户的用户 ID 后读取其当前会员权益。</div>
        <div v-else-if="detailState === 'loading'" aria-busy="true">正在读取持久化会员权益…</div>
        <div v-else-if="detailState === 'not_found'" class="text-[var(--hk-ink-muted)]">该用户尚未初始化 Account Portfolio；不能从 Console 创建虚构账户。</div>
        <div v-else-if="detailState === 'invalid'" class="text-[var(--hk-ink-muted)]">用户 ID 格式无效。</div>
        <div v-else-if="detailState === 'unavailable'" class="text-[var(--hk-ink-muted)]"><p>会员权益暂不可用。</p><UiButton class="mt-3" @click="lookupMembership">重新加载</UiButton></div>
        <template v-else-if="membership">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="eyebrow">CURRENT ENTITLEMENT</p>
              <h2 id="account-membership-detail-heading" class="mt-1 text-xl font-bold">{{ membershipLabel(membership) }}</h2>
            </div>
            <span class="rounded-full bg-[var(--hk-paper)] px-3 py-1 text-sm">版本 {{ membership.version }}</span>
          </div>
          <p class="mt-3 text-sm leading-6 text-[var(--hk-ink-muted)]">每次授权或撤销都写入不可变审计事件，并向用户创建一条持久化通知。</p>

          <form class="mt-5 border-t border-[var(--hk-line)] pt-5" @submit.prevent="submitMutation">
            <label class="grid gap-2 text-sm font-semibold">
              操作原因
              <textarea v-model="reason" required maxlength="1000" rows="4" class="rounded-[var(--hk-radius-control)] border border-[var(--hk-line)] bg-white px-3 py-2 font-normal" :placeholder="membership.plan === 'free' ? '说明为何发放终身权益。' : '说明为何撤销终身权益。'"></textarea>
            </label>
            <UiButton class="mt-3" type="submit" :disabled="busy || !reason.trim()">
              {{ membership.plan === 'free' ? '发放终身会员' : '撤销终身会员' }}
            </UiButton>
          </form>
        </template>
      </section>
    </div>
  </section>
</template>
