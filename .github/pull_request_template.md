## 背景

<!-- 用户问题、当前代码证据和为什么现在要改。 -->

## 目标

<!-- 一个可验收的结果。 -->

## 范围

- 

## 明确不做

- 

## 影响模块

- [ ] Portal
- [ ] Study Web
- [ ] Study Admin
- [ ] Study API
- [ ] Study Worker
- [ ] QuizCraft
- [ ] Platform Core
- [ ] Platform Worker
- [ ] Design Tokens
- [ ] API Contracts
- [ ] Infra / Deployment
- [ ] Documentation only

## API / 数据 / 事件

<!-- 路径、表、Migration、OpenAPI、事件类型和 Owner。没有则写“无”。 -->

## 产品边界检查

- [ ] 没有在资料库新增第二套刷题能力
- [ ] QuizCraft 仍是唯一刷题产品
- [ ] 主站没有复制业务正文
- [ ] 数据 Owner 唯一，未跨服务直连数据库
- [ ] 页面使用 HENU Kit 产品归属和统一跳转

## 品牌与可访问性

- [ ] 使用“Kit 墨绿”，未称河南大学官方标准色
- [ ] 展示“学生自主运营 · 非河南大学官方项目”
- [ ] 360px 核心流程可用
- [ ] 键盘焦点、触控尺寸和对比度合格
- [ ] Loading / Empty / Error / Success / Disabled / Login required 完整
- [ ] 动画尊重 `prefers-reduced-motion`

## 安全与隐私

- [ ] 日志不包含完整邮箱、验证码、Token、Cookie 或密钥
- [ ] callback / return_to / CORS 使用白名单
- [ ] 写接口幂等与并发路径已设计
- [ ] 安全关键实现由非作者评审

## 验证

```text
在这里粘贴实际执行的命令和结果。
```

- [ ] 单元测试
- [ ] API 契约测试
- [ ] 集成测试
- [ ] E2E / Smoke
- [ ] 移动端人工测试
- [ ] 失败、并发和重试路径

## 发布

<!-- 环境、顺序、灰度比例、Migration 和监控。 -->

## 回滚

<!-- 明确的开关、镜像、命令或前一版本。 -->

## 截图 / 录屏

<!-- 前端变更必须附移动端和桌面端；文档中的 Mermaid 导出必须为已渲染图。 -->

## Reviewer 重点

- 
