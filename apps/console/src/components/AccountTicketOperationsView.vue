<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";

import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import {
  fetchAccountTicket,
  fetchAccountTicketFacts,
  replyToAccountTicket,
  resolveAccountTicketOperation,
  transitionAccountTicket,
  type AccountTicketWriteResult,
  type ConsoleAccountTicket,
  type ConsoleAccountTicketFacts,
  type ConsoleTicketFactReference,
  type ConsoleTicketFactsFilter,
  type ConsoleAccountTicketDetail,
  type ConsoleOperatorReplyRequest,
  type ConsoleTicketTransitionRequest,
} from "@/lib/console-gateway";
import { clearPendingAccountTicketOperation, readPendingAccountTicketOperation, writePendingAccountTicketOperation, type PendingAccountTicketOperation } from "@/lib/pending-operations";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string;
  permissions: string[];
}>();

type WorkspaceState = "loading" | "ready" | "denied" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "unavailable";

const queue = ref<ConsoleTicketFactReference[]>([]);
const queueFacts = ref<ConsoleAccountTicketFacts>();
const workspaceState = ref<WorkspaceState>("loading");
const selectedID = ref("");
const detail = ref<ConsoleAccountTicketDetail>();
const detailState = ref<DetailState>("idle");
const replyBody = ref("");
const feedback = ref("");
const busy = ref(false);
let detailSequence = 0;
let queueSequence = 0;
let principalSequence = 0;
onBeforeUnmount(() => { principalSequence++; queueSequence++; detailSequence++; });
const replyDrafts = new Map<string, string>();
const pending = ref<PendingAccountTicketOperation>();
function statusFromURL(): ConsoleTicketFactsFilter | undefined {
  const value = new URL(window.location.href).searchParams.get("status");
  return ["unresolved", "all", "open", "in_progress", "resolved"].includes(value ?? "") ? value as ConsoleTicketFactsFilter : undefined;
}
const statusFilter = ref<ConsoleTicketFactsFilter>(statusFromURL() ?? "all");
const searchQuery = ref("");
const queueList = ref<HTMLElement>();
let queueScrollTop = 0;
let restoringContext = false;
const viewStorageKey = "henukit.console.account-ticket-view.v1";
const canRead = computed(() => props.permissions.includes("account.tickets.read"));
const canReply = computed(() => props.permissions.includes("account.tickets.reply"));
const canTransition = computed(() => props.permissions.includes("account.tickets.transition"));
const visibleTickets = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase();
  return queue.value.filter((ticket) => !query || ticket.title.toLocaleLowerCase().includes(query) || ticket.reference.toLocaleLowerCase().includes(query));
});

function persistView() {
  if (!props.operatorID || props.authState !== "authenticated") return;
  queueScrollTop = queueList.value?.scrollTop ?? queueScrollTop;
  try { sessionStorage.setItem(viewStorageKey, JSON.stringify({ version: 1, operator_id: props.operatorID, status: statusFilter.value, selected_id: selectedID.value, scroll_top: queueScrollTop })); } catch { /* reading remains available without storage */ }
}
function restoreView() {
  restoringContext = true;
  const explicitStatus = statusFromURL();
  statusFilter.value = explicitStatus ?? "all"; searchQuery.value = ""; queueScrollTop = 0;
  try {
    const value = JSON.parse(sessionStorage.getItem(viewStorageKey) ?? "null") as Record<string, unknown> | null;
    if (value?.version === 1 && value.operator_id === props.operatorID && ["unresolved", "all", "open", "in_progress", "resolved"].includes(String(value.status)) && typeof value.selected_id === "string" && (value.selected_id === "" || /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value.selected_id)) && typeof value.scroll_top === "number" && Number.isFinite(value.scroll_top) && value.scroll_top >= 0) {
      if (!explicitStatus || explicitStatus === value.status) {
        statusFilter.value = value.status as typeof statusFilter.value;
        selectedID.value = value.selected_id;
        queueScrollTop = value.scroll_top;
      }
    } else sessionStorage.removeItem(viewStorageKey);
  } catch { /* invalid view context is ignored */ }
  restoringContext = false;
}
async function backToQueue() {
  await nextTick();
  queueList.value?.querySelector<HTMLButtonElement>(`[data-ticket-id="${selectedID.value}"]`)?.focus({ preventScroll: true });
}
watch([statusFilter, searchQuery], () => {
  if (restoringContext) return;
  detailSequence++;
  if (selectedID.value) replyDrafts.set(selectedID.value, replyBody.value);
  selectedID.value = ""; detail.value = undefined; detailState.value = "idle"; replyBody.value = "";
  queueScrollTop = 0;
  persistView();
}, { flush: "sync" });
watch(statusFilter, () => {
  if (restoringContext) return;
  const url = new URL(window.location.href);
  if (/\/account\/?$/.test(url.pathname)) {
    url.searchParams.set("status", statusFilter.value);
    history.replaceState(history.state, "", url);
  }
  void refreshQueue();
}, { flush: "sync" });

