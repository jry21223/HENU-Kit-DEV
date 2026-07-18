<template>
  <AdminShellV2 :title="meta.title" environment="runtime">
    <div class="space-y-6">
      <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div><p class="text-xs font-semibold uppercase tracking-[.16em] text-primary">{{ domain }}</p><h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ meta.title }}</h1><p class="mt-2 text-sm text-muted-foreground">{{ meta.description }}</p></div>
        <Button variant="outline" :disabled="loading" @click="load"><RefreshCw :class="['size-4', { 'animate-spin': loading }]" />刷新</Button>
      </header>
      <Alert v-if="error" variant="destructive">{{ error }}</Alert>
      <Alert v-if="message" class="border-emerald-200 bg-emerald-50 text-emerald-800">{{ message }}</Alert>

      <Card v-if="domain === 'notice'">
        <CardHeader><CardTitle>校园通知审核</CardTitle><CardDescription>人工表单与 JSONL 共用不可变版本写入；审核操作使用乐观锁。</CardDescription></CardHeader>
        <CardContent><Table><TableHeader><TableRow><TableHead>通知</TableHead><TableHead>状态</TableHead><TableHead>版本</TableHead><TableHead>发布时间</TableHead><TableHead class="w-[320px]">操作</TableHead></TableRow></TableHeader><TableBody>
          <TableRow v-for="item in noticeData.items" :key="item.id"><TableCell><strong>{{ item.title }}</strong><p class="mt-1 max-w-md truncate text-xs text-muted-foreground">{{ item.original_url }}</p></TableCell><TableCell><Badge :variant="statusBadge(item.status)">{{ statusLabel(item.status) }}</Badge></TableCell><TableCell>v{{ item.current_version }} / #{{ item.version }}</TableCell><TableCell>{{ formatTime(item.original_published_at) }}</TableCell><TableCell><div class="flex flex-wrap gap-2"><Button size="sm" variant="outline" @click="showNoticeVersions(item)">版本对比</Button><Button size="sm" :disabled="item.status === 'approved'" @click="reviewNotice(item, 'approve')">通过</Button><Input v-model="reviewReasons[item.id]" class="h-8 min-w-32 flex-1" placeholder="驳回原因" /><Button size="sm" variant="destructive" @click="reviewNotice(item, 'reject')">驳回</Button></div></TableCell></TableRow>
          <TableRow v-if="!noticeData.items.length"><TableCell colspan="5" class="py-10 text-center text-muted-foreground">暂无通知</TableCell></TableRow>
        </TableBody></Table></CardContent>
      </Card>
      <Card v-if="domain === 'notice' && noticeVersions.length"><CardHeader><CardTitle>{{ versionNoticeTitle }} · 不可变版本对比</CardTitle><CardDescription>左侧为最新版本，右侧为上一版本；哈希和附件对象键均保留以便追溯。</CardDescription></CardHeader><CardContent><div class="grid gap-4 lg:grid-cols-2"><article v-for="item in noticeVersions.slice(0,2)" :key="item.id" class="rounded-lg border p-4"><div class="mb-3 flex items-center justify-between"><Badge>v{{ item.version }}</Badge><code class="text-[10px] text-muted-foreground">{{ item.content_hash.slice(0,16) }}…</code></div><h3 class="font-semibold">{{ item.title }}</h3><pre class="mt-3 max-h-72 overflow-auto whitespace-pre-wrap text-xs leading-6">{{ item.body }}</pre><p class="mt-3 text-xs text-muted-foreground">附件记录：{{ attachmentCount(item.object_keys) }} 个</p></article></div></CardContent></Card>

      <div v-if="domain === 'notice'" class="grid gap-4 xl:grid-cols-2">
        <Card><CardHeader><CardTitle>单条录入</CardTitle><CardDescription>表单与 JSONL 使用相同的 Upsert 和不可变版本规则。</CardDescription></CardHeader><CardContent><form class="grid gap-3" @submit.prevent="createNotice"><div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-1.5 text-xs font-medium">来源 UUID<Input v-model="noticeForm.source_id" required /></label><label class="grid gap-1.5 text-xs font-medium">外部 ID<Input v-model="noticeForm.external_id" required /></label></div><label class="grid gap-1.5 text-xs font-medium">标题<Input v-model="noticeForm.title" required /></label><label class="grid gap-1.5 text-xs font-medium">正文<Textarea v-model="noticeForm.body" class="min-h-32" required /></label><div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-1.5 text-xs font-medium">原发布时间<Input v-model="noticeForm.published_at" type="datetime-local" required /></label><label class="grid gap-1.5 text-xs font-medium">原链接<Input v-model="noticeForm.original_url" type="url" required /></label></div><div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-1.5 text-xs font-medium">受众范围<select v-model="noticeForm.audience_scope" class="h-10 rounded-md border border-input bg-background px-3 text-sm"><option value="all_verified_users">全部已验证用户</option><option value="school">指定学校</option><option value="major">指定专业</option></select></label><label v-if="noticeForm.audience_scope !== 'all_verified_users'" class="grid gap-1.5 text-xs font-medium">受众 UUID<Input v-model="noticeForm.audience_id" required placeholder="UUID" /></label></div><label class="grid gap-1.5 text-xs font-medium">附件（PDF / JPG / PNG / WebP，单个不超过 20 MB）<input class="block w-full rounded-md border border-input bg-background p-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-secondary file:px-3 file:py-1.5" type="file" multiple accept="application/pdf,image/jpeg,image/png,image/webp" :disabled="uploadingAttachments" @change="uploadNoticeAttachments" /></label><div v-if="noticeAttachments.length" class="space-y-2"><div v-for="(item,index) in noticeAttachments" :key="item.object_key" class="flex items-center justify-between rounded-md border p-2 text-xs"><span class="truncate">{{ item.file_name }} · {{ formatBytes(item.size_bytes) }}</span><Button type="button" size="sm" variant="ghost" @click="noticeAttachments.splice(index,1)">移除</Button></div></div><Button type="submit" :disabled="loading || uploadingAttachments">{{ uploadingAttachments ? '正在上传附件…' : '保存并送审' }}</Button></form></CardContent></Card>
        <Card><CardHeader><CardTitle>JSONL 批量导入</CardTitle><CardDescription>UTF-8 `campus-notice-import/1.0`；每任务最多 1,000 条或 10 MB。</CardDescription></CardHeader><CardContent><div class="grid gap-4"><input class="block w-full rounded-md border border-input bg-background p-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-secondary file:px-3 file:py-1.5 file:text-sm file:font-medium" type="file" accept=".jsonl,application/x-ndjson" @change="selectJSONL" /><Button :disabled="loading || !jsonlContent" @click="importJSONL">开始导入</Button><div v-if="importSummary" class="grid grid-cols-5 gap-2 rounded-lg border bg-muted/30 p-3 text-center"><div v-for="item in importSummary" :key="item.label"><strong class="block text-lg">{{ item.value }}</strong><span class="text-[11px] text-muted-foreground">{{ item.label }}</span></div></div></div></CardContent></Card>
      </div>

      <template v-else-if="domain === 'mail'">
        <Card><CardHeader><CardTitle>邮件投递</CardTitle><CardDescription>SMTP accepted 与 delivered 分开展示，不把接受冒充送达。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>队列</TableHead><TableHead>模板</TableHead><TableHead>状态</TableHead><TableHead>尝试</TableHead><TableHead>排队时间</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in mailData.deliveries" :key="item.id"><TableCell>{{ item.category }}</TableCell><TableCell>{{ item.template_code }}</TableCell><TableCell><Badge :variant="statusBadge(item.status)">{{ item.status }}</Badge></TableCell><TableCell>{{ item.attempt_count }}</TableCell><TableCell>{{ formatTime(item.queued_at) }}</TableCell><TableCell><Button v-if="item.status === 'failed'" size="sm" variant="outline" @click="retryMail(item)">安全重试</Button><span v-else class="text-xs text-muted-foreground">—</span></TableCell></TableRow><TableRow v-if="!mailData.deliveries.length"><TableCell colspan="6" class="py-10 text-center text-muted-foreground">当前没有投递记录</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card><CardHeader><CardTitle>投递尝试证据</CardTitle><CardDescription>每次 Worker 尝试独立留痕；accepted 不等于 delivered。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>投递 ID</TableHead><TableHead>次数</TableHead><TableHead>状态</TableHead><TableHead>错误码</TableHead><TableHead>开始</TableHead><TableHead>结束</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in mailData.attempts" :key="item.id"><TableCell><code class="text-xs">{{ item.delivery_id.slice(0,8) }}</code></TableCell><TableCell>#{{ item.attempt }}</TableCell><TableCell><Badge :variant="statusBadge(item.status)">{{ item.status }}</Badge></TableCell><TableCell>{{ item.error_code || '—' }}</TableCell><TableCell>{{ formatTime(item.started_at) }}</TableCell><TableCell>{{ formatTime(item.ended_at) }}</TableCell></TableRow><TableRow v-if="!mailData.attempts.length"><TableCell colspan="6" class="py-8 text-center text-muted-foreground">暂无投递尝试</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card><CardHeader><CardTitle>死信队列</CardTitle><CardDescription>重放会重置投递尝试并留下审计记录。</CardDescription></CardHeader><CardContent><div v-if="mailData.dead_letters.length" class="space-y-2"><div v-for="item in mailData.dead_letters" :key="item.id" class="flex items-center justify-between rounded-lg border p-3"><div><span class="text-sm">{{ item.reason_code }}</span><Badge class="ml-2" variant="destructive">{{ item.status }}</Badge></div><Button size="sm" @click="replayDeadLetter(item)">重放</Button></div></div><p v-else class="py-8 text-center text-sm text-muted-foreground">当前没有死信</p></CardContent></Card>
        <Card><CardHeader><CardTitle>抑制名单</CardTitle><CardDescription>邮箱只用于计算 SHA-256，列表不返回明文。</CardDescription></CardHeader><CardContent><form class="grid gap-3 sm:grid-cols-[1fr_1fr_auto]" @submit.prevent="createSuppression"><Input v-model="suppressionForm.recipient" type="email" placeholder="recipient@example.com" required /><Input v-model="suppressionForm.reason_code" placeholder="unsubscribe" required /><Button type="submit">加入抑制</Button></form><div class="mt-4 space-y-2"><div v-for="item in mailData.suppressions" :key="item.id" class="flex items-center justify-between rounded-md border p-3 text-sm"><code>{{ item.recipient_hash.slice(0, 16) }}…</code><span>{{ item.reason_code }}</span><Button v-if="!item.expires_at || new Date(item.expires_at).getTime() > Date.now()" size="sm" variant="outline" @click="releaseSuppression(item)">解除</Button><Badge v-else variant="secondary">已解除</Badge></div><p v-if="!mailData.suppressions.length" class="py-6 text-center text-sm text-muted-foreground">当前没有抑制记录</p></div></CardContent></Card>
      </template>

      <template v-else-if="domain === 'feedback'">
        <Card><CardHeader><CardTitle>统一反馈待办</CardTitle><CardDescription>紧急 24 小时、普通 72 小时；超过 due_at 且未解决才标记超时。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>反馈</TableHead><TableHead>等级</TableHead><TableHead>状态</TableHead><TableHead>到期</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in feedbackData.platform" :key="item.id"><TableCell><strong>{{ item.summary }}</strong><p class="mt-1 max-w-lg line-clamp-2 text-xs text-muted-foreground">{{ item.content }}</p></TableCell><TableCell><Badge :variant="item.urgency === 'urgent' ? 'destructive' : 'warning'">{{ item.urgency === "urgent" ? "24h" : "72h" }}</Badge></TableCell><TableCell>{{ statusLabel(item.status) }}</TableCell><TableCell :class="isOverdue(item) ? 'font-medium text-destructive' : ''">{{ formatTime(item.due_at) }}</TableCell><TableCell><div class="flex gap-2"><Button size="sm" variant="outline" @click="updateFeedback(item, 'in_progress')">处理中</Button><Button size="sm" :disabled="item.status === 'resolved'" @click="updateFeedback(item, 'resolved')">解决</Button></div></TableCell></TableRow><TableRow v-if="!feedbackData.platform.length"><TableCell colspan="5" class="py-10 text-center text-muted-foreground">暂无平台反馈</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card><CardHeader><CardTitle>题目反馈验证</CardTitle><CardDescription>提交时快照、PostgreSQL 题目与线上 API 三方全部验证后才可解决。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>题目 / 原因</TableHead><TableHead>JSON</TableHead><TableHead>PostgreSQL</TableHead><TableHead>API</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in feedbackData.quiz" :key="item.id"><TableCell><code class="text-xs">{{ item.targetId }}</code><p class="mt-1 text-sm">{{ item.reason }}</p></TableCell><TableCell><Badge :variant="item.jsonVerified ? 'success' : 'warning'">{{ item.jsonVerified ? '通过' : '待验证' }}</Badge></TableCell><TableCell><Badge :variant="item.postgresVerified ? 'success' : 'warning'">{{ item.postgresVerified ? '通过' : '待验证' }}</Badge></TableCell><TableCell><Badge :variant="item.apiVerified ? 'success' : 'warning'">{{ item.apiVerified ? '通过' : '待验证' }}</Badge></TableCell><TableCell><div class="flex gap-2"><Button size="sm" variant="outline" @click="verifyQuizFeedback(item)">执行三方验证</Button><Button size="sm" :disabled="!quizVerified(item) || item.status !== 'pending'" @click="resolveQuizFeedback(item)">解决</Button></div></TableCell></TableRow><TableRow v-if="!feedbackData.quiz.length"><TableCell colspan="5" class="py-10 text-center text-muted-foreground">暂无题目反馈</TableCell></TableRow></TableBody></Table></CardContent></Card>
      </template>

      <template v-else-if="domain === 'food'">
        <Card><CardHeader><CardTitle>投稿审核</CardTitle><CardDescription>通过后在同一事务创建榜单条目和首轮校准。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>名称</TableHead><TableHead>位置</TableHead><TableHead>状态</TableHead><TableHead>建议档位</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in foodData.submissions" :key="item.id"><TableCell><strong>{{ item.name }}</strong><p class="mt-1 text-xs text-muted-foreground">{{ item.reason }}</p></TableCell><TableCell>{{ item.location }}</TableCell><TableCell><Badge :variant="statusBadge(item.status)">{{ statusLabel(item.status) }}</Badge></TableCell><TableCell>{{ tierName(item.suggested_tier_id) }}</TableCell><TableCell><Button size="sm" :disabled="item.status === 'approved'" @click="approveFood(item)">审核通过</Button></TableCell></TableRow><TableRow v-if="!foodData.submissions.length"><TableCell colspan="5" class="py-10 text-center text-muted-foreground">暂无投稿</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card><CardHeader><CardTitle>校准候选</CardTitle><CardDescription>有效票少于 10 人不决策；70% 形成升降档或稳定结论，异常票不计入分母。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>条目</TableHead><TableHead>参与者</TableHead><TableHead>被低估</TableHead><TableHead>差不多</TableHead><TableHead>被高估</TableHead><TableHead>结论与操作</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in foodData.candidates" :key="item.round_id"><TableCell>{{ item.name }}</TableCell><TableCell>{{ item.participants }}</TableCell><TableCell>{{ percent(item.underrated_rate) }}</TableCell><TableCell>{{ percent(item.about_right_rate) }}</TableCell><TableCell>{{ percent(item.overrated_rate) }}</TableCell><TableCell><div class="flex items-center gap-2"><Badge :variant="candidateBadge(item.decision)">{{ candidateLabel(item.decision) }}</Badge><Button v-if="item.decision === 'promote_candidate'" size="sm" @click="adjustFood(item, 'promote')">确认升档</Button><Button v-if="item.decision === 'demote_candidate'" size="sm" variant="destructive" @click="adjustFood(item, 'demote')">确认降档</Button></div></TableCell></TableRow><TableRow v-if="!foodData.candidates.length"><TableCell colspan="6" class="py-10 text-center text-muted-foreground">暂无开放校准轮次</TableCell></TableRow></TableBody></Table></CardContent></Card>
        <Card><CardHeader><CardTitle>异常票处置</CardTitle><CardDescription>未处理的阻断异常会阻止调档。</CardDescription></CardHeader><CardContent><div class="space-y-2"><div v-for="item in foodData.anomalies" :key="item.id" class="flex items-center justify-between rounded-md border p-3"><div><strong class="text-sm">{{ item.rule_code }}</strong><p class="text-xs text-muted-foreground">{{ item.severity }} · {{ item.blocking ? '阻断' : '非阻断' }}</p></div><div class="flex items-center gap-2"><Badge :variant="item.status === 'open' ? 'destructive' : 'success'">{{ item.status }}</Badge><Button v-if="item.status === 'open'" size="sm" variant="outline" @click="resolveFoodAnomaly(item)">标记已处理</Button></div></div><p v-if="!foodData.anomalies.length" class="py-6 text-center text-sm text-muted-foreground">暂无异常票</p></div></CardContent></Card>
        <Card><CardHeader><CardTitle>调档历史</CardTitle><CardDescription>每次只能升降一档，保留原轮次和操作人。</CardDescription></CardHeader><CardContent><Table><TableHeader><TableRow><TableHead>条目</TableHead><TableHead>方向</TableHead><TableHead>原档位</TableHead><TableHead>新档位</TableHead><TableHead>时间</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in foodData.history" :key="item.id"><TableCell><code class="text-xs">{{ item.entry_id }}</code></TableCell><TableCell>{{ item.direction === 'promote' ? '升档' : '降档' }}</TableCell><TableCell>{{ tierName(item.from_tier_id) }}</TableCell><TableCell>{{ tierName(item.to_tier_id) }}</TableCell><TableCell>{{ formatTime(item.adjusted_at) }}</TableCell></TableRow><TableRow v-if="!foodData.history.length"><TableCell colspan="5" class="py-8 text-center text-muted-foreground">暂无调档历史</TableCell></TableRow></TableBody></Table></CardContent></Card>
      </template>

      <Card v-else-if="domain === 'system'">
        <CardHeader><CardTitle>部署单元状态</CardTitle><CardDescription>版本、Commit SHA、部署时间、Readiness、Outbox 和 Worker 异常。</CardDescription></CardHeader><CardContent><div class="mb-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-5"><div class="rounded-lg border bg-muted/30 p-3"><span class="text-xs text-muted-foreground">API P95</span><strong class="mt-1 block text-xl">{{ systemData.runtime.http.p95_ms.toFixed(1) }} ms</strong><small>{{ systemData.runtime.http.errors_5xx }} 次 5xx / {{ systemData.runtime.http.requests }} 请求</small></div><div class="rounded-lg border bg-muted/30 p-3"><span class="text-xs text-muted-foreground">PostgreSQL / Redis</span><strong class="mt-1 block text-xl">{{ systemData.runtime.postgresql }} / {{ systemData.runtime.redis }}</strong></div><div class="rounded-lg border bg-muted/30 p-3"><span class="text-xs text-muted-foreground">邮件队列 / DLQ</span><strong class="mt-1 block text-xl">{{ systemData.runtime.mail_pending }} / {{ systemData.runtime.mail_dlq }}</strong></div><div class="rounded-lg border bg-muted/30 p-3"><span class="text-xs text-muted-foreground">Outbox 待处理 / 失败</span><strong class="mt-1 block text-xl">{{ systemData.runtime.outbox_pending }} / {{ systemData.runtime.outbox_failed }}</strong></div><div class="rounded-lg border bg-muted/30 p-3"><span class="text-xs text-muted-foreground">最新 Migration</span><strong class="mt-1 block break-all text-sm">{{ systemData.runtime.latest_migration }}</strong></div></div><Table><TableHeader><TableRow><TableHead>服务</TableHead><TableHead>状态</TableHead><TableHead>版本 / Commit</TableHead><TableHead>最后就绪</TableHead><TableHead>Outbox</TableHead><TableHead>Worker 异常</TableHead></TableRow></TableHeader><TableBody><TableRow v-for="item in systemData.items" :key="item.id"><TableCell>{{ item.service_id }}</TableCell><TableCell><Badge :variant="statusBadge(item.status)">{{ item.status }}</Badge></TableCell><TableCell>{{ item.service_version || "—" }} / {{ item.commit_sha || "—" }}</TableCell><TableCell>{{ formatTime(item.last_ready_at) }}</TableCell><TableCell>{{ item.outbox_pending }}</TableCell><TableCell>{{ item.worker_anomalies }}</TableCell></TableRow><TableRow v-if="!systemData.items.length"><TableCell colspan="6" class="py-10 text-center text-muted-foreground">尚无独立服务心跳；当前 API、PostgreSQL、Redis 状态请查看总览系统卡。</TableCell></TableRow></TableBody></Table></CardContent>
      </Card>
    </div>
  </AdminShellV2>
