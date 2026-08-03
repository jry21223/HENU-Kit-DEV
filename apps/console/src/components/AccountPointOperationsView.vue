<script setup lang="ts">
import { computed, ref, watch } from "vue";

import UiButton from "@/components/ui/UiButton.vue";
import { adjustAccountPoints, type ConsoleAccountPointAdjustmentResult, type ConsolePointAdjustmentRequest } from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type PendingCommand = {
  operatorID: string;
  input: ConsolePointAdjustmentRequest;
  key: string;
};

const targetUserID = ref("");
const amount = ref("");
const reason = ref("");
const workspaceState = ref<WorkspaceState>("loading");
const feedback = ref("");
const busy = ref(false);
const adjustment = ref<ConsoleAccountPointAdjustmentResult>();
const pending = ref<PendingCommand>();
const pendingStorageKey = "henukit.account-points.pending-command";
const canAdjust = computed(() => props.permissions.includes("account.points.adjust"));

function operationKey() {
  return `idem_account_points_${crypto.randomUUID()}`;
}

function isUUID(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

function parseAmount(value: string): number | undefined {
  if (!/^-?\d+$/.test(value.trim())) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed !== 0 ? parsed : undefined;
}

function validInput(value: ConsolePointAdjustmentRequest): boolean {
  return isUUID(value.user_id) && Number.isSafeInteger(value.amount) && value.amount !== 0 && value.reason.trim().length > 0 && value.reason.length <= 1000;
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
    if (!stored || stored.operatorID !== operatorID || typeof stored.key !== "string" || !stored.input || typeof stored.input !== "object" || !validInput(stored.input as ConsolePointAdjustmentRequest)) {
      sessionStorage.removeItem(pendingStorageKey);
      return;
    }
    pending.value = stored as PendingCommand;
    feedback.value = "发现一项结果未确认的积分调整；可按原请求重试。";
  } catch {
    sessionStorage.removeItem(pendingStorageKey);
  }
}