function operationKey(kind: "reply" | "transition") {
  return `idem_account_${kind}_${crypto.randomUUID()}`;
}

function clearPending() { pending.value = undefined; clearPendingAccountTicketOperation(); }
function currentPrincipal(sequence: number, actor: string | undefined) {
  return sequence === principalSequence && actor === props.operatorID && props.authState === "authenticated";
}
const canWrite = computed(() => !busy.value && !pending.value && Boolean(props.operatorID) && props.authState === "authenticated");

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
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "short" }).format(date);
}

async function refreshQueue() {
  if (props.authState !== "authenticated" || !canRead.value) return;
  const sequence = ++queueSequence, principal = principalSequence, actor = props.operatorID;
  const filter = statusFilter.value;
  workspaceState.value = "loading";
  queue.value = [];
  queueFacts.value = undefined;
  const result = await fetchAccountTicketFacts(filter);
  if (sequence !== queueSequence || filter !== statusFilter.value || !currentPrincipal(principal, actor)) return;
  if (result.state === "authenticated") {
    queueFacts.value = result.facts;
    queue.value = result.facts.window.items; workspaceState.value = "ready";
    await nextTick();
    if (sequence === queueSequence && currentPrincipal(principal, actor) && queueList.value) queueList.value.scrollTop = queueScrollTop;
  }
  else { queue.value = []; workspaceState.value = result.state === "denied" ? "denied" : "unavailable"; }
}

async function openTicket(ticketID: string) {
  if (props.authState !== "authenticated" || !canRead.value) return;
  const sequence = ++detailSequence, principal = principalSequence, actor = props.operatorID;
  if (selectedID.value) replyDrafts.set(selectedID.value, replyBody.value);
  selectedID.value = ticketID;
  persistView();
  replyBody.value = replyDrafts.get(ticketID) ?? "";
  detail.value = undefined;
  detailState.value = "loading";
  const result = await fetchAccountTicket(ticketID);
  if (sequence !== detailSequence || selectedID.value !== ticketID || !currentPrincipal(principal, actor)) return;
  if (result.state === "authenticated" && result.ticket.ticket.id === ticketID) { detail.value = result.ticket; detailState.value = "ready"; }
  else detailState.value = result.state === "not_found" ? "not_found" : "unavailable";
}

async function finishResult(command: PendingAccountTicketOperation, result: AccountTicketWriteResult | { state: "unknown" }, viewSequence: number) {
  const principal = principalSequence, actor = props.operatorID;
  if (result.state === "succeeded") {
    if (result.ticket.id !== command.resource_id || result.ticket.version !== command.expected_version + 1) {
      feedback.value = "返回结果与原工单不一致，结果尚未确认；请查询原操作结果。";
      return;
    }
    clearPending();
    queue.value = queue.value.map((ticket) => ticket.id === result.ticket.id ? result.ticket : ticket);
    if (command.operation === "ticket_reply") {
      replyDrafts.delete(command.resource_id);
      if (selectedID.value === command.resource_id) replyBody.value = "";
    }
    feedback.value = command.operation === "ticket_reply" ? "回复已写入工单。" : transitionFeedback(result.ticket.status === "in_progress" ? "in_progress" : "resolved");
    await refreshQueue();
    if (!currentPrincipal(principal, actor)) return;
    if (detailSequence === viewSequence && (selectedID.value === command.resource_id || !selectedID.value)) await openTicket(command.resource_id);
  } else if (result.state === "conflict") {
    clearPending();
    feedback.value = "工单版本已变化；回复草稿已保留，请核对最新记录后再提交。";
    await refreshQueue();
    if (!currentPrincipal(principal, actor)) return;
    if (detailSequence === viewSequence && selectedID.value === command.resource_id) await openTicket(command.resource_id);
  } else if (result.state === "unknown" || result.state === "unavailable") {
    feedback.value = "结果仍未确认，原操作已保留；请查询原操作结果，勿重复提交。";
  } else {
    feedback.value = result.state === "denied" ? "当前账户缺少该项工单权限，暂时无法核对原操作结果。" : result.state === "signed_out" ? "登录已过期，请重新登录后查询原操作结果。" : "暂时无法核对原操作结果，请稍后再次查询。";
  }
}

