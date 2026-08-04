<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { Button, Card, Label, PageHeader, Textarea } from "@/components/ui";
import {
  fetchAccountTicket,
  fetchAccountTicketQueue,
  replyToAccountTicket,
  transitionAccountTicket,
  type AccountTicketWriteResult,
  type ConsoleAccountTicket,
  type ConsoleAccountTicketDetail,
  type ConsoleOperatorReplyRequest,
  type ConsoleTicketTransitionRequest,
} from "@/lib/console-gateway";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "denied" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "unavailable";
type PendingCommand =
  | { kind: "reply"; ticketID: string; input: ConsoleOperatorReplyRequest; key: string }
  | { kind: "transition"; ticketID: string; input: ConsoleTicketTransitionRequest; key: string };

const queue = ref<ConsoleAccountTicket[]>([]);
const workspaceState = ref<WorkspaceState>("loading");
const selectedID = ref("");
const detail = ref<ConsoleAccountTicketDetail>();
const detailState = ref<DetailState>("idle");
const replyBody = ref("");
const feedback = ref("");
const busy = ref(false);
const pendingStorageKey = "henukit.account-ticket.pending-command";
const pending = ref<PendingCommand>();
const canRead = computed(() => props.permissions.includes("account.tickets.read"));
const canReply = computed(() => props.permissions.includes("account.tickets.reply"));
const canTransition = computed(() => props.permissions.includes("account.tickets.transition"));

function operationKey(kind: "reply" | "transition") {
  return `idem_account_${kind}_${crypto.randomUUID()}`;
}

function persistPending(value?: PendingCommand) {
  pending.value = value;
  if (value) sessionStorage.setItem(pendingStorageKey, JSON.stringify(value));
  else sessionStorage.removeItem(pendingStorageKey);
}

function restorePending() {
  try {
    const stored = JSON.parse(sessionStorage.getItem(pendingStorageKey) ?? "null") as Partial<PendingCommand> | null;
    if (!stored || (stored.kind !== "reply" && stored.kind !== "transition") || typeof stored.ticketID !== "string" || typeof stored.key !== "string" || !stored.input || typeof stored.input !== "object") {
      sessionStorage.removeItem(pendingStorageKey);
      return;
    }
    if (stored.kind === "reply" && typeof (stored.input as ConsoleOperatorReplyRequest).body === "string" && typeof (stored.input as ConsoleOperatorReplyRequest).expected_version === "number") {
      pending.value = stored as PendingCommand;
    }
    if (stored.kind === "transition" && (stored.input as ConsoleTicketTransitionRequest).status && typeof (stored.input as ConsoleTicketTransitionRequest).expected_version === "number") {
      pending.value = stored as PendingCommand;
    }
  } catch {
    sessionStorage.removeItem(pendingStorageKey);
  }
}

restorePending();

function statusLabel(status: ConsoleAccountTicket["status"]) {
  return status === "open" ? "待处理" : status === "in_progress" ? "处理中" : "已解决";
}

// Success feedback must name the transition that actually happened. Reporting
// "已解决" for an in_progress transition tells the operator the ticket is done,
// so they stop working it and the queue silently accumulates open tickets.
function transitionFeedback(status: ConsoleTicketTransitionRequest["status"]) {
  return status === "in_progress" ? "工单已开始处理。" : "工单已标记为已解决。";
}

function timestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

async function refreshQueue() {
  if (props.authState !== "authenticated" || !canRead.value) {
    workspaceState.value = props.authState === "denied" || !canRead.value ? "denied" : "unavailable";
    return;
  }
  workspaceState.value = "loading";
  const result = await fetchAccountTicketQueue();
  if (result.state === "authenticated") {
    queue.value = result.queue.tickets;
    workspaceState.value = "ready";
    return;
  }
  workspaceState.value = result.state === "denied" ? "denied" : "unavailable";
}

async function openTicket(ticketID: string) {
  selectedID.value = ticketID;
  detail.value = undefined;
  detailState.value = "loading";
  const result = await fetchAccountTicket(ticketID);
  if (result.state === "authenticated") {
    detail.value = result.ticket;
    detailState.value = "ready";
    return;
  }
  detailState.value = result.state === "not_found" ? "not_found" : "unavailable";
}

