<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { accountName, accountStatusLabel } from "@/lib/account-labels";
import { adjustAccountPoints, lookupConsoleAccount, resolveAccountOperation, type AccountPointAdjustmentWriteResult, type ConsoleAccountPointAdjustmentResult, type ConsoleLookedUpAccount, type ConsolePointAdjustmentRequest } from "@/lib/console-gateway";
import { clearPendingPointOperation, readPendingPointOperation, writePendingPointOperation, type PendingPointOperation } from "@/lib/pending-operations";
const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string; permissions: string[];
}>();
type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type LookupState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type Confirmation = { command: PendingPointOperation; input: ConsolePointAdjustmentRequest };
type PointResult = AccountPointAdjustmentWriteResult | { state: "unknown" };
const targetEmail = ref("");
const account = ref<ConsoleLookedUpAccount>();
const amount = ref("");
const reason = ref("");
const workspaceState = ref<WorkspaceState>("loading");
const lookupState = ref<LookupState>("idle");
const feedback = ref("");
const busy = ref(false);
const confirmedResult = ref<{ resourceID: string; result: ConsoleAccountPointAdjustmentResult }>();
const adjustment = computed(() => confirmedResult.value?.resourceID === account.value?.id ? confirmedResult.value?.result : undefined);
const confirm = ref<Confirmation>();
const pending = ref<PendingPointOperation>();
const canAdjust = computed(() => props.permissions.includes("account.points.adjust"));
const writable = computed(() => workspaceState.value === "ready" && !busy.value && !pending.value && canAdjust.value);
let lookupToken = 0, principal = 0, writeToken = 0, viewToken = 0;
onBeforeUnmount(() => { principal++; lookupToken++; writeToken++; });
function current(generation: number, actor: string | undefined) { return generation === principal && actor === props.operatorID && props.authState === "authenticated" && canAdjust.value; }
function parseAmount(value: string): number | undefined {
  if (!/^-?\d+$/.test(value.trim())) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed !== 0 ? parsed : undefined;
}
function resetLookup() {
  lookupToken++; viewToken++;
  account.value = undefined; lookupState.value = "idle"; confirm.value = undefined; feedback.value = "";
  amount.value = ""; reason.value = "";
}
function loseAccess(state: "signed_out" | "denied") {
  resetLookup(); confirmedResult.value = undefined; workspaceState.value = state;
}
function formatPoints(value: number) { return new Intl.NumberFormat("zh-CN").format(value); }
function formatTime(value: string) { return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }
async function lookupAccount() {
  const email = targetEmail.value.trim();
  if (!email || workspaceState.value !== "ready") return;
  const token = ++lookupToken, generation = principal, actor = props.operatorID;
  viewToken++; lookupState.value = "loading"; account.value = undefined; confirm.value = undefined; feedback.value = "";
  const result = await lookupConsoleAccount(email);
  if (token !== lookupToken || targetEmail.value.trim() !== email || !current(generation, actor)) return;
  if (result.state === "authenticated") {
    account.value = result.account ?? undefined; lookupState.value = result.account ? "ready" : "not_found";
  } else if (result.state === "signed_out" || result.state === "denied") loseAccess(result.state);
  else lookupState.value = result.state === "invalid" ? "invalid" : "unavailable";
}
async function query(command: PendingPointOperation): Promise<PointResult> {
  const result = await resolveAccountOperation("point_adjustment", command.idempotency_key);
  if (result.state !== "succeeded") return { state: result.state };
  const value = result.result.result;
  return value && "balance" in value && "entry" in value ? { state: "succeeded", result: value } : { state: "unavailable" };
}
async function settle(command: PendingPointOperation, result: PointResult, view: number) {
  if (result.state === "succeeded" && result.result.entry.amount === command.expected_amount) {
    confirmedResult.value = { resourceID: command.resource_id, result: result.result };
    pending.value = undefined; clearPendingPointOperation();
    if (view === viewToken && account.value?.id === command.resource_id) {
      amount.value = ""; reason.value = "";
      feedback.value = "积分调整已确认；下方仅显示本次记账结果。";
    } else if (!account.value && view === viewToken) feedback.value = "原积分调整已确认；请重新查找原目标账户后核对本次记账结果。";
  } else if (result.state === "conflict") {
    pending.value = undefined; clearPendingPointOperation();
    if (view === viewToken && account.value?.id === command.resource_id) feedback.value = "积分不足或数据有变化；本次未完成，输入已保留，请核对后重新确认。";
  } else {
    if (result.state === "signed_out" || result.state === "denied") loseAccess(result.state);
    if (view === viewToken || !account.value) feedback.value = result.state === "signed_out" ? "登录已过期，原操作已保留；重新登录后仅查询原操作结果。" : "结果尚未确认，原操作已保留；请查询原操作结果，勿重复提交。";
  }
}
async function resolvePending() {
  const command = pending.value;
  if (!command || busy.value || command.operator_id !== props.operatorID || workspaceState.value !== "ready") return;
  const generation = principal, actor = props.operatorID, token = ++writeToken, view = viewToken;
  busy.value = true;
  const result = await query(command);
  if (!current(generation, actor) || token !== writeToken) return;
  await settle(command, result, view);
  if (current(generation, actor) && token === writeToken) busy.value = false;
}
async function finish(command: PendingPointOperation, input: ConsolePointAdjustmentRequest) {
  if (!writable.value || command.operator_id !== props.operatorID || command.resource_id !== account.value?.id) return;
  if (!writePendingPointOperation(command)) { feedback.value = "浏览器无法保存操作恢复信息，操作未提交；请允许会话存储后重试。"; return; }
  pending.value = command; busy.value = true; confirmedResult.value = undefined;
  const generation = principal, actor = props.operatorID, token = ++writeToken, view = viewToken;
  feedback.value = "正在提交积分调整…";
  let result: PointResult = await adjustAccountPoints(input, command.idempotency_key);
  if (!current(generation, actor) || token !== writeToken) return;
  if (result.state !== "conflict" && result.state !== "signed_out" && (result.state !== "succeeded" || result.result.entry.amount !== command.expected_amount)) {
    result = await query(command);
    if (!current(generation, actor) || token !== writeToken) return;
  }
  await settle(command, result, view);
  if (current(generation, actor) && token === writeToken) busy.value = false;
}
function submitForm() {
  if (confirm.value || !writable.value) return;
  const value = parseAmount(amount.value), inputReason = reason.value.trim();
  if (!account.value || value === undefined || !inputReason || inputReason.length > 1000 || !props.operatorID) {
    feedback.value = "请先查找并核对账户，再填写非零安全整数积分和不超过 1000 字的操作原因。"; return;
  }
  confirm.value = { input: { user_id: account.value.id, amount: value, reason: inputReason }, command: { version: 1, operator_id: props.operatorID, operation: "point_adjustment", resource_id: account.value.id, expected_amount: value, idempotency_key: `idem_account_points_${crypto.randomUUID()}` } };
}
function confirmAdjustment() {
  const value = confirm.value;
  if (!value || !writable.value) return;
  confirm.value = undefined; void finish(value.command, value.input);
}
function cancelAdjustment() { confirm.value = undefined; feedback.value = "已取消，未写入任何积分流水。"; }
watch([amount, reason], () => { confirm.value = undefined; });
watch(() => [props.authState, props.operatorID, canAdjust.value] as const, ([auth, actor, allowed]) => {
  principal++; writeToken++; resetLookup();
  targetEmail.value = ""; confirmedResult.value = undefined; pending.value = undefined; busy.value = false;
  workspaceState.value = auth === "authenticated" && actor && allowed ? "ready" : auth === "loading" ? "loading" : auth === "signed_out" ? "signed_out" : auth === "denied" || !allowed ? "denied" : "unavailable";
  if (workspaceState.value !== "ready" || !actor) return;
  pending.value = readPendingPointOperation(actor);
  if (pending.value) void resolvePending();
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="account-points-heading">
    <PageHeader
      eyebrow="积分调整操作"
      title="积分调整运营"
      description="每次积分调整均以当前登录身份记录。"
      title-id="account-points-heading"
    >
      <div class="access-context"><strong>本次积分调整</strong></div>
    </PageHeader>

    <p v-if="feedback || pending" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">原积分调整结果尚未确认，查询不会再次增减积分。</span>
      <Button v-if="pending && !busy && workspaceState === 'ready'" class="mt-3" @click="resolvePending">查询原操作结果</Button>
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
            <Input v-model="targetEmail" required type="email" inputmode="email" autocomplete="off" placeholder="student@stu.henu.edu.cn" @input="resetLookup" />
          </Label>
          <Button type="button" :disabled="!targetEmail.trim()" @click="lookupAccount">查找账户</Button>
        </div>
        <div v-if="lookupState === 'ready' && account" class="mt-3 rounded-md border border-border bg-muted/40 p-3 text-sm">
          已核对账户：<strong>{{ accountName(account.display_name) }}</strong><span class="break-all"> · {{ account.email }}</span>（{{ accountStatusLabel(account.status) }}）
        </div>

        <Label class="mt-4 grid gap-2">
          积分变更
          <Input v-model="amount" required inputmode="numeric" autocomplete="off" placeholder="正数增加，负数扣减" :disabled="!writable || !account" />
        </Label>
        <Label class="mt-4 grid gap-2">
          操作原因
          <Textarea v-model="reason" required maxlength="1000" rows="4" :disabled="!writable || !account" placeholder="记录可复核的调整原因。"></Textarea>
        </Label>

        <div v-if="confirm && account" class="mt-4 rounded-md border border-border p-3" data-points-confirm-step>
          <p class="font-medium">确认写入这笔积分流水？</p>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">
            将向「{{ accountName(account.display_name) }} · {{ account.email }}」{{ confirm.input.amount > 0 ? "增加" : "扣减" }} {{ formatPoints(Math.abs(confirm.input.amount)) }} 积分，写入不可撤销的积分流水，并为该用户创建持久化通知。
          </p>
          <div class="mt-3 flex gap-3">
            <Button type="button" :disabled="!writable" @click="confirmAdjustment">确认写入</Button>
            <Button type="button" variant="outline" :disabled="busy" @click="cancelAdjustment">取消</Button>
          </div>
        </div>
        <Button v-else class="mt-4" type="submit" :disabled="!writable || !account || !amount.trim() || !reason.trim()">{{ busy ? "正在提交…" : "提交积分调整" }}</Button>
      </form>

      <Card class="!mt-0 p-4" :data-account-lookup-state="lookupState" data-account-points-result aria-labelledby="account-points-result-heading">
        <div v-if="lookupState === 'idle'" class="text-muted-foreground">输入完整邮箱查找账户并核对姓名后，这里显示本次调整确认的余额与一笔记账明细；这不是完整账本。</div>
        <div v-else-if="lookupState === 'loading'" class="text-muted-foreground" aria-busy="true">正在查找账户…</div>
        <div v-else-if="lookupState === 'not_found'" class="text-muted-foreground">没有找到该邮箱对应的账户。请核对邮箱后重试；这不是服务不可用。</div>
        <div v-else-if="lookupState === 'invalid'" class="text-muted-foreground">邮箱格式不对，请检查后重试。</div>
        <div v-else-if="lookupState === 'unavailable'" class="text-muted-foreground"><p>账户查找服务暂时不可用，请稍后再试。</p><Button class="mt-3" @click="lookupAccount">重新查找</Button></div>
        <template v-else-if="account">
          <p class="eyebrow">账户核对</p>
          <h2 id="account-points-result-heading" class="mt-1 text-xl font-bold">{{ accountName(account.display_name) }}</h2>
          <p class="mt-1 break-all text-sm text-muted-foreground">{{ account.email }} · 账户状态：{{ accountStatusLabel(account.status) }}。请核对姓名和邮箱后再提交。</p>
          <template v-if="adjustment">
            <div class="mt-5 flex flex-wrap items-start justify-between gap-3 border-t border-border pt-5">
              <div>
                <p class="eyebrow">本次记账已确认</p>
                <h3 class="mt-1 text-lg font-bold">本次调整后的余额 {{ formatPoints(adjustment.balance) }}</h3>
              </div>
              <span class="rounded-full bg-muted px-3 py-1 text-sm">{{ adjustment.entry.amount > 0 ? "+" : "" }}{{ formatPoints(adjustment.entry.amount) }}</span>
            </div>
            <dl class="mt-5 grid gap-3 border-t border-border pt-5 text-sm">
              <div class="grid gap-1"><dt class="text-muted-foreground">记账原因</dt><dd>{{ adjustment.entry.reason }}</dd></div>
              <div class="grid gap-1"><dt class="text-muted-foreground">记账时间（中国标准时间）</dt><dd>{{ formatTime(adjustment.entry.created_at) }}</dd></div>
            </dl>
          </template>
          <p v-else class="mt-4 text-muted-foreground">提交后这里显示本次调整确认的余额与一笔记账明细；这不是完整账本。</p>
        </template>
      </Card>
    </div>
  </section>
</template>
