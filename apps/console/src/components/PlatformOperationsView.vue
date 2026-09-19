<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { PageHeader } from "@/components/ui";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import { fetchPlatformOperations, resolvePlatformOperation, revokePlatformSession, updatePlatformAccess, type PlatformAccessGrantInput, type PlatformOperationWriteResult, type PlatformOperationsAuditEvent, type PlatformOperationsSnapshot } from "@/lib/console-gateway";
import { localDateTime } from "@/lib/format";
import { clearPendingPlatformOperation, readPendingPlatformOperation, writePendingPlatformOperation, type PendingPlatformOperation } from "@/lib/pending-operations";

const props = defineProps<{ authState: "loading" | "authenticated" | "signed_out" | "denied" | "unavailable"; operatorID?: string }>();
const operations = ref<PlatformOperationsSnapshot>();
const state = ref<"loading" | "ready" | "denied" | "unavailable">("loading");
const notice = ref("");
const pending = ref<PendingPlatformOperation>();
const statuses = ref<Record<string, "active" | "suspended" | "deleted">>({});
const grants = ref<Record<string, PlatformAccessGrantInput[]>>({});
type Confirmation =
  | { kind: "session_revoke"; resourceID: string; targetLabel: string; changeSummary: string }
  | { kind: "access_update"; resourceID: string; targetLabel: string; changeSummary: string; expectedRevision: number; status: "active" | "suspended" | "deleted"; grants: PlatformAccessGrantInput[] };
const confirmation = ref<Confirmation>();
const confirmationDialog = ref<HTMLDialogElement>();
const restoredOperator = ref("");
const submittingResourceID = ref("");

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

function clearPending() {
  pending.value = undefined;
  clearPendingPlatformOperation();
}

