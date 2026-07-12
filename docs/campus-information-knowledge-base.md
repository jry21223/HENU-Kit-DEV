# 校园通知与规则知识库设计

> 状态：设计提案，暂不包含生产实现  
> 目标仓库：`jry21223/final-review-platform`  
> 首个信息源：河南大学校园 QQ 空间信息账号

## 1. 背景

学校通知、保研规则、竞赛目录、奖学金与考试信息长期分散在 QQ 空间动态、图片、评论、PDF 和网页中。现有内容难以检索、缺少版本与适用范围，也无法稳定支持 Web、LangBot 或外部 Agent 问答。

本提案建立一条可审计的数据链路：

```text
QQ 空间 / 学校网站 / PDF / 人工投稿
                │
                ▼
         Source Connector Layer
                │
                ▼
       Canonical Source Record
                │
       OCR / 清洗 / 分类 / 去重
                │
                ▼
      Notice / Knowledge Document
                │
          人工审核与发布
                │
                ▼
     Full Text + Vector Retrieval
                │
       Web / LangBot / Skill
```

## 2. 设计原则

1. **原始证据不可覆盖**：保留原始 JSON、HTML、图片和采集元数据。
2. **采集器与业务解耦**：任何外部工具都必须先转换为统一格式，不能直接写业务表。
3. **Word 不是数据源**：Word、Markdown、PDF 只作为导出物；长期数据源是数据库和对象存储。
4. **问答必须可追溯**：每条事实必须能够回溯到动态、图片、评论或正式文件。
5. **规则必须带范围和版本**：学校、学院、届次、有效期、发布机构、替代关系均为必填或显式未知。
6. **AI 不直接发布**：模型只生成草稿，所有校园政策类内容进入人工审核。
7. **增量幂等**：历史导入和实时同步使用同一外部 ID 去重。
8. **评论默认低可信**：发布者补充评论可作证据，普通用户评论只进入 FAQ 候选池。

## 3. 信息源工具选择

### 3.1 历史全量导出

