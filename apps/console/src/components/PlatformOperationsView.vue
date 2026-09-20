<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { PageHeader } from "@/components/ui";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import { fetchPlatformOperations, resolvePlatformOperation, revokePlatformSession, searchPlatformOperationAccounts, updatePlatformAccess, type PlatformAccessGrantInput, type PlatformOperationWriteResult, type PlatformOperationsAuditEvent, type PlatformOperationsPageRequest, type PlatformOperationsSnapshot } from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable" }>();
const operations = ref<PlatformOperationsSnapshot>();
const state = ref<"loading" | "ready" | "denied" | "rate_limited" | "unavailable">("loading");
const notice = ref("");
const pending = ref<{ operation: "session_revoke" | "access_update"; key: string }>();
const statuses = ref<Record<string, "active" | "suspended" | "deleted">>({});
const grants = ref<Record<string, PlatformAccessGrantInput[]>>({});
const confirmTarget = ref<string>();
const pages = ref<PlatformOperationsPageRequest>({ accounts_page: 1, sessions_page: 1, inbox_page: 1, audit_page: 1 });
const accountQueryInput = ref("");
const submittedAccountQuery = ref("");
const accountSearchBusy = ref(false);
type PageSection = "accounts_page" | "sessions_page" | "inbox_page" | "audit_page";
const cursorFields: Record<PageSection, "accounts_cursor" | "sessions_cursor" | "inbox_cursor" | "audit_cursor"> = {
  accounts_page: "accounts_cursor", sessions_page: "sessions_cursor", inbox_page: "inbox_cursor", audit_page: "audit_cursor",
};
const cursorHistory = ref<Record<PageSection, Array<string | undefined>>>({ accounts_page: [undefined], sessions_page: [undefined], inbox_page: [undefined], audit_page: [undefined] });
const searchCursorHistory = ref<Array<string | undefined>>([undefined]);

// Platform Core owns both identity fields. Display names are optional for
// legacy rows, while the verified email remains the stable human identifier.
function personName(value: unknown) {
  const name = (value as { display_name?: unknown } | null | undefined)?.display_name;
  return typeof name === "string" && name.trim() ? name.trim() : "";
}

function personLabel(value: unknown) {
  return personName(value) || "未设置姓名";
}

function personEmail(value: unknown) {
  const email = (value as { email?: unknown } | null | undefined)?.email;
  return typeof email === "string" && email.trim() ? email.trim() : "邮箱不可用";
}

function idempotencyKey(prefix: string) {
  return `idem_console_${prefix}_${crypto.randomUUID()}`;
}

async function load(nextPages: PlatformOperationsPageRequest = pages.value, preserveAccountDrafts = false) {
  state.value = "loading";
  const previousAccounts = preserveAccountDrafts ? operations.value?.accounts : undefined;
  const previousAccountPagination = preserveAccountDrafts ? operations.value?.pagination.accounts : undefined;
  const result = await fetchPlatformOperations(nextPages);
  if (result.state === "authenticated") {
    operations.value = result.operations;
    if (previousAccounts && previousAccountPagination) {
      operations.value.accounts = previousAccounts;
      operations.value.pagination.accounts = previousAccountPagination;
    }
    pages.value = {
      accounts_page: result.operations.pagination.accounts.page,
      sessions_page: result.operations.pagination.sessions.page,
      inbox_page: result.operations.pagination.inbox_items.page,
      audit_page: result.operations.pagination.audit.page,
      snapshot_at: result.operations.generated_at,
      accounts_cursor: nextPages.accounts_cursor,
      sessions_cursor: nextPages.sessions_cursor,
      inbox_cursor: nextPages.inbox_cursor,
      audit_cursor: nextPages.audit_cursor,
    };
    if (!preserveAccountDrafts) {
      statuses.value = Object.fromEntries(result.operations.accounts.map((account) => [account.id, account.status]));
      grants.value = Object.fromEntries(result.operations.accounts.map((account) => [account.id, structuredClone(account.grants)]));
    }
    state.value = "ready";
  } else state.value = result.state === "denied" || result.state === "signed_out" ? "denied" : result.state === "rate_limited" ? "rate_limited" : "unavailable";
}

