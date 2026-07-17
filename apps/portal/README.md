# HENU Kit Portal

目标域名：`henukit.cn`

本目录用于 HENU Kit 主站。主站是统一校园工具系统的产品外壳，不是简单外链列表，也不复制资料库和 QuizCraft 的业务实现。

## 首批职责

- HENU Kit 品牌和非官方声明。
- 美食榜单、工具箱、学习三个一级入口。
- 学习页中的资料库、刷题和接毕设（二期）。
- 全局导航和账户入口。
- 子产品状态、维护者和更新时间。
- 继续上次任务。
- 工具目录和公告。

## 明确不做

- 资料文件、资料详情和下载。
- 题库、作答、错题和排行榜。
- 账户数据库和邮件发送。
- 所有业务后台。

## 首批路由

```text
/
/tools
/learn
/food
/status
/legal/non-official
```

## 验收

- 360px 宽度可完成入口发现。
- 新用户 30 秒内找到资料库或开始刷题。
- HENU Kit Logo、导航、账户入口和页脚位置稳定。
- 固定展示“学生自主运营 · 非河南大学官方项目”。
- 使用 `packages/design-tokens`，主色称为 Kit 墨绿。
- 不使用校徽或暗示官方身份的图形和文案。

## 技术栈决定

Portal 的具体前端栈在实现 Issue 中确认。优先满足：

- 与当前 Node 22/npm 工作流兼容。
- 支持 SSR/静态输出与 SEO。
- 可独立构建和部署。
- 不要求 Study Web 或 Quiz Web 改框架。

在更新根 `package.json` 和 lockfile 前，本目录只作为产品与结构占位，不加入现有 npm workspace，避免破坏当前 `npm ci`。