async function resolvePending() {
  const command = pending.value;
  if (!command || busy.value || command.operator_id !== props.operatorID || props.authState !== "authenticated") return;
  const principal = principalSequence, actor = props.operatorID, viewSequence = detailSequence;
  busy.value = true;
  feedback.value = "正在查询原操作结果…";
  const result = await resolveAccountTicketOperation(command.operation, command.idempotency_key);
  if (!currentPrincipal(principal, actor)) return;
  await finishResult(command, result, viewSequence);
  if (currentPrincipal(principal, actor)) busy.value = false;
}

async function finish(command: PendingAccountTicketOperation, input: ConsoleOperatorReplyRequest | ConsoleTicketTransitionRequest) {
  if (!writePendingAccountTicketOperation(command)) { feedback.value = "浏览器无法保存操作恢复信息，操作未提交；请允许会话存储后重试。"; return; }
  pending.value = command;
  const principal = principalSequence, actor = props.operatorID, viewSequence = detailSequence;
  busy.value = true;
  feedback.value = command.operation === "ticket_reply" ? "正在发送客服回复…" : "正在更新工单状态…";
  let result: AccountTicketWriteResult | { state: "unknown" } = command.operation === "ticket_reply"
    ? await replyToAccountTicket(command.resource_id, input as ConsoleOperatorReplyRequest, command.idempotency_key)
    : await transitionAccountTicket(command.resource_id, input as ConsoleTicketTransitionRequest, command.idempotency_key);
  if (!currentPrincipal(principal, actor)) return;
  if (result.state !== "succeeded" && result.state !== "conflict") {
    // The owner may have committed before Gateway identity enrichment failed.
    // A rejection returned by that later step is not proof of non-execution.
    result = await resolveAccountTicketOperation(command.operation, command.idempotency_key);
    if (!currentPrincipal(principal, actor)) return;
  }
  await finishResult(command, result, viewSequence);
  if (currentPrincipal(principal, actor)) busy.value = false;
}

function commandFor(operation: PendingAccountTicketOperation["operation"]): PendingAccountTicketOperation | undefined {
  const current = detail.value?.ticket;
  if (!current || current.id !== selectedID.value || detailState.value !== "ready" || !canWrite.value || !props.operatorID) return;
  return { version: 1, operator_id: props.operatorID, operation, resource_id: current.id, expected_version: current.version, idempotency_key: operationKey(operation === "ticket_reply" ? "reply" : "transition") };
}
async function sendReply() {
  const body = replyBody.value.trim();
  if (!body || !canReply.value || detail.value?.ticket.status === "resolved") return;
  const command = commandFor("ticket_reply");
  if (command) await finish(command, { body, expected_version: command.expected_version });
}
async function transition(status: "in_progress" | "resolved") {
  if (!canTransition.value || detail.value?.ticket.status === "resolved" || (status === "in_progress" && detail.value?.ticket.status !== "open")) return;
  const command = commandFor("ticket_transition");
  if (command) await finish(command, { status, expected_version: command.expected_version });
}