async function loadPage(section: PageSection, page: number) {
  if (state.value === "loading" || accountSearchBusy.value || page < 1 || !operations.value) return;
  const pagination = section === "accounts_page" ? operations.value.pagination.accounts : section === "sessions_page" ? operations.value.pagination.sessions : section === "inbox_page" ? operations.value.pagination.inbox_items : operations.value.pagination.audit;
  const cursor = page === pagination.page + 1 ? pagination.next_cursor ?? undefined : cursorHistory.value[section][page - 1];
  if (page > 1 && !cursor) return;
  cursorHistory.value[section][page - 1] = cursor;
  await load({ ...pages.value, [section]: page, [cursorFields[section]]: cursor }, section !== "accounts_page");
}

async function searchAccounts(page = 1, cursor?: string) {
  if (!operations.value || accountSearchBusy.value || page < 1) return;
  accountSearchBusy.value = true;
  const query = accountQueryInput.value.trim();
  const result = await searchPlatformOperationAccounts({ query, page, snapshot_at: operations.value.generated_at, ...(cursor ? { cursor } : {}) });
  if (result.state === "authenticated") {
    submittedAccountQuery.value = query;
    operations.value.accounts = result.page.accounts;
    operations.value.pagination.accounts = { page, next_page: result.page.next_page, next_cursor: result.page.next_cursor };
    if (result.page.next_cursor) searchCursorHistory.value[page] = result.page.next_cursor;
    statuses.value = Object.fromEntries(result.page.accounts.map((account) => [account.id, account.status]));
    grants.value = Object.fromEntries(result.page.accounts.map((account) => [account.id, structuredClone(account.grants)]));
    notice.value = result.page.accounts.length ? "账户搜索结果已更新。" : "没有匹配的账户。";
  } else {
    notice.value = result.state === "denied" ? "当前账户没有平台运营读取权限。" : result.state === "invalid" ? "请输入有效的账户搜索条件。" : result.state === "rate_limited" ? "请求过于频繁，请稍后重试。" : "账户搜索暂不可用，请稍后重试。";
  }
  accountSearchBusy.value = false;
}

function loadAccountPage(page: number) {
  if (submittedAccountQuery.value) {
    accountQueryInput.value = submittedAccountQuery.value;
    const current = operations.value?.pagination.accounts;
    const cursor = current && page === current.page + 1 ? current.next_cursor ?? undefined : searchCursorHistory.value[page - 1];
    if (page > 1 && !cursor) return;
    void searchAccounts(page, cursor);
    return;
  }
  void loadPage("accounts_page", page);
}

function clearAccountSearch() {
  accountQueryInput.value = "";
  submittedAccountQuery.value = "";
  searchCursorHistory.value = [undefined];
  cursorHistory.value.accounts_page = [undefined];
  void load({ ...pages.value, accounts_page: 1, accounts_cursor: undefined });
}

function refreshSnapshot() {
  accountQueryInput.value = "";
  submittedAccountQuery.value = "";
  searchCursorHistory.value = [undefined];
  cursorHistory.value = { accounts_page: [undefined], sessions_page: [undefined], inbox_page: [undefined], audit_page: [undefined] };
  notice.value = "";
  void load({ accounts_page: 1, sessions_page: 1, inbox_page: 1, audit_page: 1 });
}

async function refreshAfterWrite() {
  const activeQuery = submittedAccountQuery.value;
  cursorHistory.value = { accounts_page: [undefined], sessions_page: [undefined], inbox_page: [undefined], audit_page: [undefined] };
  searchCursorHistory.value = [undefined];
  await load({ accounts_page: 1, sessions_page: 1, inbox_page: 1, audit_page: 1 });
  if (activeQuery) {
    accountQueryInput.value = activeQuery;
    await searchAccounts(1);
  }
}