function handleWrite(result: PlatformOperationWriteResult, command: PendingPlatformOperation, reconciling = false) {
  if (result.state === "succeeded") {
    clearPending();
    notice.value = "操作已完成并写入审计记录。";
    void load();
  } else if (result.state === "unknown" || (reconciling && (result.state === "unavailable" || result.state === "signed_out" || result.state === "denied" || result.state === "not_found"))) {
    pending.value = command;
    const saved = writePendingPlatformOperation(command);
    notice.value = saved ? (result.state === "unknown" ? "结果尚未确认，已保留原请求；请勿重复提交。" : "暂时无法核对结果，原请求仍已保留；请勿重复提交。") : "结果未确认，且浏览器无法保存原请求；请勿刷新或重复提交，并联系管理员。";
  } else {
    clearPending();
    notice.value = result.state === "conflict" ? "数据有变化，请刷新后重试。" : result.state === "denied" ? "操作被拒绝，请联系管理员确认权限。" : result.state === "not_found" ? "目标不存在或已不可操作，请刷新后核对。" : result.state === "invalid" ? "操作内容无效，请检查后重试。" : "操作没有完成，请稍后刷新页面重试。";
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

function hasPending(resourceID: string) {
  return pending.value?.resource_id === resourceID;
}

function isResourceBusy(resourceID: string) {
  return Boolean(pending.value) || submittingResourceID.value !== "";
}

function isCurrentResourceBusy(resourceID: string) {
  return hasPending(resourceID) || submittingResourceID.value === resourceID;
}

function busyLabel(resourceID: string, idleLabel: string) {
  if (isCurrentResourceBusy(resourceID)) return "等待结果";
  return isResourceBusy(resourceID) ? "已有操作待确认" : idleLabel;
}

function statusLabel(value: "active" | "suspended" | "deleted") {
  return value === "active" ? "正常" : value === "suspended" ? "已停用" : "已删除";
}

function grantLabel(grant: PlatformAccessGrantInput) {
  const scope = grant.scope.kind === "platform" ? "平台" : grant.scope.kind === "product" ? `产品 ${grant.scope.product_code ?? "未填写"}` : `资源 ${grant.scope.product_code ?? "未填写"}/${grant.scope.resource_type ?? "未填写"}/${grant.scope.resource_id ?? "未填写"}`;
  return `${grant.role_code}（${scope}）`;
}

function openConfirmation(value: Confirmation) {
  confirmation.value = value;
  void nextTick(() => {
    if (confirmationDialog.value && !confirmationDialog.value.open) confirmationDialog.value.showModal();
  });
}

function requestRevoke(session: PlatformOperationsSnapshot["sessions"][number]) {
  if (isResourceBusy(session.id)) return;
  openConfirmation({ kind: "session_revoke", resourceID: session.id, targetLabel: `${personLabel(session)} · ${personEmail(session)}`, changeSummary: `撤销${sessionKindLabel(session.kind)}登录；当前会话到期时间 ${localDateTime(session.expires_at)}` });
}

function requestAccessSave(account: PlatformOperationsSnapshot["accounts"][number]) {
  if (isResourceBusy(account.id)) return;
  const nextStatus = statuses.value[account.id];
  const nextGrants = grants.value[account.id].map((grant) => ({ role_code: grant.role_code, scope: { ...grant.scope } }));
  const changes: string[] = [];
  if (nextStatus !== account.status) changes.push(`账户状态：${statusLabel(account.status)} → ${statusLabel(nextStatus)}`);
  if (JSON.stringify(nextGrants) !== JSON.stringify(account.grants)) changes.push(`授权集合：${account.grants.length ? account.grants.map(grantLabel).join("、") : "无"} → ${nextGrants.length ? nextGrants.map(grantLabel).join("、") : "无"}`);
  if (!changes.length) {
    notice.value = "没有需要保存的访问设置变更。";
    return;
  }
  openConfirmation({ kind: "access_update", resourceID: account.id, targetLabel: `${personLabel(account)} · ${personEmail(account)}`, changeSummary: changes.join("；"), expectedRevision: account.authorization_revision, status: nextStatus, grants: nextGrants });
}

function cancelConfirm() {
  if (confirmationDialog.value?.open) confirmationDialog.value.close();
  confirmation.value = undefined;
}

async function confirmOperation() {
  const action = confirmation.value;
  const operatorID = props.operatorID;
  if (!action || !operatorID || isResourceBusy(action.resourceID)) {
    notice.value = "当前登录身份尚未确认，请刷新后重试。";
    return;
  }
  if (confirmationDialog.value?.open) confirmationDialog.value.close();
  confirmation.value = undefined;
  submittingResourceID.value = action.resourceID;
  const operation = action.kind;
  const key = idempotencyKey(operation === "session_revoke" ? "revoke" : "access");
  const command: PendingPlatformOperation = { version: 1, operator_id: operatorID, operation, idempotency_key: key, resource_id: action.resourceID };
  pending.value = command;
  const saved = writePendingPlatformOperation(command);
  if (!saved) {
    pending.value = undefined;
    submittingResourceID.value = "";
    notice.value = "浏览器无法安全保存原请求，操作未提交。请允许会话存储后重试。";
    return;
  }
  notice.value = `正在提交：${action.targetLabel} · ${action.changeSummary}`;
  const result = action.kind === "session_revoke"
    ? await revokePlatformSession(action.resourceID, key)
    : await updatePlatformAccess(action.resourceID, { expected_revision: action.expectedRevision, status: action.status, grants: action.grants }, key);
  handleWrite(result, command, false);
  submittingResourceID.value = "";
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
  const command = pending.value;
  handleWrite(await resolvePlatformOperation(command.operation, command.idempotency_key), command, true);
}

async function restorePending() {
  const operatorID = props.operatorID;
  if (!operatorID) {
    pending.value = undefined;
    submittingResourceID.value = "";
    notice.value = "";
    cancelConfirm();
    restoredOperator.value = "";
    return;
  }
  if (restoredOperator.value === operatorID) return;
  pending.value = undefined;
  submittingResourceID.value = "";
  notice.value = "";
  cancelConfirm();
  restoredOperator.value = operatorID;
  const restored = readPendingPlatformOperation(operatorID);
  if (!restored) return;
  pending.value = restored;
  notice.value = "正在核对之前未确认的平台操作。";
  await resolveUnknown();
}

const mailTotal = computed(() => operations.value ? Object.values(operations.value.mail).reduce((sum, value) => sum + value, 0) : 0);
const canWrite = computed(() => operations.value?.access_context.permissions.includes("platform.operations.write") ?? false);
watch(() => props.operatorID, () => { void restorePending(); });
onMounted(() => { if (props.authState !== "signed_out") void load(); void restorePending(); });
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
    <dialog
      ref="confirmationDialog"
      class="m-auto w-[min(92vw,42rem)] rounded-lg border border-destructive/30 bg-background p-5 shadow-xl backdrop:bg-foreground/30"
      aria-labelledby="platform-confirm-heading"
      @close="confirmation = undefined"
      @click.self="cancelConfirm"
    >
      <div v-if="confirmation">
        <p class="eyebrow">高风险操作确认</p>
        <h2 id="platform-confirm-heading" class="mt-1 text-lg font-semibold">{{ confirmation.kind === "session_revoke" ? "确认撤销登录会话" : "确认更新访问设置" }}</h2>
        <dl class="mt-3 grid gap-2 text-sm"><div><dt class="text-muted-foreground">目标</dt><dd class="break-all font-medium">{{ confirmation.targetLabel }}</dd></div><div><dt class="text-muted-foreground">将要发生</dt><dd class="break-words font-medium">{{ confirmation.changeSummary }}</dd></div></dl>
        <p v-if="confirmation.kind === 'access_update' && confirmation.status === 'deleted'" class="mt-2 text-sm text-muted-foreground">账户数据不会被物理删除，但该账户将无法登录，授权检查也不再通过。</p>
        <p class="mt-2 text-sm text-muted-foreground">取消不会产生任何写入。</p>
        <div class="mt-3 flex flex-wrap gap-2"><button type="button" class="bg-destructive! border-destructive!" @click="confirmOperation">{{ confirmation.kind === "session_revoke" ? "确认撤销会话" : "确认更新访问设置" }}</button><button type="button" class="secondary-action" autofocus @click="cancelConfirm">取消</button></div>
      </div>
    </dialog>
    <div v-if="state === 'loading'" class="operation-state" aria-busy="true">正在读取运营状态…</div>
    <div v-else-if="state === 'denied'" class="operation-state">当前登录账户没有平台运营权限，请联系管理员。</div>
    <div v-else-if="state === 'unavailable'" class="operation-state">运营数据暂不可用。<button type="button" @click="load">重试</button></div>

    <template v-else-if="operations">
      <div class="operation-summary-grid">
        <article><span>账户</span><strong>{{ operations.accounts.length }}</strong></article>
        <article><span>登录会话</span><strong>{{ operations.sessions.length }}</strong></article>
        <article><span>邮件事件</span><strong>{{ mailTotal }}</strong></article>
        <article><span>收件箱</span><strong>{{ operations.inbox_items.length }}</strong></article>
        <article><span>数据库</span><StatusBadge :status="healthBadgeStatus(operations.dependencies.postgres)">{{ healthLabel(operations.dependencies.postgres) }}</StatusBadge></article>
        <article><span>缓存</span><StatusBadge :status="healthBadgeStatus(operations.dependencies.redis)">{{ healthLabel(operations.dependencies.redis) }}</StatusBadge></article>
      </div>

      <section class="operation-panel" aria-labelledby="accounts-heading">
        <h2 id="accounts-heading">账户、角色与权限</h2>
        <div class="operation-list">
          <template v-for="account in operations.accounts" :key="account.id">
            <article class="operation-row">
              <div class="account-access-editor">
                <strong>{{ personLabel(account) }}</strong>
                <p class="break-all">{{ personEmail(account) }} · 授权版本 {{ account.authorization_revision }} · {{ account.email_verified ? "邮箱已验证" : "邮箱未验证" }}</p>
                <div v-for="(grant, index) in grants[account.id]" :key="index" class="grant-editor">
                  <label>角色代码<input v-model="grant.role_code" :disabled="!canWrite || isResourceBusy(account.id)" pattern="[a-z][a-z0-9-]+" /></label>
                  <label>权限范围<select v-model="grant.scope.kind" :disabled="!canWrite || isResourceBusy(account.id)" @change="normalizeScope(grant)"><option value="platform">平台</option><option value="product">产品</option><option value="resource">资源</option></select></label>
                  <label v-if="grant.scope.kind !== 'platform'">产品代码<input v-model="grant.scope.product_code" :disabled="!canWrite || isResourceBusy(account.id)" /></label>
                  <label v-if="grant.scope.kind === 'resource'">资源类型<input v-model="grant.scope.resource_type" :disabled="!canWrite || isResourceBusy(account.id)" /></label>
                  <label v-if="grant.scope.kind === 'resource'">资源 ID<input v-model="grant.scope.resource_id" :disabled="!canWrite || isResourceBusy(account.id)" /></label>
                  <button v-if="canWrite" type="button" class="secondary-action" :disabled="isResourceBusy(account.id)" @click="removeGrant(account.id, index)">删除授权</button>
                </div>
                <button v-if="canWrite" type="button" class="secondary-action" :disabled="isResourceBusy(account.id)" @click="addGrant(account.id)">新增角色 / 权限</button>
              </div>
              <div class="operation-actions"><label>账户状态<select v-model="statuses[account.id]" :disabled="!canWrite || isResourceBusy(account.id)"><option value="active">正常</option><option value="suspended">已停用</option><option value="deleted">已删除</option></select></label><button v-if="canWrite" type="button" :disabled="isResourceBusy(account.id)" @click="requestAccessSave(account)">{{ busyLabel(account.id, "保存访问设置") }}</button><span v-else>只读权限</span></div>
            </article>
          </template>
        </div>
      </section>

      <section class="operation-panel" aria-labelledby="sessions-heading"><h2 id="sessions-heading">登录会话</h2><div class="operation-list"><article v-for="session in operations.sessions" :key="session.id" class="operation-row"><div><strong>{{ personLabel(session) }}</strong><p class="break-all">{{ personEmail(session) }} · {{ sessionKindLabel(session.kind) }} · 到期 {{ localDateTime(session.expires_at) }}</p></div><button v-if="canWrite" type="button" :disabled="Boolean(session.revoked_at) || isResourceBusy(session.id)" @click="requestRevoke(session)">{{ session.revoked_at ? "已撤销" : busyLabel(session.id, "撤销登录") }}</button><span v-else>只读权限</span></article></div></section>

      <div class="operation-two-column">
        <section class="operation-panel"><h2>邮件基础设施</h2><dl class="mail-grid"><template v-for="(value, key) in operations.mail" :key="key"><dt>{{ mailLabel(key) }}</dt><dd>{{ value }}</dd></template></dl></section>
        <section class="operation-panel"><h2>运营收件箱</h2><article v-for="item in operations.inbox_items" :key="item.id" class="compact-row"><strong>{{ item.source_product_code }} / {{ item.source_resource_type }}</strong><span>{{ item.source_resource_id }} · {{ inboxStatusLabel(item.status) }} · v{{ item.version }}</span></article><p v-if="!operations.inbox_items.length">暂无引用项</p></section>
      </div>
      <section class="operation-panel"><h2>授权审计</h2><article v-for="event in operations.audit" :key="event.request_id + event.created_at" class="compact-row"><strong>{{ decisionLabel(event.decision) }} · {{ event.permission_code }}</strong><span class="break-all">{{ personLabel(event) }} · {{ personEmail(event) }} · {{ localDateTime(event.created_at) }} · 目标 {{ targetLabel(event) }}</span><span>原因：{{ reasonLabel(event.reason_code) }}<template v-if="unknownReason(event.reason_code)">（{{ event.reason_code }}）</template> · {{ event.request_id }}</span></article><p v-if="!operations.audit.length">暂无审计事件</p></section>
    </template>
  </section>
</template>
