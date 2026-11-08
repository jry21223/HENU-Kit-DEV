<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { Button, Card, Input, Label, PageHeader, Textarea } from "@/components/ui";
import { accountName, accountStatusLabel } from "@/lib/account-labels";
import {
  fetchAccountMembership, grantAccountMembership, revokeAccountMembership, searchAccountMemberships, resolveAccountOperation,
  type AccountMembershipWriteResult, type ConsoleAccountMembership, type ConsoleMembershipAccountSummary, type ConsoleMembershipMutationRequest,
} from "@/lib/console-gateway";
import { clearPendingMembershipOperation, readPendingMembershipOperation, writePendingMembershipOperation, type PendingMembershipOperation } from "@/lib/pending-operations";

const props = defineProps<{
  authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable";
  operatorID?: string; permissions: string[];
}>();
type WorkspaceState = "loading" | "ready" | "signed_out" | "denied" | "unavailable";
type ListState = "idle" | "loading" | "ready" | "empty" | "invalid" | "unavailable";
type DetailState = "idle" | "loading" | "ready" | "not_found" | "invalid" | "unavailable";
type Confirmation = { kind: "grant" | "revoke"; command: PendingMembershipOperation; input: ConsoleMembershipMutationRequest };
type MembershipResult = AccountMembershipWriteResult | { state: "unknown" };
const searchQuery = ref("");
const activeQuery = ref("");
const accounts = ref<ConsoleMembershipAccountSummary[]>([]);
const account = ref<ConsoleMembershipAccountSummary>();
const membership = ref<ConsoleAccountMembership & { userID: string }>();
const currentPage = ref(1);
const nextPage = ref<number | null>(null);
const workspaceState = ref<WorkspaceState>("loading");
const listState = ref<ListState>("idle");
const detailState = ref<DetailState>("idle");
const reason = ref("");
const feedback = ref("");
const busy = ref(false);
const confirm = ref<Confirmation>();
const pending = ref<PendingMembershipOperation>();
const canWrite = computed(() => props.permissions.includes("account.membership.write"));
const writable = computed(() => workspaceState.value === "ready" && !busy.value && !pending.value && canWrite.value);
let listToken = 0, detailToken = 0, principal = 0, writeToken = 0, viewToken = 0;
const viewKey = "henukit.console.membership-view.v1";
const listPanel = ref<HTMLElement>();
const drafts = new Map<string, string>();
const confirmedMemberships = new Map<string, ConsoleAccountMembership>();
onBeforeUnmount(() => { principal++; listToken++; detailToken++; writeToken++; });
function current(generation: number, actor: string | undefined) {
  return principal === generation && actor === props.operatorID && props.authState === "authenticated" && canWrite.value;
}
function persistView() {
  if (!props.operatorID) return;
  try { sessionStorage.setItem(viewKey, JSON.stringify({ version: 1, operator_id: props.operatorID, page: activeQuery.value ? 1 : currentPage.value, selected_id: activeQuery.value ? "" : account.value?.id ?? "" })); } catch { /* context is optional */ }
}
function membershipLabel(value: ConsoleAccountMembership) { return value.plan === "lifetime" && value.lifetime ? "终身会员" : "免费会员"; }
function accountMembershipLabel(value: ConsoleMembershipAccountSummary) { return value.membership ? membershipLabel(value.membership) : "未开通会员账户"; }
function selectAccount(value: ConsoleMembershipAccountSummary) {
  if (account.value) drafts.set(account.value.id, reason.value);
  viewToken++; detailToken++;
  account.value = value;
  membership.value = value.membership ? { ...value.membership, userID: value.id } : undefined;
  detailState.value = value.membership ? "ready" : "not_found";
  reason.value = drafts.get(value.id) ?? ""; confirm.value = undefined; feedback.value = "";
  persistView();
}
function loseAccess(state: "signed_out" | "denied") {
  listToken++; detailToken++; viewToken++;
  accounts.value = []; account.value = undefined; membership.value = undefined;
  reason.value = ""; drafts.clear(); confirmedMemberships.clear(); confirm.value = undefined;
  workspaceState.value = state; listState.value = "idle"; detailState.value = "idle";
}
async function returnToList() {
  await nextTick();
  listPanel.value?.querySelector<HTMLButtonElement>('[aria-current="true"]')?.focus();
}
async function loadAccounts(page = 1, restoredID = "") {
  if (workspaceState.value !== "ready") return;
  const token = ++listToken, generation = principal, actor = props.operatorID;
  viewToken++; detailToken++;
  listState.value = "loading";
  if (account.value) drafts.set(account.value.id, reason.value);
  account.value = undefined; membership.value = undefined; detailState.value = "idle"; confirm.value = undefined; feedback.value = "";
  if (page === 1) activeQuery.value = searchQuery.value.trim();
  const result = await searchAccountMemberships({ query: activeQuery.value, page });
  if (token !== listToken || !current(generation, actor)) return;
  if (result.state === "authenticated") {
    accounts.value = result.page.accounts.map((item) => {
      const confirmed = confirmedMemberships.get(item.id);
      return item.membership && confirmed && confirmed.version > item.membership.version ? { ...item, membership: confirmed } : item;
    });
    currentPage.value = page; nextPage.value = result.page.next_page;
    listState.value = accounts.value.length ? "ready" : "empty";
    const restored = accounts.value.find((item) => item.id === restoredID);
    if (restored) selectAccount(restored);
    persistView(); return;
  }
  accounts.value = []; nextPage.value = null;
  if (result.state === "signed_out" || result.state === "denied") loseAccess(result.state);
  else listState.value = result.state === "invalid" ? "invalid" : "unavailable";
}
async function loadMembership(userID: string) {
  if (account.value?.id !== userID || workspaceState.value !== "ready") return;
  const token = ++detailToken, generation = principal, actor = props.operatorID;
  detailState.value = "loading"; membership.value = undefined;
  const result = await fetchAccountMembership(userID);
  if (token !== detailToken || account.value?.id !== userID || !current(generation, actor)) return;
  if (result.state === "authenticated") {
    membership.value = { ...result.membership, userID }; account.value = { ...account.value, membership: result.membership };
    accounts.value = accounts.value.map((item) => item.id === userID ? { ...item, membership: result.membership } : item);
    detailState.value = "ready";
  } else if (result.state === "signed_out" || result.state === "denied") loseAccess(result.state);
  else {
    detailState.value = result.state === "not_found" ? "not_found" : result.state === "invalid" ? "invalid" : "unavailable";
    if (result.state === "not_found") {
      account.value = { ...account.value, membership: null };
      accounts.value = accounts.value.map((item) => item.id === userID ? { ...item, membership: null } : item);
    }
  }
}
function validOutcome(command: PendingMembershipOperation, value: ConsoleAccountMembership) {
  return value.version === command.expected_version + 1 && value.lifetime === (command.operation === "membership_grant") && value.plan === (command.operation === "membership_grant" ? "lifetime" : "free");
}
async function query(command: PendingMembershipOperation): Promise<MembershipResult> {
  const result = await resolveAccountOperation(command.operation, command.idempotency_key);
  if (result.state !== "succeeded") return { state: result.state };
  const value = result.result.result;
  return value && "membership" in value ? { state: "succeeded", membership: value.membership } : { state: "unavailable" };
}
async function settle(command: PendingMembershipOperation, result: MembershipResult, view: number) {
  const selected = () => view === viewToken && account.value?.id === command.resource_id;
  if (result.state === "succeeded" && validOutcome(command, result.membership)) {
    const confirmed = confirmedMemberships.get(command.resource_id);
    if (!confirmed || confirmed.version < result.membership.version) confirmedMemberships.set(command.resource_id, result.membership);
    pending.value = undefined; clearPendingMembershipOperation();
    accounts.value = accounts.value.map((item) => item.id === command.resource_id && (!item.membership || item.membership.version <= result.membership.version) ? { ...item, membership: result.membership } : item);
    const currentMembership = membership.value?.userID === command.resource_id ? membership.value : undefined;
    const newerRevision = currentMembership && currentMembership.version > result.membership.version;
    if (account.value?.id === command.resource_id && currentMembership && !newerRevision) {
      membership.value = { ...result.membership, userID: command.resource_id };
      account.value = { ...account.value, membership: result.membership };
      detailState.value = "ready";
    }
    if (selected()) {
      reason.value = ""; drafts.delete(command.resource_id);
      feedback.value = newerRevision ? "原会员操作已确认；当前权益保留较新版本。" : command.operation === "membership_grant" ? "终身会员权益已发放。" : "终身会员权益已撤销。";
    } else if (!account.value && view === viewToken) feedback.value = command.operation === "membership_grant" ? "原终身会员发放操作已确认。" : "原终身会员撤销操作已确认。";
  } else if (result.state === "conflict") {
    pending.value = undefined; clearPendingMembershipOperation();
    if (selected()) {
      feedback.value = "会员版本已变化；操作原因已保留，请核对最新权益后重新确认。";
      await loadMembership(command.resource_id);
    }
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
async function finish(command: PendingMembershipOperation, input: ConsoleMembershipMutationRequest) {
  if (!writable.value || command.operator_id !== props.operatorID || account.value?.id !== command.resource_id || membership.value?.version !== command.expected_version) return;
  if (!writePendingMembershipOperation(command)) { feedback.value = "浏览器无法保存操作恢复信息，操作未提交；请允许会话存储后重试。"; return; }
  pending.value = command; busy.value = true;
  const generation = principal, actor = props.operatorID, token = ++writeToken, view = viewToken;
  feedback.value = "正在提交会员权益变更…";
  let result: MembershipResult = command.operation === "membership_grant" ? await grantAccountMembership(command.resource_id, input, command.idempotency_key) : await revokeAccountMembership(command.resource_id, input, command.idempotency_key);
  if (!current(generation, actor) || token !== writeToken) return;
  if (result.state !== "conflict" && result.state !== "signed_out" && (result.state !== "succeeded" || !validOutcome(command, result.membership))) {
    result = await query(command);
    if (!current(generation, actor) || token !== writeToken) return;
  }
  await settle(command, result, view);
  if (current(generation, actor) && token === writeToken) busy.value = false;
}
function requestMutation() {
  const value = membership.value, inputReason = reason.value.trim();
  if (!value || !writable.value || !inputReason || inputReason.length > 1000 || !props.operatorID || account.value?.id !== value.userID || confirm.value) return;
  const kind = value.plan === "free" ? "grant" : "revoke";
  confirm.value = { kind, input: { reason: inputReason, expected_version: value.version }, command: { version: 1, operator_id: props.operatorID, operation: kind === "grant" ? "membership_grant" : "membership_revoke", resource_id: value.userID, expected_version: value.version, idempotency_key: `idem_account_membership_${kind}_${crypto.randomUUID()}` } };
}
function confirmMutation() {
  const value = confirm.value;
  if (!value || !writable.value) return;
  confirm.value = undefined; void finish(value.command, value.input);
}
function cancelMutation() { confirm.value = undefined; feedback.value = "已取消，未执行任何操作。"; }
watch(() => [props.authState, props.operatorID, canWrite.value] as const, ([auth, actor, allowed]) => {
  principal++; listToken++; detailToken++; writeToken++; viewToken++;
  accounts.value = []; account.value = undefined; membership.value = undefined; confirm.value = undefined; pending.value = undefined;
  reason.value = ""; searchQuery.value = ""; activeQuery.value = ""; feedback.value = ""; drafts.clear(); confirmedMemberships.clear(); busy.value = false;
  listState.value = "idle"; detailState.value = "idle"; currentPage.value = 1; nextPage.value = null;
  workspaceState.value = auth === "authenticated" && actor && allowed ? "ready" : auth === "loading" ? "loading" : auth === "signed_out" ? "signed_out" : auth === "denied" || !allowed ? "denied" : "unavailable";
  if (workspaceState.value !== "ready" || !actor) return;
  pending.value = readPendingMembershipOperation(actor);
  let page = 1, selected = "";
  try {
    const value = JSON.parse(sessionStorage.getItem(viewKey) ?? "null");
    if (value?.version === 1 && value.operator_id === actor && Number.isSafeInteger(value.page) && value.page > 0 && value.page <= 100000 && typeof value.selected_id === "string") { page = value.page; selected = value.selected_id; }
    else sessionStorage.removeItem(viewKey);
  } catch { /* invalid context is ignored */ }
  void loadAccounts(page, selected);
  if (pending.value) void resolvePending();
}, { immediate: true });
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

    <p v-if="feedback || pending" class="operation-notice mt-5" role="status">
      {{ feedback }}
      <span v-if="pending" class="mt-2 block text-sm">有一项原会员操作结果尚未确认，查询不会再次变更权益。</span>
      <Button v-if="pending && !busy && workspaceState === 'ready'" class="mt-3" @click="resolvePending">查询原操作结果</Button>
    </p>

    <div v-if="workspaceState === 'loading'" class="operation-state" aria-busy="true">正在验证会员操作权限…</div>
    <div v-else-if="workspaceState === 'signed_out'" class="operation-state">登录状态已过期，请重新登录后再操作。</div>
    <div v-else-if="workspaceState === 'denied'" class="operation-state">当前账户没有会员权益运营权限，请联系管理员开通。</div>
    <div v-else-if="workspaceState === 'unavailable'" class="operation-state"><p>会员权益服务暂时不可用，请稍后再试。</p></div>

    <div v-else class="mt-6 grid gap-5 xl:grid-cols-[minmax(20rem,.9fr)_minmax(0,1.1fr)]">
      <section ref="listPanel" class="operation-panel !mt-0" aria-labelledby="membership-account-list-heading">
        <h2 id="membership-account-list-heading">选择用户</h2>
        <p class="mt-2 text-sm leading-6 text-muted-foreground">每页最多 20 位用户；可按显示名称或完整邮箱缩小结果。检索内容仅在本页保留，刷新后请重新输入。</p>
        <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="loadAccounts(1)">
          <Label class="min-w-0 flex-1 grid gap-2">
            显示名称或邮箱
            <Input v-model="searchQuery" autocomplete="off" placeholder="姓名或 student@henu.edu.cn" />
          </Label>
          <Button class="sm:self-end" type="submit" :disabled="listState === 'loading'">搜索</Button>
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
              :aria-current="account?.id === item.id ? 'true' : undefined"
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
          <Button class="mb-4" variant="ghost" @click="returnToList">返回用户列表</Button>
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
              <Button class="mt-3" type="submit" :disabled="!writable || !reason.trim()">
                {{ membership.plan === 'free' ? '发放终身会员' : '撤销终身会员' }}
              </Button>
            </form>

            <div v-else class="mt-5 border-t border-border pt-5" data-membership-confirm-step>
              <p class="font-medium">确认{{ confirm.kind === "grant" ? "发放" : "撤销" }}终身会员权益？</p>
              <p class="mt-2 text-sm leading-6 text-muted-foreground">
                将向「{{ accountName(account.display_name) }} · {{ account.email }}」{{ confirm.kind === "grant" ? "发放" : "撤销" }}终身会员权益，写入不可变审计事件并向该用户创建持久化通知；提交后立即生效，之后可通过相反操作调整。
              </p>
              <div class="mt-3 flex flex-wrap gap-3">
                <Button :disabled="!writable" @click="confirmMutation">确认执行</Button>
                <Button variant="outline" :disabled="busy" @click="cancelMutation">取消</Button>
              </div>
            </div>
          </template>
        </template>
      </Card>
    </div>
  </section>
</template>
