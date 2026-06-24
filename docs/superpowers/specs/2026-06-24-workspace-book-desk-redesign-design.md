# Workspace book-desk redesign design

## Goal

把 `/workspace` 从普通后台静态页调整为“首页资料册打开后的工作台”，保持课程资料库主流程清晰，同时统一首页的暖纸张、资料夹、彩色 PDF 和轻动效语言。

## Scope

- 只改 web 前端的 `/workspace` 页面和必要测试。
- 不改后端、admin、Wiki、Blog、Forum、Packages 等队友新增功能。
- 保留现有静态数据和路由入口，先做视觉与交互统一，不接入新接口。

## Design Direction

- 页面底色继续使用暖纸张色，但增加纸纹、横线和档案索引感。
- 侧栏改成“档案索引”，强调资料分类和核心课程，不做重型后台导航。
- 主内容增加“工作台封面区”，明确这是从首页进入后的资料整理空间。
- 今日资料改为文件条/资料卡组合，核心课程改为彩色 PDF 入口卡，和首页右页课程入口呼应。
- 归档表格保持可扫读，视觉上更像纸张清单，hover 时浮起或高亮。
- 所有可交互卡片、课程入口、小组件具备 hover 浮起、轻微缩放和 active pressed 状态。
- 移动端保留顶部、搜索、分类横滑、资料卡和归档列表，避免复杂侧栏。

## Acceptance Checks

- `/workspace` 有稳定测试钩子 `data-workspace-style="book-desk"`。
- 桌面端能看到“资料册工作台”标题和“档案索引”侧栏语义。
- 课程入口卡使用彩色 PDF 视觉，并暴露 `data-workspace-card="course-pdf"`。
- 关键卡片具备 hover transform，Playwright 能检测 hover 后 transform 发生变化。
- `npm --workspace @final-review/web run lint` 和 `npm --workspace @final-review/web run build` 通过。
