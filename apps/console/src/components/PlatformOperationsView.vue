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
        <p class="eyebrow">平台运营</p>
        <h1 id="operations-heading" class="mt-2 text-2xl font-bold tracking-[-0.03em] sm:text-3xl">平台运营工作台</h1>
        <p class="mt-2 max-w-3xl text-base leading-7 text-[var(--hk-ink-muted)]">这里展示账户、登录、邮件与审计的运营状态。</p>
      </div>
      <div class="access-context"><span>{{ canWrite ? "可读写" : "只读" }}</span><strong>平台权限</strong></div>
    </div>

    <p v-if="notice" class="operation-notice" role="status">{{ notice }} <button v-if="pending" type="button" @click="resolveUnknown">查询结果</button></p>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取运营状态…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前登录账户没有平台运营权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state">运营数据暂不可用。<button type="button" @click="load">重试</button></div>

    <template v-else-if="operations">
      <div class="operation-summary-grid">
        <article><span>账户</span><strong>{{ operations.accounts.length }}</strong></article>
        <article><span>登录会话</span><strong>{{ operations.sessions.length }}</strong></article>
        <article><span>邮件事件</span><strong>{{ mailTotal }}</strong></article>
        <article><span>收件箱</span><strong>{{ operations.inbox_items.length }}</strong></article>
        <article><span>数据库</span><strong>{{ operations.dependencies.postgres }}</strong></article>
        <article><span>缓存</span><strong>{{ operations.dependencies.redis }}</strong></article>
      </div>

      <section class="operation-panel" aria-labelledby="accounts-heading">
        <h2 id="accounts-heading">账户、角色与权限</h2>
        <div class="operation-list">
          <article v-for="account in operations.accounts" :key="account.id" class="operation-row">
            <div class="account-access-editor"><strong>{{ account.id }}</strong><p>授权版本 {{ account.authorization_revision }} · {{ account.email_verified ? "邮箱已验证" : "邮箱未验证" }}</p>
              <div v-for="(grant, index) in grants[account.id]" :key="index" class="grant-editor">
                <label>角色代码<input v-model="grant.role_code" :disabled="!canWrite" pattern="[a-z][a-z0-9-]+" /></label>
                <label>权限范围<select v-model="grant.scope.kind" :disabled="!canWrite" @change="normalizeScope(grant)"><option value="platform">平台</option><option value="product">产品</option><option value="resource">资源</option></select></label>
                <label v-if="grant.scope.kind !== 'platform'">产品代码<input v-model="grant.scope.product_code" :disabled="!canWrite" /></label>
                <label v-if="grant.scope.kind === 'resource'">资源类型<input v-model="grant.scope.resource_type" :disabled="!canWrite" /></label>
                <label v-if="grant.scope.kind === 'resource'">资源 ID<input v-model="grant.scope.resource_id" :disabled="!canWrite" /></label>
                <button v-if="canWrite" type="button" class="secondary-action" @click="removeGrant(account.id, index)">删除授权</button>
              </div>
              <button v-if="canWrite" type="button" class="secondary-action" @click="addGrant(account.id)">新增角色 / 权限</button>
            </div>
            <div class="operation-actions"><label>账户状态<select v-model="statuses[account.id]" :disabled="!canWrite"><option value="active">正常</option><option value="suspended">已停用</option><option value="deleted">已删除</option></select></label><button v-if="canWrite" type="button" @click="saveAccess(account)">保存访问设置</button><span v-else>只读权限</span></div>
          </article>
        </div>
      </section>

      <section class="operation-panel" aria-labelledby="sessions-heading"><h2 id="sessions-heading">登录会话</h2><div class="operation-list"><article v-for="session in operations.sessions" :key="session.id" class="operation-row"><div><strong>{{ session.id }}</strong><p>{{ sessionKindLabel(session.kind) }} · 用户 {{ session.user_id }} · 到期 {{ new Date(session.expires_at).toLocaleString() }}</p></div><button v-if="canWrite" type="button" :disabled="Boolean(session.revoked_at)" @click="revoke(session.id)">{{ session.revoked_at ? "已撤销" : "撤销登录" }}</button><span v-else>只读权限</span></article></div></section>

      <div class="operation-two-column">
        <section class="operation-panel"><h2>邮件基础设施</h2><dl class="mail-grid"><template v-for="(value, key) in operations.mail" :key="key"><dt>{{ mailLabel(key) }}</dt><dd>{{ value }}</dd></template></dl></section>
        <section class="operation-panel"><h2>运营收件箱</h2><article v-for="item in operations.inbox_items" :key="item.id" class="compact-row"><strong>{{ item.source_product_code }} / {{ item.source_resource_type }}</strong><span>{{ item.source_resource_id }} · {{ inboxStatusLabel(item.status) }} · v{{ item.version }}</span></article><p v-if="!operations.inbox_items.length">暂无引用项</p></section>
      </div>
      <section class="operation-panel"><h2>授权审计</h2><article v-for="event in operations.audit" :key="event.request_id + event.created_at" class="compact-row"><strong>权限 · {{ decisionLabel(event.decision) }}</strong><span>{{ event.request_id }} · {{ event.reason_code }}</span></article><p v-if="!operations.audit.length">暂无审计事件</p></section>
    </template>
  </section>
</template>
