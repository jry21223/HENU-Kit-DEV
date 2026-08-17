---
status: accepted
amends: 0032
---

# Food Post 发布后治理由 Console 运营负责

ADR-0032 让 Food 拥有 Food Post 的创建与公开读，并刻意声明"发布后治理
（隐藏、档位争议）保持缺席"。运营在 Console 只能处理旧投稿模型
（`food_submissions` 审核、异常票、调档确认），对已经公开的学生投稿
没有任何修正手段——错误的校区、店名或点评只能留在公开榜单上。

本决策把 **Food Post 发布后治理** 移入 Console 运营职责：运营可读取已
发布投稿列表，修正店名、校区、档位、点评与价格 / 营业时间参考，并可
隐藏或恢复展示。

## Decision

- Console 运营权限环（`food.review`）新增 `post_edit` 治理命令，与
  `submission_edit` 同一命令通道（`POST /api/v1/commands`）、同一幂等账本、
  同一乐观版本与同一追加式审计事件（target_type=`post`）。
- Console 的 workspace 读接口新增 `posts` 段，返回已发布投稿的治理视图
  （含 `hidden` 与 `version`）。这是 ADR-0032 "console 凭据在 Post 路由上
  无效" 边界的受控扩展：治理读写在 Console 命令通道内完成，Food Post
  创建与公开读取仍走独立凭据环（food-post-create / food-post-read），
  三个身份互不通用。
- 编辑字段值域与创建一致：`venue_name`（1-160）、`campus`
  （minglun/jinming/longzihu）、`tier`（wire 枚举 hang/top/elite/npc/bad，
  存库映射为中文 label）、`review_text`（2-2000）、`price_reference` /
  `hours_reference`（≤200）、`hidden`（布尔）。只更新调用方提供的字段，
  `version+1`，创建者身份与创建快照不变。
- 隐藏不影响公开读路由的数据所有权：`hidden=true` 的 post 从公开
  workspace 治理列表仍可见（供恢复），但从公开读与 venue 汇总中消失。

## Consequences

- 运营可在 Console 修正错误信息并隐藏违规投稿，无需改库或请求开发。
- ADR-0032 的 `food_posts` 表新增 `version` 列（迁移 000004），旧行默认
  version=1；治理写入与创建写入共用同一张表，创建路径不写 version。
- 任何治理写入都留下幂等记录与审计事件，可追溯运营对公开内容的所有修改。
- 后续若出现"学生编辑自己的投稿"需求，与运营治理通道保持独立，仍待单独决策。
