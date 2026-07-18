# HENU Kit API Contracts

本目录是 HENU Kit 新平台接口和事件的唯一契约来源。

```text
packages/api-contracts/
├── openapi/
│   ├── platform-core.yaml
│   └── admin.yaml
├── schemas/
│   └── campus-notice-import.schema.json
├── examples/
│   ├── campus-notice.valid.json
│   └── campus-notice.invalid.json
└── events/
    └── event-envelope.schema.json
```

## 规则

- OpenAPI 使用 3.1。
- 新平台 API 前缀为 `/api/v1`。
- JSON 字段使用 `snake_case`。
- 时间使用 UTC ISO 8601。
- 响应包含 `request_id`。
- 写接口按场景声明 `Idempotency-Key`。
- 内部服务接口与浏览器接口使用不同 tag、安全方案和路径空间。
- 生成客户端和服务端类型不得手工修改；生成命令应进入 CI。

## 旧接口兼容

现有 final-review API 使用 `{code,message,data}`，QuizCraft 使用裸 JSON。它们不在一个发布中强制改写。

迁移期间：

- adapter 将旧格式转换为新格式；或
- 新前端按页面逐步改用新契约；
- 旧接口加入 deprecation 文档与调用指标；
- 消费方清零后才删除实现。

## 变更级别

- Patch：文案、示例和不影响消费者的约束澄清。
- Minor：新增可选字段、接口或事件版本。
- Major：删除字段、改变语义或不兼容认证方式。

安全关键契约变更必须由非作者评审。

## 本地验证

```bash
python -m unittest discover -s packages/api-contracts/tests -p "test_*.py"
npx --yes @redocly/cli@2.39.0 lint --extends minimal packages/api-contracts/openapi/admin.yaml
npx --yes openapi-typescript@7.13.0 packages/api-contracts/openapi/admin.yaml -o apps/admin/src/lib/generated/admin.ts
```