</template>

<script setup lang="ts">
import { RefreshCw } from "@lucide/vue";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import AdminShellV2 from "@/components/AdminShellV2.vue";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { adminRequest } from "@/lib/admin-api";

type Domain = "notice" | "mail" | "feedback" | "food" | "system";
type Base = { id:string; created_at:string; updated_at:string };
type Notice = Base & { title:string; original_url:string; original_published_at?:string; status:string; current_version:number; version:number };
type NoticeVersion = Base & { version:number; title:string; body:string; content_hash:string; object_keys?:string };
type NoticeAttachment = { file_name:string; object_key:string; sha256:string; content_type:string; size_bytes:number };
type MailDelivery = Base & { category:string; template_code:string; status:string; attempt_count:number; queued_at:string; version:number };
type MailAttempt = Base & { delivery_id:string; attempt:number; status:string; error_code?:string; started_at:string; ended_at?:string };
type DeadLetter = Base & { delivery_id:string; reason_code:string; status:string };
type MailSuppression = Base & { recipient_hash:string; reason_code:string; expires_at?:string; version:number };
type Feedback = Base & { summary:string; content:string; urgency:"urgent"|"normal"; status:string; due_at:string; version:number };
type QuizFeedback = Base & { targetId:string; reason:string; status:string; jsonVerified:boolean; postgresVerified:boolean; apiVerified:boolean; version:number };
type FoodTier = Base & { name:string; sort_order:number };
type FoodSubmission = Base & { name:string; location:string; reason:string; status:string; suggested_tier_id:string; version:number };
type FoodCandidate = { entry_id:string; entry_version:number; round_id:string; name:string; participants:number; underrated_rate:number; about_right_rate:number; overrated_rate:number; decision:string };
type FoodAnomaly = Base & { rule_code:string; severity:string; blocking:boolean; status:string; version:number };
type FoodHistory = Base & { entry_id:string; from_tier_id:string; to_tier_id:string; direction:string; adjusted_at:string };
type Heartbeat = Base & { service_id:string; status:string; service_version:string; commit_sha:string; last_ready_at:string; outbox_pending:number; worker_anomalies:number };
type SystemRuntime = { postgresql:string; redis:string; mail_pending:number; mail_dlq:number; outbox_pending:number; outbox_failed:number; latest_migration:string; http:{requests:number;errors_5xx:number;p95_ms:number} };