function formatPoints(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

async function finish(command: PendingCommand) {
  if (props.authState !== "authenticated" || !props.operatorID || props.operatorID !== command.operatorID || !canAdjust.value) {
    persistPending(command);
    feedback.value = "当前登录状态或操作员身份已变化，未执行原积分调整。";
    return;
  }
  busy.value = true;
  feedback.value = "正在写入不可变积分账本…";
  const result = await adjustAccountPoints(command.input, command.key);
  if (props.authState !== "authenticated" || props.operatorID !== command.operatorID || !canAdjust.value) {
    persistPending(command);
    feedback.value = "当前登录状态或操作员身份已变化，未确认原积分调整结果。";
    busy.value = false;
    return;
  }
  if (result.state === "succeeded") {
    adjustment.value = result.result;
    amount.value = "";
    reason.value = "";
    persistPending();
    feedback.value = "积分调整已写入账本，并已为目标用户创建持久化通知。";
  } else if (result.state === "conflict") {
    persistPending();
    feedback.value = "积分不足或数据有变化，未写入记录，请刷新后重试。";
  } else if (result.state === "unavailable") {
    feedback.value = "结果还没确认，可点下方按钮按原请求重试。";
  } else {
    if (result.state === "signed_out" || result.state === "denied") persistPending(command);
    if (result.state === "signed_out") workspaceState.value = "signed_out";
    if (result.state === "denied") workspaceState.value = "denied";
    feedback.value = result.state === "signed_out"
      ? "登录状态已过期，请重新登录后再操作。"
      : result.state === "denied"
        ? "当前账户缺少积分调整权限。"
        : result.state === "invalid"
          ? "请求内容无效；请检查用户 ID、增减积分和原因。"
          : "积分调整没有完成，请稍后刷新页面重试。";
  }
  busy.value = false;
}

async function submitAdjustment() {
  const input: ConsolePointAdjustmentRequest = {
    user_id: targetUserID.value.trim(),
    amount: parseAmount(amount.value) ?? 0,
    reason: reason.value.trim(),
  };
  if (busy.value || !props.operatorID || !validInput(input)) {
    feedback.value = "请填写正确的用户 ID、非零整数积分和不超过 1000 字的操作原因。";
    return;
  }
  const command: PendingCommand = { operatorID: props.operatorID, input, key: operationKey() };
  persistPending(command);
  await finish(command);
}

watch(
  () => [props.authState, props.operatorID, canAdjust.value] as const,
  ([authState, operatorID, hasPermission]) => {
    if (authState === "authenticated" && hasPermission && operatorID) {
      workspaceState.value = "ready";
      restorePending(operatorID);
      return;
    }
    if (authState === "loading") {
      workspaceState.value = "loading";
      adjustment.value = undefined;
      return;
    }
    persistPending(pending.value);
    workspaceState.value = authState === "signed_out" ? "signed_out" : authState === "denied" || !hasPermission ? "denied" : "unavailable";
    adjustment.value = undefined;
  },
  { immediate: true },
);
</script>

<template>
  <section aria-labelledby="account-points-heading">
    <div class="overview-hero">
      <div>
        <p class="eyebrow">积分调整操作</p>
        <h1 id="account-points-heading" class="mt-2 text-2xl font-bold sm:text-3xl">积分账本运营</h1>
        <p class="mt-2 max-w-3xl leading-7 text-[var(--hk-ink-muted)]">积分调整以当前登录身份记录，不可伪造。</p>
      </div>
      <div class="access-context"><strong>不可变账本</strong></div>
    </div>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">待确认操作：用户 <code>{{ pending.input.user_id }}</code> {{ pending.input.amount > 0 ? "增加" : "扣减" }} {{ formatPoints(Math.abs(pending.input.amount)) }} 积分。</span>
      <UiButton v-if="pending && !busy" class="mt-3" @click="finish(pending)">确认并按原请求重试</UiButton>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证积分调整权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有积分调整权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state">积分服务暂时不可用，请稍后重试。</div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <form class="operation-panel !mt-0" @submit.prevent="submitAdjustment">
        <h2>记账积分调整</h2>
        <p class="mt-2 text-sm leading-6 text-[var(--hk-ink-muted)]">输入用户 ID，系统将直接为该用户记入或扣减积分，并记录操作日志。</p>
        <label class="mt-5 grid gap-2 text-sm font-semibold">
          目标用户 ID
          <input v-model="targetUserID" required autocomplete="off" placeholder="目标用户 ID" :disabled="busy" class="font-mono text-sm font-normal">
        </label>
        <label class="mt-4 grid gap-2 text-sm font-semibold">
          积分变更
          <input v-model="amount" required inputmode="numeric" autocomplete="off" placeholder="正数增加，负数扣减" :disabled="busy" class="font-normal">
        </label>
        <label class="mt-4 grid gap-2 text-sm font-semibold">
          操作原因
          <textarea v-model="reason" required maxlength="1000" rows="4" :disabled="busy" placeholder="记录可复核的调整原因。" class="rounded-[var(--hk-radius-control)] border border-[var(--hk-line)] bg-white px-3 py-2 font-normal"></textarea>
        </label>
        <UiButton class="mt-4" type="submit" :disabled="busy">{{ busy ? "正在提交…" : "提交积分调整" }}</UiButton>
      </form>

      <section class="operation-panel !mt-0" data-account-points-result aria-labelledby="account-points-result-heading">
        <div v-if="!adjustment" class="text-[var(--hk-ink-muted)]">
          提交后这里显示该用户的最新余额与本次记账明细。
        </div>
        <template v-else>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="eyebrow">账本已确认</p>
              <h2 id="account-points-result-heading" class="mt-1 text-xl font-bold">当前积分余额 {{ formatPoints(adjustment.balance) }}</h2>
            </div>
            <span class="rounded-full bg-[var(--hk-paper)] px-3 py-1 text-sm">{{ adjustment.entry.amount > 0 ? "+" : "" }}{{ formatPoints(adjustment.entry.amount) }}</span>
          </div>
          <dl class="mt-5 grid gap-3 border-t border-[var(--hk-line)] pt-5 text-sm">
            <div class="grid gap-1"><dt class="text-[var(--hk-ink-muted)]">记账原因</dt><dd>{{ adjustment.entry.reason }}</dd></div>
            <div class="grid gap-1"><dt class="text-[var(--hk-ink-muted)]">记账时间（中国标准时间）</dt><dd>{{ formatTime(adjustment.entry.created_at) }}</dd></div>
          </dl>
        </template>
      </section>
    </div>
  </section>
</template>
