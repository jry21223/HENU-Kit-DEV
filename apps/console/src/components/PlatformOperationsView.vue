<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { fetchPlatformOperations, resolvePlatformOperation, revokePlatformSession, updatePlatformAccess, type PlatformAccessGrantInput, type PlatformOperationWriteResult, type PlatformOperationsSnapshot } from "@/lib/console-gateway";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable" }>();
const operations = ref<PlatformOperationsSnapshot>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const notice = ref("");
const pending = ref<{ operation: "session_revoke" | "access_update"; key: string }>();
const statuses = ref<Record<string, "active" | "suspended" | "deleted">>({});
const grants = ref<Record<string, PlatformAccessGrantInput[]>>({});

function idempotencyKey(prefix: string) {
  return `idem_console_${prefix}_${crypto.randomUUID()}`;
}

async function load() {
  state.value = "loading";
  const result = await fetchPlatformOperations();
  if (result.state === "authenticated") {
    operations.value = result.operations;
    statuses.value = Object.fromEntries(result.operations.accounts.map((account) => [account.id, account.status]));
    grants.value = Object.fromEntries(result.operations.accounts.map((account) => [account.id, structuredClone(account.grants)]));
    state.value = "ready";
  } else state.value = result.state === "denied" || result.state === "signed_out" ? "denied" : "unavailable";
}

function handleWrite(result: PlatformOperationWriteResult, operation: "session_revoke" | "access_update", key: string) {
  if (result.state === "succeeded") {
    pending.value = undefined;
    notice.value = "操作已完成并写入审计记录。";
    void load();
  } else if (result.state === "unknown") {
    pending.value = { operation, key };
    notice.value = "结果未知，请查询持久化结果，勿直接重复提交。";
  } else {
    notice.value = result.state === "conflict" ? "状态或幂等键冲突，请刷新后重试。" : "操作未完成。";
  }
}

async function revoke(sessionID: string) {
  const key = idempotencyKey("revoke");
  handleWrite(await revokePlatformSession(sessionID, key), "session_revoke", key);
}

async function saveAccess(account: PlatformOperationsSnapshot["accounts"][number]) {
  const key = idempotencyKey("access");
  handleWrite(await updatePlatformAccess(account.id, { expected_revision: account.authorization_revision, status: statuses.value[account.id], grants: grants.value[account.id] }, key), "access_update", key);
}

function addGrant(accountID: string) {
  grants.value[accountID].push({ role_code: "operations-operator", scope: { kind: "platform" } });
}