const route = useRoute();
const domain = computed(() => String(route.meta.domain) as Domain);
const meta = computed(() => ({ title:String(route.meta.title ?? "业务运营"), description:String(route.meta.description ?? "") }));
const loading = ref(false); const error = ref(""); const message = ref("");
const reviewReasons = reactive<Record<string,string>>({});
const noticeForm = reactive({ source_id:crypto.randomUUID(), external_id:"", title:"", body:"", published_at:"", original_url:"", importance:"normal", audience_scope:"all_verified_users", audience_id:"" });
const noticeAttachments = reactive<NoticeAttachment[]>([]);
const uploadingAttachments = ref(false);
const jsonlContent = ref("");
const importSummary = ref<Array<{label:string;value:number}> | null>(null);
const noticeData = reactive<{items:Notice[];total:number}>({items:[],total:0});
const noticeVersions = ref<NoticeVersion[]>([]); const versionNoticeTitle = ref("");
const mailData = reactive<{deliveries:MailDelivery[];attempts:MailAttempt[];dead_letters:DeadLetter[];suppressions:MailSuppression[];total:number}>({deliveries:[],attempts:[],dead_letters:[],suppressions:[],total:0});
const suppressionForm = reactive({recipient:"",reason_code:"unsubscribe"});
const feedbackData = reactive<{platform:Feedback[];quiz:QuizFeedback[]}>({platform:[],quiz:[]});
const foodData = reactive<{tiers:FoodTier[];submissions:FoodSubmission[];candidates:FoodCandidate[];anomalies:FoodAnomaly[];history:FoodHistory[]}>({tiers:[],submissions:[],candidates:[],anomalies:[],history:[]});
const systemData = reactive<{items:Heartbeat[];runtime:SystemRuntime}>({items:[],runtime:{postgresql:"unknown",redis:"unknown",mail_pending:0,mail_dlq:0,outbox_pending:0,outbox_failed:0,latest_migration:"unknown",http:{requests:0,errors_5xx:0,p95_ms:0}}});