function handleWrite(result: PlatformOperationWriteResult, operation: "session_revoke" | "access_update", key: string) {
  if (result.state === "succeeded") {
    pending.value = undefined;
    notice.value = "操作已完成并写入审计记录。";
    void refreshAfterWrite();
  } else if (result.state === "unknown") {
    pending.value = { operation, key };
    notice.value = "结果还没确认，请勿重复提交，稍后刷新查看。";
  } else {
    notice.value = result.state === "conflict" ? "数据有变化，请刷新后重试。" : "操作没有完成，请稍后刷新页面重试。";
  }
}

function mailLabel(key: string) {
  return key === "accepted" ? "已接收" : key === "pending" ? "等待中" : key === "processing" ? "处理中" : key === "delivered" ? "已送达" : key === "failed" ? "发送失败" : key === "retry_due" ? "待重试" : key === "dead_letters" ? "无法送达" : key;
}

function inboxStatusLabel(status: string) {
  return status === "open" ? "待处理" : status === "in_progress" ? "处理中" : status === "blocked" ? "受阻" : status === "resolved" ? "已解决" : status === "archived" ? "已归档" : status;
}

function sessionKindLabel(kind: string) {
  return kind === "core" ? "核心" : kind === "client_exchange" ? "客户端" : kind;
}

function decisionLabel(decision: string) {
  return decision === "allowed" ? "允许" : decision === "denied" ? "拒绝" : decision;
}

function healthLabel(value: string) {
  return value === "ready" ? "正常" : value === "unavailable" ? "不可用" : "状态未知";
}

function healthBadgeStatus(value: string): "ok" | "unavailable" | "denied" {
  return value === "ready" ? "ok" : value === "unavailable" ? "unavailable" : "denied";
}

// 授权审计 reason_code 映射：授权检查（GRANTED / SESSION_* / ACCOUNT_NOT_ACTIVE …）
// 与平台运营写操作（access_update_succeeded / session_revoke_succeeded）。未知值兜底为「其他原因」并附原码小字。
const reasonLabels: Record<string, string> = {
  GRANTED: "权限授予",
  PERMISSION_OR_SCOPE_MISSING: "缺少权限或范围",
  SESSION_REVOKED: "会话已撤销",
  SESSION_EXPIRED: "会话已过期",
  PARENT_SESSION_REVOKED: "上级会话已撤销",
  PARENT_SESSION_EXPIRED: "上级会话已过期",
  ACCOUNT_NOT_ACTIVE: "账户未激活",
  access_update_succeeded: "访问设置更新成功",
  session_revoke_succeeded: "会话撤销成功",
  permission_granted: "权限授予",
};

function reasonLabel(code: string) {
  return reasonLabels[code] ?? "其他原因";
}

function unknownReason(code: string) {
  return !(code in reasonLabels);
}

function targetKindLabel(kind: string) {
  return kind === "platform" ? "平台" : kind === "product" ? "产品" : kind === "resource" ? "资源" : kind;
}

function targetLabel(event: PlatformOperationsAuditEvent) {
  const parts = [targetKindLabel(event.target_kind)];
  if (event.target_product_code) parts.push(event.target_product_code);
  if (event.target_resource_type) parts.push(event.target_resource_type);
  if (event.target_resource_id) parts.push(event.target_resource_id);
  return parts.join(" / ");
}

async function revoke(sessionID: string) {
  if (accountSearchBusy.value) return;
  const key = idempotencyKey("revoke");
  handleWrite(await revokePlatformSession(sessionID, key), "session_revoke", key);
}

function accountStatus(accountID: string): "active" | "suspended" | "deleted" | undefined {
  return operations.value?.accounts.find((account) => account.id === accountID)?.status;
}