function removeGrant(accountID: string, index: number) {
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
    <div class="overview-hero">
      <div>
        <p class="eyebrow">Platform Operations</p>
        <h1 id="operations-heading" class="mt-2 text-2xl font-bold tracking-[-0.03em] sm:text-3xl">平台运营工作台</h1>
        <p class="mt-2 max-w-3xl text-base leading-7 text-[var(--hk-ink-muted)]">账户与 Scope、Session、邮件状态、Operations Inbox、审计和依赖健康均来自 Platform Core；邮件正文、收件人、令牌和密钥不会下发。</p>
      </div>
      <div class="access-context"><span>{{ canWrite ? "platform.operations.read / write" : "platform.operations.read" }}</span><strong>scope:platform</strong></div>
    </div>

    <p v-if="notice" class="operation-notice" role="status">{{ notice }} <button v-if="pending" type="button" @click="resolveUnknown">查询结果</button></p>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取运营状态…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前 Session 缺少 platform.operations.read 或 platform Scope。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state">运营数据暂不可用。<button type="button" @click="load">重试</button></div>

    <template v-else-if="operations">
      <div class="operation-summary-grid">
        <article><span>账户</span><strong>{{ operations.accounts.length }}</strong></article>
        <article><span>Session</span><strong>{{ operations.sessions.length }}</strong></article>
        <article><span>邮件事件</span><strong>{{ mailTotal }}</strong></article>
        <article><span>Inbox</span><strong>{{ operations.inbox_items.length }}</strong></article>
        <article><span>Postgres</span><strong>{{ operations.dependencies.postgres }}</strong></article>
        <article><span>Redis</span><strong>{{ operations.dependencies.redis }}</strong></article>
      </div>

      <section class="operation-panel" aria-labelledby="accounts-heading">
        <h2 id="accounts-heading">账户、角色与 Scope</h2>
        <div class="operation-list">
          <article v-for="account in operations.accounts" :key="account.id" class="operation-row">
            <div class="account-access-editor"><strong>{{ account.id }}</strong><p>revision {{ account.authorization_revision }} · {{ account.email_verified ? "邮箱已验证" : "邮箱未验证" }}</p>
              <div v-for="(grant, index) in grants[account.id]" :key="index" class="grant-editor">
                <label>角色代码<input v-model="grant.role_code" :disabled="!canWrite" pattern="[a-z][a-z0-9-]+" /></label>
                <label>Scope<select v-model="grant.scope.kind" :disabled="!canWrite" @change="normalizeScope(grant)"><option value="platform">platform</option><option value="product">product</option><option value="resource">resource</option></select></label>
                <label v-if="grant.scope.kind !== 'platform'">产品代码<input v-model="grant.scope.product_code" :disabled="!canWrite" /></label>
                <label v-if="grant.scope.kind === 'resource'">资源类型<input v-model="grant.scope.resource_type" :disabled="!canWrite" /></label>
                <label v-if="grant.scope.kind === 'resource'">资源 ID<input v-model="grant.scope.resource_id" :disabled="!canWrite" /></label>
                <button v-if="canWrite" type="button" class="secondary-action" @click="removeGrant(account.id, index)">删除 grant</button>
              </div>
              <button v-if="canWrite" type="button" class="secondary-action" @click="addGrant(account.id)">新增 role / Scope</button>
            </div>
            <div class="operation-actions"><label>账户状态<select v-model="statuses[account.id]" :disabled="!canWrite"><option value="active">active</option><option value="suspended">suspended</option><option value="deleted">deleted</option></select></label><button v-if="canWrite" type="button" @click="saveAccess(account)">保存访问设置</button><span v-else>只读权限</span></div>
          </article>
        </div>
      </section>

      <section class="operation-panel" aria-labelledby="sessions-heading"><h2 id="sessions-heading">Session</h2><div class="operation-list"><article v-for="session in operations.sessions" :key="session.id" class="operation-row"><div><strong>{{ session.id }}</strong><p>{{ session.kind }} · user {{ session.user_id }} · 到期 {{ new Date(session.expires_at).toLocaleString() }}</p></div><button v-if="canWrite" type="button" :disabled="Boolean(session.revoked_at)" @click="revoke(session.id)">{{ session.revoked_at ? "已撤销" : "撤销 Session" }}</button><span v-else>只读权限</span></article></div></section>

      <div class="operation-two-column">
        <section class="operation-panel"><h2>邮件基础设施</h2><dl class="mail-grid"><template v-for="(value, key) in operations.mail" :key="key"><dt>{{ key }}</dt><dd>{{ value }}</dd></template></dl></section>
        <section class="operation-panel"><h2>Operations Inbox</h2><article v-for="item in operations.inbox_items" :key="item.id" class="compact-row"><strong>{{ item.source_product_code }} / {{ item.source_resource_type }}</strong><span>{{ item.source_resource_id }} · {{ item.status }} · v{{ item.version }}</span></article><p v-if="!operations.inbox_items.length">暂无引用项</p></section>
      </div>
      <section class="operation-panel"><h2>授权审计</h2><article v-for="event in operations.audit" :key="event.request_id + event.created_at" class="compact-row"><strong>{{ event.permission_code }} · {{ event.decision }}</strong><span>{{ event.request_id }} · {{ event.reason_code }}</span></article><p v-if="!operations.audit.length">暂无审计事件</p></section>
    </template>
  </section>
</template>