onMounted(load);
watch(domain, load);
async function load() {
  loading.value=true; error.value="";
  try {
    if(domain.value==="notice") Object.assign(noticeData,(await adminRequest<typeof noticeData>("/admin/notices")).data);
    if(domain.value==="mail") Object.assign(mailData,(await adminRequest<typeof mailData>("/admin/mail-operations")).data);
    if(domain.value==="feedback") Object.assign(feedbackData,(await adminRequest<typeof feedbackData>("/admin/feedback-operations")).data);
    if(domain.value==="food") Object.assign(foodData,(await adminRequest<typeof foodData>("/admin/food-operations")).data);
    if(domain.value==="system") Object.assign(systemData,(await adminRequest<typeof systemData>("/admin/system-operations")).data);
  } catch(e) { error.value=e instanceof Error?e.message:"业务数据加载失败"; } finally { loading.value=false; }
}
async function mutate(path:string,method:string,body:unknown) { return adminRequest(path,{method,headers:{"Idempotency-Key":crypto.randomUUID()},body:JSON.stringify(body)}); }
async function reviewNotice(item:Notice,decision:"approve"|"reject") { const reason=reviewReasons[item.id]?.trim()??""; if(decision==="reject"&&!reason){error.value="请先填写驳回原因";return;} await runAction(async()=>mutate(`/admin/notices/${item.id}/reviews`,"POST",{decision,reason,expected_version:item.version}),"通知审核已更新"); }
async function showNoticeVersions(item:Notice){try{const result=await adminRequest<{title:string;versions:NoticeVersion[]}>(`/admin/notices/${item.id}/versions`);noticeVersions.value=result.data.versions;versionNoticeTitle.value=result.data.title;}catch(e){error.value=e instanceof Error?e.message:'通知版本读取失败';}}
async function updateFeedback(item:Feedback,status:string) { await runAction(async()=>mutate(`/admin/platform-feedback/${item.id}`,"PATCH",{status,expected_version:item.version}),status==="resolved"?"反馈已解决":"反馈已进入处理中"); }
async function verifyQuizFeedback(item:QuizFeedback){await runAction(async()=>mutate(`/admin/quiz-feedback/${item.id}/verifications`,"POST",{expected_version:item.version}),"三方验证结果已更新");}
async function resolveQuizFeedback(item:QuizFeedback){await runAction(async()=>mutate(`/admin/quiz-feedback/${item.id}/resolutions`,"POST",{expected_version:item.version}),"题目反馈已解决");}
function quizVerified(item:QuizFeedback){return item.jsonVerified&&item.postgresVerified&&item.apiVerified;}
async function approveFood(item:FoodSubmission) { await runAction(async()=>mutate(`/admin/food-submissions/${item.id}/approvals`,"POST",{expected_version:item.version}),"投稿已通过并创建校准轮次"); }
async function adjustFood(item:FoodCandidate,direction:"promote"|"demote") { await runAction(async()=>mutate(`/admin/food-entries/${item.entry_id}/adjustments`,"POST",{direction,expected_version:item.entry_version}),direction==="promote"?"条目已升档并进入 7 天冷却":"条目已降档并进入 7 天冷却"); }
async function resolveFoodAnomaly(item:FoodAnomaly){await runAction(async()=>mutate(`/admin/food-vote-anomalies/${item.id}/resolutions`,"POST",{expected_version:item.version,resolution:"管理员人工复核"}),"异常票已处理");}
async function retryMail(item:MailDelivery){await runAction(async()=>mutate(`/admin/mail-deliveries/${item.id}/retries`,"POST",{expected_version:item.version}),"邮件已重新入队");}
async function replayDeadLetter(item:DeadLetter){const delivery=mailData.deliveries.find(value=>value.id===item.delivery_id);if(!delivery){error.value="无法找到死信对应的投递记录";return;}await runAction(async()=>mutate(`/admin/mail-dead-letters/${item.id}/replays`,"POST",{expected_version:delivery.version}),"死信已重放");}
async function createSuppression(){await runAction(async()=>mutate("/admin/mail-suppressions","POST",suppressionForm),"抑制名单已更新");suppressionForm.recipient="";}
async function releaseSuppression(item:MailSuppression){await runAction(async()=>mutate(`/admin/mail-suppressions/${item.id}`,"PATCH",{expected_version:item.version}),"抑制已解除");}
async function runAction(action:()=>Promise<unknown>,success:string){loading.value=true;error.value="";message.value="";try{await action();message.value=success;await load();}catch(e){error.value=e instanceof Error?e.message:"操作失败";}finally{loading.value=false;}}
function formatTime(value?:string){return value?new Intl.DateTimeFormat("zh-CN",{dateStyle:"short",timeStyle:"short"}).format(new Date(value)):"—";}
function percent(value:number){return `${(value*100).toFixed(0)}%`;}
function isOverdue(item:Feedback){return item.status!=="resolved"&&new Date(item.due_at).getTime()<Date.now();}
function tierName(id:string){return foodData.tiers.find(item=>item.id===id)?.name??id;}
function statusLabel(value:string){return ({review_pending:"待审核",approved:"已通过",rejected:"已驳回",pending:"待审核",new:"新反馈",in_progress:"处理中",resolved:"已解决"} as Record<string,string>)[value]??value;}
function statusBadge(value:string){return value==="approved"||value==="resolved"||value==="ok"?"success":value==="rejected"||value==="failed"?"destructive":value==="review_pending"||value==="pending"||value==="in_progress"?"warning":"secondary";}
function candidateLabel(value:string){return ({insufficient_votes:"人数不足",promote_candidate:"升档候选",demote_candidate:"降档候选",stable:"稳定",contested:"有争议",cooldown:"冷却期",blocked_by_risk:"风险阻断"} as Record<string,string>)[value]??value;}
function candidateBadge(value:string){return value==="promote_candidate"||value==="stable"?"success":value==="demote_candidate"||value==="blocked_by_risk"?"destructive":"warning";}
async function createNotice(){
  if(!noticeForm.published_at){error.value="请填写原发布时间";return;}
  const {audience_scope,audience_id,...fields}=noticeForm; const audience=[audience_scope==='all_verified_users'?'all_verified_users':`${audience_scope}:${audience_id}`];
  const body={schema_version:"campus-notice-import/1.0",...fields,audience,published_at:new Date(noticeForm.published_at).toISOString(),content_sha256:await sha256(noticeForm.body),attachments:noticeAttachments.map(({file_name:_,...item})=>item)};
  await runAction(async()=>mutate("/admin/school-notices","POST",body),"通知已保存并进入审核队列");
  noticeForm.external_id="";noticeForm.title="";noticeForm.body="";noticeForm.original_url="";
  noticeAttachments.splice(0);
}
async function uploadNoticeAttachments(event:Event){
  const input=event.target as HTMLInputElement; const files=Array.from(input.files??[]); input.value="";
  if(!files.length)return; if(noticeAttachments.length+files.length>20){error.value="每条通知最多上传 20 个附件";return;}
  uploadingAttachments.value=true; error.value="";
  try{
    for(const file of files){
      const digest=await sha256Bytes(await file.arrayBuffer());
      const response=await adminRequest<{object_key:string;upload_url:string;headers:Record<string,string>}>('/object-upload-intents',{method:'POST',body:JSON.stringify({scope:'notice_attachment',file_name:file.name,content_type:file.type,size_bytes:file.size})});
      const upload=await fetch(response.data.upload_url,{method:'PUT',headers:response.data.headers,body:file});
      if(!upload.ok)throw new Error(`附件 ${file.name} 上传失败（${upload.status}）`);
      noticeAttachments.push({file_name:file.name,object_key:response.data.object_key,sha256:digest,content_type:file.type,size_bytes:file.size});
    }
    message.value=`已上传 ${files.length} 个附件`;
  }catch(e){error.value=e instanceof Error?e.message:'附件上传失败';}finally{uploadingAttachments.value=false;}
}
async function selectJSONL(event:Event){const input=event.target as HTMLInputElement;const file=input.files?.[0];jsonlContent.value=file?await file.text():"";importSummary.value=null;}
async function importJSONL(){
  loading.value=true;error.value="";message.value="";
  try{const response=await adminRequest<{job:{total_rows:number;created_rows:number;updated_rows:number;duplicate_rows:number;failed_rows:number}}>("/admin/notice-import-jobs",{method:"POST",headers:{"Idempotency-Key":crypto.randomUUID(),"Content-Type":"application/x-ndjson"},body:jsonlContent.value});const job=response.data.job;importSummary.value=[{label:"总行数",value:job.total_rows},{label:"创建",value:job.created_rows},{label:"更新",value:job.updated_rows},{label:"重复",value:job.duplicate_rows},{label:"失败",value:job.failed_rows}];message.value="JSONL 导入已完成";await load();}catch(e){error.value=e instanceof Error?e.message:"导入失败";}finally{loading.value=false;}
}
async function sha256(value:string){const digest=await crypto.subtle.digest("SHA-256",new TextEncoder().encode(value));return Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,"0")).join("");}
async function sha256Bytes(value:ArrayBuffer){const digest=await crypto.subtle.digest("SHA-256",value);return Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,"0")).join("");}
function formatBytes(value:number){return value<1024?`${value} B`:value<1024*1024?`${(value/1024).toFixed(1)} KB`:`${(value/1024/1024).toFixed(1)} MB`;}
function attachmentCount(value?:string){if(!value)return 0;try{return JSON.parse(atob(value)).length??0;}catch{return 0;}}
</script>