watch(() => [props.authState, props.operatorID, props.permissions.join(",")], () => {
  principalSequence++; queueSequence++; detailSequence++;
  queue.value = []; queueFacts.value = undefined; detail.value = undefined; selectedID.value = "";
  detailState.value = "idle"; feedback.value = ""; replyBody.value = ""; replyDrafts.clear();
  pending.value = undefined; busy.value = false;
  try { sessionStorage.removeItem("henukit.account-ticket.pending-command"); } catch { /* storage may be unavailable */ }
  if (props.authState !== "authenticated" || !canRead.value) {
    workspaceState.value = props.authState === "loading" ? "loading" : props.authState === "denied" || !canRead.value ? "denied" : "unavailable";
    return;
  }
  if (props.operatorID) pending.value = readPendingAccountTicketOperation(props.operatorID);
  restoreView();
  const principal = principalSequence, actor = props.operatorID;
  void refreshQueue().then(() => {
    if (currentPrincipal(principal, actor) && selectedID.value && detailState.value === "idle") void openTicket(selectedID.value);
  });
  if (pending.value) void resolvePending();
}, { immediate: true });
</script>

<template>
  <section aria-labelledby="account-tickets-heading">
    <PageHeader
      eyebrow="工单处理操作"
      title="账户工单运营"
      description="工单处理均以当前登录身份记录。"
      title-id="account-tickets-heading"
    >
      <div class="access-context"><strong v-if="workspaceState === 'ready'">本次载入 {{ queue.length }} 条</strong><span v-else>载入数量尚未确认</span></div>
    </PageHeader>

    <p v-if="feedback" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <Button v-if="pending && !busy" class="mt-3" @click="resolvePending">查询原操作结果</Button>
    </p>

    <div v-if="workspaceState === 'denied'" class="operation-state">当前账户没有工单查看权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>工单服务暂时不可用。</p><Button class="mt-3" @click="refreshQueue">重新加载</Button></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(18rem,.8fr)_minmax(0,1.2fr)]">
      <Card class="!mt-0 p-4" aria-labelledby="account-ticket-queue-heading">
        <div class="flex flex-wrap items-center justify-between gap-3"><h2 id="account-ticket-queue-heading">工单队列</h2><Button variant="ghost" @click="refreshQueue">刷新</Button></div>
        <p class="mt-2 text-sm text-muted-foreground">范围：全局工单按服务端所选状态筛选，按更新时间载入最多 100 条窗口内的工单；标题与编号检索仅作用于这个窗口。</p>
        <div class="mt-4 grid gap-3">
          <Label class="grid gap-2">工单状态<select v-model="statusFilter" aria-label="工单状态" class="w-full rounded-md border border-input bg-background px-3 py-2"><option value="all">全部状态</option><option value="unresolved">未解决（待处理与处理中）</option><option value="open">待处理</option><option value="in_progress">处理中</option><option value="resolved">已解决</option></select></Label>
          <Label class="grid gap-2">在本次队列中检索<Input v-model="searchQuery" maxlength="200" placeholder="输入工单标题或编号" /></Label>
        </div>
        <p v-if="workspaceState === 'loading'" class="mt-3 text-muted-foreground" aria-busy="true">正在读取工单队列…</p>
        <p v-if="queueFacts" class="mt-3 text-sm text-muted-foreground">全局匹配 {{ queueFacts.window.matched_total }} 条 · 本次载入 {{ queueFacts.window.returned }} 条<span v-if="queueFacts.window.has_more"> · 还有更多匹配工单未载入</span></p>
        <p v-if="queueFacts" class="mt-1 text-sm text-muted-foreground">源数据 <time :datetime="queueFacts.as_of">{{ timestamp(queueFacts.as_of) }}（北京时间）</time>；后续详情按当前版本重新读取。</p>
        <p v-if="queueFacts?.window.matched_total === 0" class="mt-3 text-muted-foreground">当前筛选下没有工单。</p>
        <p v-else-if="workspaceState === 'ready' && visibleTickets.length === 0" class="mt-3 text-muted-foreground">本次队列中没有符合条件的工单，请调整筛选或检索。</p>
        <div ref="queueList" class="mt-3 grid max-h-[36rem] gap-2 overflow-y-auto" @scroll.passive="persistView">
          <button
            v-for="ticket in visibleTickets"
            :key="ticket.id"
            :data-ticket-id="ticket.id"
            type="button"
            class="rounded-md border border-border p-3 text-left transition hover:bg-muted"
            :class="selectedID === ticket.id ? 'border-primary bg-accent' : ''"
            :aria-current="selectedID === ticket.id ? 'page' : undefined"
            @click="openTicket(ticket.id)"
          >
            <div class="flex items-start justify-between gap-3"><strong>{{ ticket.title }}</strong><span class="shrink-0 rounded-full bg-muted px-2 py-1 text-xs">{{ statusLabel(ticket.status) }}</span></div>
            <p class="mt-1 break-all text-xs text-muted-foreground">{{ ticket.reference }}</p>
            <p class="mt-1 text-xs text-muted-foreground">更新于 {{ timestamp(ticket.updated_at) }}</p>
          </button>
        </div>
      </Card>

      <Card class="!mt-0 p-4" aria-labelledby="account-ticket-detail-heading" :data-account-ticket-detail-state="detailState">
        <div v-if="detailState === 'idle'" class="text-muted-foreground">选择队列中的工单查看详情、回复和状态流转。</div>
        <div v-else-if="detailState === 'loading'" aria-busy="true">正在读取工单详情…</div>
        <div v-else-if="detailState === 'not_found'" class="text-muted-foreground">该工单已不可访问；请刷新队列。</div>
        <div v-else-if="detailState === 'unavailable'" class="text-muted-foreground"><p>工单详情暂不可用。</p><Button class="mt-3" @click="selectedID && openTicket(selectedID)">重新加载</Button></div>
        <template v-else-if="detail">
          <Button class="mb-4" variant="ghost" @click="backToQueue">返回队列</Button>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div><p class="eyebrow">{{ detail.ticket.reference }}</p><h2 id="account-ticket-detail-heading" class="mt-1 text-xl font-bold">{{ detail.ticket.title }}</h2><p class="mt-1 font-medium">{{ detail.ticket.display_name || "未设置姓名" }}</p><p class="break-all text-sm text-muted-foreground">{{ detail.ticket.email || "邮箱不可用" }}</p></div>
            <span class="rounded-full bg-muted px-3 py-1 text-sm">{{ statusLabel(detail.ticket.status) }}</span>
          </div>
          <p class="mt-2 text-sm text-muted-foreground">版本 {{ detail.ticket.version }} · 更新于 {{ timestamp(detail.ticket.updated_at) }}</p>

          <div class="mt-5 grid gap-3 border-y border-border py-5">
            <article v-for="message in detail.messages" :key="message.id" class="border-l-2 border-border pl-3">
              <p class="text-xs text-muted-foreground">{{ message.author_kind === 'operator' ? '客服' : '用户' }} · {{ timestamp(message.created_at) }}</p>
              <p class="mt-1 whitespace-pre-wrap leading-7">{{ message.body }}</p>
            </article>
          </div>

          <form v-if="canReply && detail.ticket.status !== 'resolved'" class="mt-5" @submit.prevent="sendReply">
            <Label class="grid gap-2">客服回复<Textarea v-model="replyBody" required maxlength="5000" rows="4" placeholder="回复将以你的登录身份写入工单。"></Textarea></Label>
            <Button class="mt-3" type="submit" :disabled="!canWrite || !replyBody.trim()">发送回复</Button>
          </form>
          <p v-else-if="canReply && detail.ticket.status === 'resolved'" class="mt-5 text-sm text-muted-foreground">工单已解决；用户再次追问后可继续回复。</p>

          <div v-if="canTransition && detail.ticket.status !== 'resolved'" class="mt-5 flex flex-wrap gap-3 border-t border-border pt-5">
            <Button v-if="detail.ticket.status === 'open'" :disabled="!canWrite" @click="transition('in_progress')">开始处理</Button>
            <Button :disabled="!canWrite" variant="ghost" class="secondary-action" @click="transition('resolved')">标记已解决</Button>
          </div>
          <p v-else-if="!canReply && !canTransition" class="mt-5 text-sm text-muted-foreground">当前账户仅拥有工单只读权限。</p>
        </template>
      </Card>
    </div>
  </section>
</template>