function updateQueue(ticket: ConsoleAccountTicket) {
  queue.value = queue.value.map((current) => (current.id === ticket.id ? ticket : current));
}

function clearPendingForEditedReply() {
  if (pending.value?.kind === "reply" && !busy.value) persistPending();
}

async function finish(command: PendingCommand) {
  busy.value = true;
  feedback.value = command.kind === "reply" ? "正在发送客服回复…" : "正在更新工单状态…";
  const result: AccountTicketWriteResult = command.kind === "reply"
    ? await replyToAccountTicket(command.ticketID, command.input, command.key)
    : await transitionAccountTicket(command.ticketID, command.input, command.key);
  await finishResult(command, result);
}

async function finishResult(command: PendingCommand, result: AccountTicketWriteResult) {
  if (result.state === "succeeded") {
    persistPending();
    updateQueue(result.ticket);
    if (command.kind === "reply") replyBody.value = "";
    feedback.value = command.kind === "reply" ? "回复已写入工单。" : transitionFeedback(command.input.status);
    await refreshQueue();
    await openTicket(command.ticketID);
  } else if (result.state === "conflict") {
    persistPending();
    feedback.value = "工单版本已变化，已刷新最新记录；请基于最新版本重试。";
    await refreshQueue();
    await openTicket(command.ticketID);
  } else if (result.state === "unavailable") {
    feedback.value = "结果还没确认，可点下方按钮按原请求重试。";
  } else {
    persistPending();
    feedback.value = result.state === "denied" ? "当前账户缺少该项账户工单权限。" : result.state === "not_found" ? "该工单已不可访问。" : result.state === "invalid" ? "请求内容无效，请检查填写后重试。" : "操作没有完成，请稍后刷新页面重试。";
  }
  busy.value = false;
}

async function sendReply() {
  const current = detail.value?.ticket;
  const body = replyBody.value.trim();
  if (!current || !body || busy.value) return;
  const command: PendingCommand = {
    kind: "reply",
    ticketID: current.id,
    input: { body, expected_version: current.version },
    key: operationKey("reply"),
  };
  persistPending(command);
  await finish(command);
}

async function transition(status: "in_progress" | "resolved") {
  const current = detail.value?.ticket;
  if (!current || busy.value) return;
  const command: PendingCommand = {
    kind: "transition",
    ticketID: current.id,
    input: { status, expected_version: current.version },
    key: operationKey("transition"),
  };
  persistPending(command);
  await finish(command);
}

watch(
  () => props.authState,
  (value) => {
    if (value === "authenticated") {
      void refreshQueue();
      return;
    }
    queue.value = [];
    detail.value = undefined;
    selectedID.value = "";
    detailState.value = "idle";
    workspaceState.value = value === "denied" ? "denied" : value === "loading" ? "loading" : "unavailable";
  },
  { immediate: true },
);
</script>