首选：[`2015winter/QZoneExporter`](https://github.com/2015winter/QZoneExporter)

原因：

- Chrome Manifest V3 扩展，使用当前浏览器登录态；
- 支持说说、评论、图片等内容；
- 支持 JSON、Markdown、Excel、HTML 导出；
- 支持增量备份与离线核查；
- 2026 年仍有维护记录。

历史补漏：[`Christina-zhou/get-qzone-history`](https://github.com/Christina-zhou/get-qzone-history)

用途：

- 使用 Playwright 和真实浏览器翻页；
- 补抓长文本、正文和图片；
- 支持 checkpoint 断点续跑；
- 用于和 QZoneExporter 结果做数量与正文交叉验证。

不再作为主方案：[`xjr7670/QQzone_crawler`](https://github.com/xjr7670/QQzone_crawler)

该项目使用旧版 `emotion_cgi_msglist_v6`、Python 3.5 和有限 SQLite 字段。可以保留为接口与历史字段参考，但不应承担当前全量导入。

### 3.2 后续增量同步

推荐：[`Gu-Heping/onebot-qzone`](https://github.com/Gu-Heping/onebot-qzone)

使用范围：

- 周期拉取新动态；
- 新评论与发布者补充回复；
- 图片拉取；
- OneBot HTTP/WebSocket 事件转发。

注意：QQ 空间不存在稳定公开 API。实时同步必须按 best-effort 设计，采集失败不能阻塞平台主业务。

## 4. 仓库集成位置

建议目录：

```text
integrations/
├── qzone-importer/          # 历史文件适配器
│   ├── exporter-adapter/
│   ├── playwright-adapter/
│   └── canonical/
├── qzone-collector/         # onebot-qzone 增量消费器
└── README.md

services/api/internal/
├── source/                  # 信息源与原始记录 API
├── ingestion/               # 导入任务与幂等逻辑
├── notice/                  # 校园通知领域
├── knowledge/               # 规则文档、分块、检索
└── qa/                      # 引用式问答

services/worker/internal/tasks/
├── source_normalize
├── source_ocr
├── source_classify
├── knowledge_extract
├── knowledge_merge
├── knowledge_index
└── faq_generate
```

外部采集器只调用内部导入 API：

```http
POST /api/v1/internal/ingestion/source-items
X-Ingestion-Signature: <hmac>
X-Idempotency-Key: qzone:<source-code>:<tid>
```

禁止：

```text
外部爬虫 → 直接写 PostgreSQL
外部爬虫 → 直接创建 WikiEntry
LangBot → 保存 QQ Cookie
LangBot → 直接读知识库数据表
```

## 5. 四层数据结构

### 5.1 Raw Archive：原始档案层

保存采集工具原始输出，不做覆盖式修改：

```text
data/raw/qzone/<source-code>/
├── <run-id>/
│   ├── manifest.json
│   ├── export.json
│   ├── export.html
│   ├── logs/
│   └── assets/images/
└── incremental/
```

`manifest.json` 至少记录：

```json
{
  "run_id": "run_20260712_001",
  "source_code": "henu_exam_wall",
  "collector": "qzone_exporter",
  "collector_version": "3.x",
  "started_at": "2026-07-12T10:00:00+08:00",
  "finished_at": "2026-07-12T12:30:00+08:00",
  "post_count": 1234,
  "asset_count": 2860,
  "comment_count_captured": 4120,
  "status": "completed"
}
```

### 5.2 Canonical Source Record：统一源记录层

交换格式使用 UTF-8 JSONL，每行一条动态。Schema 版本必须显式声明。

```json
{
  "schema_version": "qzone-source-item/1.0",
  "external_id": "qzone:henu_exam_wall:tid-123",
  "source": {
    "type": "qzone",
    "source_code": "henu_exam_wall",
    "collector": "qzone_exporter",
    "captured_at": "2026-07-12T10:30:00+08:00",
    "import_run_id": "run_20260712_001"
  },
  "published_at": "2026-06-20T19:30:00+08:00",
  "content": {
    "text_raw": "关于2027届推荐免试研究生工作的通知……",
    "text_normalized": "关于2027届推荐免试研究生工作的通知……",
    "post_url": "https://user.qzone.qq.com/...",
    "device": "iPhone",
    "location": null,
    "is_forward": false
  },
  "assets": [
    {
      "asset_id": "asset-001",
      "type": "image",
      "source_url": "https://...",
      "storage_key": "qzone/henu_exam_wall/asset-001.jpg",
      "sha256": "...",
      "ocr_status": "pending"
    }
  ],
  "comments": [
    {
      "external_id": "comment-001",
      "parent_external_id": null,
      "created_at": "2026-06-20T20:10:00+08:00",
      "author_relation": "source_owner",
      "text_raw": "补充：材料提交截止时间为6月25日。",
      "text_redacted": "补充：材料提交截止时间为6月25日。",
      "trust_level": "publisher"
    }
  ],
  "metrics": {
    "comment_count_reported": 8,
    "comment_count_captured": 5,
    "comment_capture_status": "partial",
    "like_count": 36
  },
  "provenance": {
    "raw_payload_ref": "raw/run_20260712_001/post-123.json",
    "raw_html_ref": "raw/run_20260712_001/export.html"
  },
  "hashes": {
    "content_sha256": "..."
  }
}
```

约束：

- `external_id` 全局稳定；
- `content_sha256` 用于正文变更检测；
- `comment_capture_status` 只能是 `complete`、`partial`、`unknown`；
- QQ 号、昵称、头像等个人标识默认不进入发布数据；
- 原始 payload 只保存引用，不复制到所有下游表。

### 5.3 Knowledge Document：业务知识层

一条动态不等于一份规则文档。多个动态、图片、发布者补充评论和更正通知可以合并为一个知识文档。

```json
{
  "document_type": "postgraduate_recommendation_notice",
  "title": "河南大学软件学院2027届推荐免试工作通知",
  "status": "pending_review",
  "scope": {
    "school": "河南大学",
    "college": "软件学院",
    "cohort": "2027届"
  },
  "dates": {
    "published_at": "2026-06-20",
    "application_deadline": "2026-06-25T17:00:00+08:00",
    "effective_from": "2026-06-20",
    "effective_to": null
  },
  "summary": "……",
  "facts": [
    {
      "key": "材料提交截止时间",
      "value": "2026-06-25 17:00",
      "evidence_refs": [
        "source-item-001#asset-002",
        "source-item-001#comment-001"
      ]
    }
  ],
  "source_item_ids": ["source-item-001", "source-item-009"],
  "supersedes_document_id": null
}
```

### 5.4 Retrieval Index：检索层

只对已审核发布的文档创建检索分块：

```text
knowledge_chunks
- id
- document_id
- heading_path
- text
- school_id
- college_id
- cohort
- document_type
- effective_from
- effective_to
- evidence_refs JSONB
- embedding
- created_at
```

回答链路：

```text
chunk
→ knowledge_document
→ document_source_link
→ source_item
→ 原始动态 / 图片 / 评论
```

## 6. 建议数据库模型

第一阶段新增：

```text
content_sources
- id
- school_id
- code
- type
- display_name
- external_account_id
- trust_level
- authorization_status
- status

 ingestion_runs
- id
- source_id
- collector
- collector_version
- started_at
- finished_at
- status
- stats JSONB
- error_summary

source_items
- id
- source_id
- external_id
- published_at
- text_raw
- text_normalized
- raw_payload_ref
- capture_status
- content_hash
- current_version

source_item_versions
- id
- source_item_id
- version
- text_raw
- content_hash
- captured_at
- raw_payload_ref

source_assets
- id
- source_item_id
- external_id
- type
- source_url
- storage_key
- sha256
- ocr_status
- ocr_result JSONB

source_comments
- id
- source_item_id
- external_id
- parent_external_id
- text_raw
- text_redacted
- author_relation
- trust_level
- published_at
- capture_status

campus_notices
- id
- school_id
- college_id
- title
- summary
- category
- audience JSONB
- published_at
- deadline_at
- event_start_at
- event_end_at
- importance
- status

knowledge_documents
- id
- school_id
- college_id
- document_type
- title
- content
- issuing_organization
- applicable_cohort
- effective_from
- effective_to
- version
- status
- supersedes_document_id

 document_source_links
- document_id
- source_item_id
- relation_type

knowledge_chunks
faq_entries
```

关键索引：

```sql
UNIQUE (source_id, external_id)
UNIQUE (source_item_id, external_id) -- comments
UNIQUE (source_item_id, sha256)      -- assets
INDEX (school_id, college_id, document_type, status)
INDEX (effective_from, effective_to)
```

## 7. 历史导入与增量同步合并

统一外部 ID：

```text
qzone:<source-code>:<tid>
```

幂等策略：

1. 未存在：创建 `source_item`；
2. 已存在且正文哈希相同：只补充图片、评论和统计；
3. 已存在且正文哈希变化：创建新版本；
4. 原动态删除：标记 `source_deleted`，不物理删除证据；
5. 后续更正：建立 `correction` 或 `supersedes` 关系；
6. 历史导入和 OneBot 增量必须调用同一 Upsert 服务。

## 8. AI 处理流水线

### 8.1 阶段 0：确定性处理，不调用模型

使用 Go、Python 或 TypeScript 完成：

- 导出格式转换；
- JSON/HTML 清洗；
- 时间与时区规范化；
- `tid` 与内容哈希去重；
- 图片下载与 SHA-256 去重；
- 评论父子关系恢复；
- QQ 号、昵称、头像、手机号等脱敏；
- 关键词初筛；
- Schema 校验。

### 8.2 阶段 1：OCR

默认候选：PaddleOCR 系列，部署时保持 Provider 接口可替换。

OCR 输出必须包含：

```text
文本
图片编号
文本区块坐标
表格结构
置信度
```

不能只保存拼接后的纯文本，否则无法引用图片中的具体证据区域。

### 8.3 阶段 2：相关性分类

分类枚举：

```text
irrelevant
notice
policy
competition
postgraduate_recommendation
scholarship
exam
course_selection
faq_candidate
correction
```

先使用规则筛选，再使用低成本结构化输出模型分类。无关内容不进入高成本提取阶段。

### 8.4 阶段 3：结构化提取

使用支持 JSON Schema / Structured Output 的视觉语言模型。模型通过配置注入，不在领域代码中绑定单一厂商。

提取字段：

- 标题；
- 发布机构；
- 通知类型；
- 学校、学院、专业、年级、届次；
- 报名条件；
- 截止日期；
- 材料清单；
- 竞赛级别与目录项；
- 联系方式；
- 更正或替代关系；
- 证据引用；
- 不确定字段；
- 人工审核理由。

所有结果写入现有 `AITask` / `AIDraft` 审核流，建议新增：

```text
output_type = campus_notice
output_type = knowledge_document
output_type = faq_candidate
output_type = source_correction
```

### 8.5 阶段 4：检索索引

建议：

```text
PostgreSQL metadata filter
+ PostgreSQL full-text search
+ pgvector dense embedding
+ reranker
```

Embedding 可先评估 BGE-M3；必须通过 Provider 接口封装，禁止在数据模型中绑定特定模型。

检索前必须过滤：

```text
school_id
college_id
cohort
status = published
document_type
effective date
```

## 9. 评论可信度与 FAQ

评论分级：

| 来源 | 使用方式 |
| --- | --- |
| 信息源账号本人回复 | 可作为补充证据，仍需审核 |
| 授权运营者回复 | 较高可信证据，仍需审核 |
| 学生提问 | FAQ 候选问题 |
| 普通学生回答 | 仅供审核参考 |
| “听说”“应该是” | 禁止进入正式知识事实 |
| 闲聊、表情、重复内容 | 丢弃 |

FAQ 流程：

```text
评论问题
→ 去标识化
→ 相似问题聚类
→ 从正式 Knowledge Document 检索答案
→ 生成 FAQ 草稿
→ 人工审核
```

禁止根据其他学生评论直接生成政策答案。

## 10. QA 回答契约

每个政策类回答至少返回：

```json
{
  "answer": "直接结论",
  "scope": {
    "school": "河南大学",
    "college": "软件学院",
    "cohort": "2027届"
  },
  "sources": [
    {
      "document_id": "...",
      "chunk_id": "...",
      "title": "...",
      "published_at": "...",
      "evidence_refs": ["source-item-001#asset-002"]
    }
  ],
  "conflicts": [],
  "confidence": "high",
  "last_verified_at": "2026-07-12T12:00:00+08:00"
}
```

硬性要求：

- 不跨学院混合规则；
- 不跨届次默认复用规则；
- 找不到明确依据时必须说明知识库无明确条款；
- 多份有效来源冲突时同时展示；
- 只允许引用 `published` 文档；
- 不允许 LLM 以常识补齐保研、综测、竞赛认定结论。

## 11. Admin 审核界面

第一阶段增加：

1. 信息源管理；
2. 采集运行记录；
3. 原始动态审核；
4. 图片与 OCR 对照；
5. 通知与知识文档编辑；
6. FAQ 候选审核；
7. QA 引用追踪。

单条审核建议左右分栏：

```text
左侧：原始 QQ 正文、图片、评论、采集完整度
右侧：结构化字段、事实、证据引用、适用范围、有效期
```

## 12. 权限设计

不要创建具有全局权限的 `official` 用户角色。

建议新增组织范围成员表：

```text
organization_memberships
- organization_id
- user_id
- role: publisher / source_manager / reviewer
- status
```

信息源运营者只能管理自己的来源和草稿，不能获得平台全局 Admin 权限。

前台展示建议：

```text
来源账号已授权
校园信息账号
非校方认证
```

除非获得校方明确授权，不使用“学校官方”或校方背书表述。

## 13. 隐私与安全

1. 只采集公开可见或已获授权的内容；
2. QQ Cookie、`skey`、`p_skey` 仅存在采集服务 Secret 中；
3. Cookie 不写数据库、不写日志、不提交 Git；
4. 评论者 QQ 号、昵称、头像默认去标识化；
5. 原始评论设置保留期限和删除流程；
6. 图片下载设置域名白名单、大小限制和 MIME 校验；
7. 内部导入 API 使用 HMAC、时间戳和幂等键；
8. 采集器只读，不自动点赞、评论、转发；
9. 设置低频轮询与指数退避；
10. 提供错误、侵权和下架反馈入口。

## 14. 分阶段实施计划

### Phase 0：工具验证

- 使用 QZoneExporter 导出 5–10 页；
- 抽查纯文本、长文本、多图、转发、评论较多、发布者补充评论；
- 记录正文、`tid`、图片、发布时间和评论完整度；
- 使用 get-qzone-history 对相同范围交叉校验。

验收：

- 至少 95% 样本正文完整；
- 每条记录有稳定去重键；
- 图片可落盘并计算哈希；
- 评论明确标记完整、部分或未知。

### Phase 1：历史导入

- 定义 JSON Schema；
- 实现 QZoneExporter Adapter；
- 实现 Playwright Export Adapter；
- 导出 Canonical JSONL；
- 建立 Raw Archive；
- 导入 `content_sources`、`ingestion_runs`、`source_items`、`source_assets`、`source_comments`。

### Phase 2：OCR 与知识草稿

- 图片 OCR；
- 相关性分类；
- Structured Output 提取；
- 复用 `AITask` / `AIDraft` 审核链；
- Admin 对照审核。

### Phase 3：检索与 QA

- 建立 `knowledge_documents` 与 `knowledge_chunks`；
- 全文和向量混合检索；
- 引用式 QA API；
- Web 问答入口；
- LangBot 工具调用。

### Phase 4：增量同步

- 部署 onebot-qzone；
- 实现增量事件 Adapter；
- 和历史导入共用 Upsert；
- 增量失败告警和重新登录流程；
- 新通知订阅与现有 `Notification` 投递衔接。

## 15. 暂不实施

本设计 PR 不包含：

- QQ Cookie 或任何真实账号凭据；
- 自动化全量抓取任务；
- 生产数据库迁移；
- 自动发布知识文档；
- 自动群发 QQ 消息；
- 未经授权的校方身份或品牌标识；
- 对现有 `Notification`、Wiki、Moment 语义的破坏性修改。

## 16. 后续决策点

实施前需要确认：

1. 信息账号是否同意转载、结构化整理和长期保存；
2. 历史导出工具对目标账号的实测完整度；
3. 评论是否确有产品价值，或只抓发布者回复；
4. 原始图片存储位置和保留周期；
5. OCR Provider 与运行成本；
6. Embedding/Reranker Provider；
7. 首期只覆盖全校信息，还是先覆盖单学院；
8. 哪些类型必须双人审核；
9. 规则废止和替代的运营流程。
