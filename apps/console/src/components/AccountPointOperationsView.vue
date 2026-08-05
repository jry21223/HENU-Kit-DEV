<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { adjustAccountPoints, lookupConsoleAccount, type ConsoleAccountPointAdjustmentResult, type ConsoleLookedUpAccount, type ConsolePointAdjustmentRequest } from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type LookupState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type PendingCommand = {
  operatorID: string;
  input: ConsolePointAdjustmentRequest;
  key: string;
};

const targetEmail = ref("");
const account = ref<ConsoleLookedUpAccount>();
const amount = ref("");
const reason = ref("");
const workspaceState = ref<WorkspaceState>("loading");
const lookupState = ref<LookupState>("idle");
const feedback = ref("");
const busy = ref(false);
const adjustment = ref<ConsoleAccountPointAdjustmentResult>();
const confirm = ref<PendingCommand>();
const pending = ref<PendingCommand>();
const pendingStorageKey = "henukit.account-points.pending-command";
const canAdjust = computed(() => props.permissions.includes("account.points.adjust"));
let lookupToken = 0;

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

function resetLookup() {
  lookupToken += 1;
  account.value = undefined;
  lookupState.value = "idle";
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

async function lookupAccount() {
  const email = targetEmail.value.trim();
  if (!email || busy.value) return;
  const token = ++lookupToken;
  lookupState.value = "loading";
  account.value = undefined;
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
    return;
  }
  if (result.state === "signed_out") {
    persistPending(pending.value);
    workspaceState.value = "signed_out";
    lookupState.value = "idle";
    return;
  }
  if (result.state === "denied") {
    persistPending(pending.value);
    workspaceState.value = "denied";
    lookupState.value = "idle";
    return;
  }
  lookupState.value = result.state === "invalid" ? "invalid" : "unavailable";
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
          ? "请求内容无效；请检查账户、增减积分和原因。"
          : "积分调整没有完成，请稍后刷新页面重试。";
  }
  busy.value = false;
}

// requestAdjustment opens the single confirmation step, which merges the
// account-name echo with the #238 write confirmation: it restates the target
// name and the signed amount and states that the ledger entry is irreversible.
// Cancel performs no write, and nothing is persisted before confirmation. The
// pending retry record stores only the resolved account id inside the request,
// never the lookup email.
function requestAdjustment() {
  const input: ConsolePointAdjustmentRequest = {
    user_id: account.value?.id ?? "",
    amount: parseAmount(amount.value) ?? 0,
    reason: reason.value.trim(),
  };
  if (busy.value || !props.operatorID || !account.value || !validInput(input)) {
    feedback.value = "请先查找并核对账户，再填写非零整数积分和不超过 1000 字的操作原因。";
    return;
  }
  confirm.value = { operatorID: props.operatorID, input, key: operationKey() };
}

// While the confirmation step is open, form submission (including Enter)
// must not rebuild or dismiss it.
function submitForm() {
  if (confirm.value || busy.value) return;
  requestAdjustment();
}

function confirmAdjustment() {
  const command = confirm.value;
  if (!command) return;
  confirm.value = undefined;
  persistPending(command);
  void finish(command);
}

function cancelAdjustment() {
  confirm.value = undefined;
  feedback.value = "已取消，未写入任何积分流水。";
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
      resetLookup();
      return;
    }
    persistPending(pending.value);
    workspaceState.value = authState === "signed_out" ? "signed_out" : authState === "denied" || !hasPermission ? "denied" : "unavailable";
    resetLookup();
  },
  { immediate: true },
);
</script>