<template>
  <section aria-labelledby="account-tickets-heading">
    <PageHeader
      eyebrow="工单处理操作"
      title="账户工单运营"
      description="工单处理均以当前登录身份记录。"
      title-id="account-tickets-heading"
    >
      <div class="access-context"><strong>{{ queue.length }} 条工单</strong></div>
    </PageHeader>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <Button v-if="pending && !busy" class="mt-3" @click="finish(pending)">按原请求重试</Button>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在读取工单队列…</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有工单查看权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>工单服务暂时不可用。</p><Button class="mt-3" @click="refreshQueue">重新加载</Button></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <Card class="!mt-0 p-4" aria-labelledby="account-ticket-queue-heading">
        <div class="flex flex-wrap items-center justify-between gap-3"><h2 id="account-ticket-queue-heading">工单队列</h2><Button variant="ghost" @click="refreshQueue">刷新</Button></div>
        <p v-if="queue.length === 0" class="mt-3 text-[var(--hk-ink-muted)]">暂无待处理工单。</p>
        <div v-else class="mt-3 grid gap-2">
          <button
            v-for="ticket in queue"
            :key="ticket.id"
            type="button"
            class="rounded-[var(--hk-radius-control)] border border-[var(--hk-line)] p-3 text-left transition hover:bg-[var(--hk-paper)]"
            :class="selectedID === ticket.id ? 'ring-2 ring-[var(--hk-accent)]' : ''"
            :aria-current="selectedID === ticket.id ? 'page' : undefined"
            @click="openTicket(ticket.id)"
          >
            <div class="flex items-start justify-between gap-3"><strong>{{ ticket.title }}</strong><span class="shrink-0 rounded-full bg-[var(--hk-paper)] px-2 py-1 text-xs">{{ statusLabel(ticket.status) }}</span></div>
            <p class="mt-1 break-all text-xs text-[var(--hk-ink-muted)]">{{ ticket.reference }}</p>
            <p class="mt-1 text-xs text-[var(--hk-ink-muted)]">更新于 {{ timestamp(ticket.updated_at) }}</p>
          </button>
        </div>
      </Card>

      <Card class="!mt-0 p-4" aria-labelledby="account-ticket-detail-heading" :data-account-ticket-detail-state="detailState">
        <div v-if="detailState === 'idle'" class="text-[var(--hk-ink-muted)]">选择队列中的工单查看详情、回复和状态流转。</div>
        <div v-else-if="detailState === 'loading'" aria-busy="true">正在读取工单详情…</div>
        <div v-else-if="detailState === 'not_found'" class="text-[var(--hk-ink-muted)]">该工单已不可访问；请刷新队列。</div>
        <div v-else-if="detailState === 'unavailable'" class="text-[var(--hk-ink-muted)]"><p>工单详情暂不可用。</p><Button class="mt-3" @click="selectedID && openTicket(selectedID)">重新加载</Button></div>
        <template v-else-if="detail">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div><p class="eyebrow">{{ detail.ticket.reference }}</p><h2 id="account-ticket-detail-heading" class="mt-1 text-xl font-bold">{{ detail.ticket.title }}</h2></div>
            <span class="rounded-full bg-[var(--hk-paper)] px-3 py-1 text-sm">{{ statusLabel(detail.ticket.status) }}</span>
          </div>
          <p class="mt-2 text-sm text-[var(--hk-ink-muted)]">版本 {{ detail.ticket.version }} · 更新于 {{ timestamp(detail.ticket.updated_at) }}</p>

          <div class="mt-5 grid gap-3 border-y border-[var(--hk-line)] py-5">
            <article v-for="message in detail.messages" :key="message.id" class="border-l-2 border-[var(--hk-accent)] pl-3">
              <p class="text-xs text-[var(--hk-ink-muted)]">{{ message.author_kind === 'operator' ? '客服' : '用户' }} · {{ timestamp(message.created_at) }}</p>
              <p class="mt-1 whitespace-pre-wrap leading-7">{{ message.body }}</p>
            </article>
          </div>

          <form v-if="canReply" class="mt-5" @submit.prevent="sendReply">
            <Label class="grid gap-2">客服回复<Textarea v-model="replyBody" required maxlength="5000" rows="4" placeholder="回复将以你的登录身份写入工单。" @input="clearPendingForEditedReply"></Textarea></Label>
            <Button class="mt-3" type="submit" :disabled="busy || !replyBody.trim()">发送回复</Button>
          </form>

          <div v-if="canTransition && detail.ticket.status !== 'resolved'" class="mt-5 flex flex-wrap gap-3 border-t border-[var(--hk-line)] pt-5">
            <Button v-if="detail.ticket.status === 'open'" :disabled="busy" @click="transition('in_progress')">开始处理</Button>
            <Button :disabled="busy" variant="ghost" class="secondary-action" @click="transition('resolved')">标记已解决</Button>
          </div>
          <p v-else-if="!canReply && !canTransition" class="mt-5 text-sm text-[var(--hk-ink-muted)]">当前账户仅拥有工单只读权限。</p>
        </template>
      </Card>
    </div>
  </section>
</template>