// 改为「已删除」是高风险写操作：未确认前不发起任何写入，取消则完全不落库。
async function saveAccess(account: PlatformOperationsSnapshot["accounts"][number], confirmed = false) {
  if (accountSearchBusy.value) return;
  const nextStatus = statuses.value[account.id];
  if (!confirmed && nextStatus === "deleted" && accountStatus(account.id) !== "deleted") {
    confirmTarget.value = account.id;
    return;
  }
  confirmTarget.value = undefined;
  const key = idempotencyKey("access");
  handleWrite(await updatePlatformAccess(account.id, { expected_revision: account.authorization_revision, status: nextStatus, grants: grants.value[account.id] }, key), "access_update", key);
}

function cancelConfirm() {
  confirmTarget.value = undefined;
}

function addGrant(accountID: string) {
  if (accountSearchBusy.value) return;
  grants.value[accountID].push({ role_code: "operations-operator", scope: { kind: "platform" } });
}

function removeGrant(accountID: string, index: number) {
  if (accountSearchBusy.value) return;
  grants.value[accountID].splice(index, 1);
}

function normalizeScope(grant: PlatformAccessGrantInput) {
  if (grant.scope.kind === "platform") {
    delete grant.scope.product_code; delete grant.scope.resource_type; delete grant.scope.resource_id;
  } else if (grant.scope.kind === "product") {
    delete grant.scope.resource_type; delete grant.scope.resource_id;
  }
}

async function resolveUnknown() {
  if (!pending.value) return;
  handleWrite(await resolvePlatformOperation(pending.value.operation, pending.value.key), pending.value.operation, pending.value.key);
}

const mailTotal = computed(() => operations.value ? Object.values(operations.value.mail).reduce((sum, value) => sum + value, 0) : 0);
const canWrite = computed(() => operations.value?.access_context.permissions.includes("platform.operations.write") ?? false);
onMounted(() => { if (props.authState !== "signed_out") void load(); });
</script>