<template>
  <section aria-labelledby="account-points-heading">
    <PageHeader
      eyebrow="积分调整操作"
      title="积分账本运营"
      description="积分调整以当前登录身份记录，不可伪造。"
      title-id="account-points-heading"
    >
      <div class="access-context"><strong>不可变账本</strong></div>
    </PageHeader>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">待确认操作：用户 <code>{{ pending.input.user_id }}</code> {{ pending.input.amount > 0 ? "增加" : "扣减" }} {{ formatPoints(Math.abs(pending.input.amount)) }} 积分。</span>
      <Button v-if="pending && !busy" class="mt-3" @click="finish(pending)">确认并按原请求重试</Button>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证积分调整权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有积分调整权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state">积分服务暂时不可用，请稍后重试。</div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <form class="operation-panel !mt-0" @submit.prevent="submitForm">
        <h2>记账积分调整</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">先按完整邮箱查找账户并核对姓名，再填写积分变更。</p>
        <div class="mt-5 flex items-end gap-2">
          <Label class="grid flex-1 gap-2">
            完整邮箱
            <Input v-model="targetEmail" required type="email" inputmode="email" autocomplete="off" placeholder="student@stu.henu.edu.cn" :disabled="busy" @input="resetLookup" />
          </Label>
          <Button type="button" :disabled="busy || !targetEmail.trim()" @click="lookupAccount">查找账户</Button>
        </div>
        <div v-if="lookupState === 'ready' && account" class="mt-3 rounded-md border border-border bg-muted/40 p-3 text-sm">
          已核对账户：<strong>{{ accountName(account.display_name) }}</strong>（{{ accountStatusLabel(account.status) }}）
        </div>

        <Label class="mt-4 grid gap-2">
          积分变更
          <Input v-model="amount" required inputmode="numeric" autocomplete="off" placeholder="正数增加，负数扣减" :disabled="busy || !account" />
        </Label>
        <Label class="mt-4 grid gap-2">
          操作原因
          <Textarea v-model="reason" required maxlength="1000" rows="4" :disabled="busy || !account" placeholder="记录可复核的调整原因。"></Textarea>
        </Label>

        <div v-if="confirm && account" class="mt-4 rounded-md border border-border p-3" data-points-confirm-step>
          <p class="font-medium">确认写入这笔积分流水？</p>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">
            将向「{{ accountName(account.display_name) }}」{{ confirm.input.amount > 0 ? "增加" : "扣减" }} {{ formatPoints(Math.abs(confirm.input.amount)) }} 积分，写入不可撤销的积分流水，并向该用户发送通知。
          </p>
          <div class="mt-3 flex gap-3">
            <Button :disabled="busy" @click="confirmAdjustment">确认写入</Button>
            <Button variant="outline" :disabled="busy" @click="cancelAdjustment">取消</Button>
          </div>
        </div>
        <Button v-else class="mt-4" type="submit" :disabled="busy || !account || !amount.trim() || !reason.trim()">{{ busy ? "正在提交…" : "提交积分调整" }}</Button>
      </form>

      <Card class="!mt-0 p-4" :data-account-lookup-state="lookupState" data-account-points-result aria-labelledby="account-points-result-heading">
        <div v-if="lookupState === 'idle'" class="text-muted-foreground">输入完整邮箱查找账户并核对姓名后，这里显示该用户的最新余额与本次记账明细。</div>
        <div v-else-if="lookupState === 'loading'" class="text-muted-foreground" aria-busy="true">正在查找账户…</div>
        <div v-else-if="lookupState === 'not_found'" class="text-muted-foreground">没有找到该邮箱对应的账户。请核对邮箱后重试；这不是服务不可用。</div>
        <div v-else-if="lookupState === 'invalid'" class="text-muted-foreground">邮箱格式不对，请检查后重试。</div>
        <div v-else-if="lookupState === 'unavailable'" class="text-muted-foreground"><p>账户查找服务暂时不可用，请稍后再试。</p><Button class="mt-3" @click="lookupAccount">重新查找</Button></div>
        <template v-else-if="account">
          <p class="eyebrow">账户核对</p>
          <h2 id="account-points-result-heading" class="mt-1 text-xl font-bold">{{ accountName(account.display_name) }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">账户状态：{{ accountStatusLabel(account.status) }}。请核对姓名后再提交。</p>
          <template v-if="adjustment">
            <div class="mt-5 flex flex-wrap items-start justify-between gap-3 border-t border-border pt-5">
              <div>
                <p class="eyebrow">账本已确认</p>
                <h3 class="mt-1 text-lg font-bold">当前积分余额 {{ formatPoints(adjustment.balance) }}</h3>
              </div>
              <span class="rounded-full bg-muted px-3 py-1 text-sm">{{ adjustment.entry.amount > 0 ? "+" : "" }}{{ formatPoints(adjustment.entry.amount) }}</span>
            </div>
            <dl class="mt-5 grid gap-3 border-t border-border pt-5 text-sm">
              <div class="grid gap-1"><dt class="text-muted-foreground">记账原因</dt><dd>{{ adjustment.entry.reason }}</dd></div>
              <div class="grid gap-1"><dt class="text-muted-foreground">记账时间（中国标准时间）</dt><dd>{{ formatTime(adjustment.entry.created_at) }}</dd></div>
            </dl>
          </template>
          <p v-else class="mt-4 text-muted-foreground">提交后这里显示该用户的最新余额与本次记账明细。</p>
        </template>
      </Card>
    </div>
  </section>
</template>