<template>
  <section aria-labelledby="operations-heading">
    <PageHeader
      eyebrow="平台运营"
      title="平台运营工作台"
      description="这里展示账户、登录、邮件与审计的运营状态。"
      title-id="operations-heading"
    >
      <div class="access-context"><span>{{ canWrite ? "可读写" : "只读" }}</span><strong>平台权限</strong></div>
    </PageHeader>

    <p v-if="notice" class="operation-notice" role="status">{{ notice }} <button v-if="pending" type="button" @click="resolveUnknown">查询结果</button></p>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取运营状态…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前登录账户没有平台运营权限，请联系管理员。</div>
    <div v-else-if="state === 'rate_limited'" class="operation-state">请求过于频繁，请稍后重试。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state">运营数据暂不可用。<button type="button" @click="load()">重试</button></div>

    <template v-else-if="operations">
      <p class="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground"><span>数据快照 {{ localDateTime(operations.generated_at) }}</span><button type="button" class="secondary-action" :disabled="accountSearchBusy" @click="refreshSnapshot">刷新数据</button></p>
      <div class="operation-summary-grid">
        <article><span>本页账户</span><strong>{{ operations.accounts.length }}</strong></article>
        <article><span>本页登录会话</span><strong>{{ operations.sessions.length }}</strong></article>
        <article><span>邮件事件</span><strong>{{ mailTotal }}</strong></article>
        <article><span>本页收件箱</span><strong>{{ operations.inbox_items.length }}</strong></article>
        <article><span>数据库</span><StatusBadge :status="healthBadgeStatus(operations.dependencies.postgres)">{{ healthLabel(operations.dependencies.postgres) }}</StatusBadge></article>
        <article><span>缓存</span><StatusBadge :status="healthBadgeStatus(operations.dependencies.redis)">{{ healthLabel(operations.dependencies.redis) }}</StatusBadge></article>
      </div>

      <section class="operation-panel" aria-labelledby="accounts-heading">
        <h2 id="accounts-heading">账户、角色与权限</h2>
        <form class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end" @submit.prevent="searchAccounts(1)">
          <label class="grid flex-1 gap-1 text-sm">账户搜索<input v-model="accountQueryInput" class="rounded-md border px-3 py-2" minlength="2" maxlength="100" placeholder="显示名称或完整邮箱" /></label>
          <button type="submit" :disabled="accountSearchBusy">搜索账户</button>
          <button v-if="submittedAccountQuery" type="button" class="secondary-action" :disabled="accountSearchBusy" @click="clearAccountSearch">清除搜索</button>
        </form>
        <div class="operation-list">
          <template v-for="account in operations.accounts" :key="account.id">
            <article class="operation-row">
              <div class="account-access-editor">
                <strong>{{ personLabel(account) }}</strong>
                <p class="break-all">{{ personEmail(account) }} · 授权版本 {{ account.authorization_revision }} · {{ account.email_verified ? "邮箱已验证" : "邮箱未验证" }}</p>
                <div v-for="(grant, index) in grants[account.id]" :key="index" class="grant-editor">
                  <label>角色代码<input v-model="grant.role_code" :disabled="!canWrite || accountSearchBusy" pattern="[a-z][a-z0-9-]+" /></label>
                  <label>权限范围<select v-model="grant.scope.kind" :disabled="!canWrite || accountSearchBusy" @change="normalizeScope(grant)"><option value="platform">平台</option><option value="product">产品</option><option value="resource">资源</option></select></label>
                  <label v-if="grant.scope.kind !== 'platform'">产品代码<input v-model="grant.scope.product_code" :disabled="!canWrite || accountSearchBusy" /></label>
                  <label v-if="grant.scope.kind === 'resource'">资源类型<input v-model="grant.scope.resource_type" :disabled="!canWrite || accountSearchBusy" /></label>
                  <label v-if="grant.scope.kind === 'resource'">资源 ID<input v-model="grant.scope.resource_id" :disabled="!canWrite || accountSearchBusy" /></label>
                  <button v-if="canWrite" type="button" class="secondary-action" :disabled="accountSearchBusy" @click="removeGrant(account.id, index)">删除授权</button>
                </div>
                <button v-if="canWrite" type="button" class="secondary-action" :disabled="accountSearchBusy" @click="addGrant(account.id)">新增角色 / 权限</button>
              </div>
              <div class="operation-actions"><label>账户状态<select v-model="statuses[account.id]" :disabled="!canWrite || accountSearchBusy"><option value="active">正常</option><option value="suspended">已停用</option><option value="deleted">已删除</option></select></label><button v-if="canWrite" type="button" :disabled="accountSearchBusy" @click="saveAccess(account)">保存访问设置</button><span v-else>只读权限</span></div>
            </article>
            <div v-if="confirmTarget === account.id" class="mb-3 rounded-md border border-destructive/30 bg-destructive/5 p-3">
              <strong>确认将账户标记为「已删除」？</strong>
              <p class="mt-1 text-sm text-muted-foreground">标记后该账户将无法登录，其授权检查将不再通过；账户数据不会被物理删除，此状态之后可再改回（可逆）。</p>
              <div class="mt-2 flex flex-wrap gap-2">
                <button type="button" class="bg-destructive! border-destructive!" @click="saveAccess(account, true)">确认标记已删除</button>
                <button type="button" class="secondary-action" @click="cancelConfirm">取消</button>
              </div>
            </div>
          </template>
        </div>
        <div class="mt-4 flex items-center justify-between gap-3">
          <button type="button" class="secondary-action" :disabled="operations.pagination.accounts.page <= 1 || accountSearchBusy" @click="loadAccountPage(operations.pagination.accounts.page - 1)">账户上一页</button>
          <span>账户第 {{ operations.pagination.accounts.page }} 页</span>
          <button type="button" class="secondary-action" :disabled="operations.pagination.accounts.next_page === null || accountSearchBusy" @click="operations.pagination.accounts.next_page && loadAccountPage(operations.pagination.accounts.next_page)">账户下一页</button>
        </div>
      </section>

      <section class="operation-panel" aria-labelledby="sessions-heading"><h2 id="sessions-heading">登录会话</h2><div class="operation-list"><article v-for="session in operations.sessions" :key="session.id" class="operation-row"><div><strong>{{ personLabel(session) }}</strong><p class="break-all">{{ personEmail(session) }} · {{ sessionKindLabel(session.kind) }} · 到期 {{ localDateTime(session.expires_at) }}</p></div><button v-if="canWrite" type="button" :disabled="Boolean(session.revoked_at) || accountSearchBusy" @click="revoke(session.id)">{{ session.revoked_at ? "已撤销" : "撤销登录" }}</button><span v-else>只读权限</span></article></div><div class="mt-4 flex items-center justify-between gap-3"><button type="button" class="secondary-action" :disabled="operations.pagination.sessions.page <= 1 || accountSearchBusy" @click="loadPage('sessions_page', operations.pagination.sessions.page - 1)">会话上一页</button><span>会话第 {{ operations.pagination.sessions.page }} 页</span><button type="button" class="secondary-action" :disabled="operations.pagination.sessions.next_page === null || accountSearchBusy" @click="operations.pagination.sessions.next_page && loadPage('sessions_page', operations.pagination.sessions.next_page)">会话下一页</button></div></section>

      <div class="operation-two-column">
        <section class="operation-panel"><h2>邮件基础设施</h2><dl class="mail-grid"><template v-for="(value, key) in operations.mail" :key="key"><dt>{{ mailLabel(key) }}</dt><dd>{{ value }}</dd></template></dl></section>
        <section class="operation-panel"><h2>运营收件箱</h2><article v-for="item in operations.inbox_items" :key="item.id" class="compact-row"><strong>{{ item.source_product_code }} / {{ item.source_resource_type }}</strong><span>{{ item.source_resource_id }} · {{ inboxStatusLabel(item.status) }} · v{{ item.version }}</span></article><p v-if="!operations.inbox_items.length">暂无引用项</p><div class="mt-4 flex items-center justify-between gap-3"><button type="button" class="secondary-action" :disabled="operations.pagination.inbox_items.page <= 1 || accountSearchBusy" @click="loadPage('inbox_page', operations.pagination.inbox_items.page - 1)">收件箱上一页</button><span>收件箱第 {{ operations.pagination.inbox_items.page }} 页</span><button type="button" class="secondary-action" :disabled="operations.pagination.inbox_items.next_page === null || accountSearchBusy" @click="operations.pagination.inbox_items.next_page && loadPage('inbox_page', operations.pagination.inbox_items.next_page)">收件箱下一页</button></div></section>
      </div>
      <section class="operation-panel"><h2>授权审计</h2><article v-for="event in operations.audit" :key="event.request_id + event.created_at" class="compact-row"><strong>{{ decisionLabel(event.decision) }} · {{ event.permission_code }}</strong><span class="break-all">{{ personLabel(event) }} · {{ personEmail(event) }} · {{ localDateTime(event.created_at) }} · 目标 {{ targetLabel(event) }}</span><span>原因：{{ reasonLabel(event.reason_code) }}<template v-if="unknownReason(event.reason_code)">（{{ event.reason_code }}）</template> · {{ event.request_id }}</span></article><p v-if="!operations.audit.length">暂无审计事件</p><div class="mt-4 flex items-center justify-between gap-3"><button type="button" class="secondary-action" :disabled="operations.pagination.audit.page <= 1 || accountSearchBusy" @click="loadPage('audit_page', operations.pagination.audit.page - 1)">审计上一页</button><span>审计第 {{ operations.pagination.audit.page }} 页</span><button type="button" class="secondary-action" :disabled="operations.pagination.audit.next_page === null || accountSearchBusy" @click="operations.pagination.audit.next_page && loadPage('audit_page', operations.pagination.audit.next_page)">审计下一页</button></div></section>
    </template>
  </section>
</template>
