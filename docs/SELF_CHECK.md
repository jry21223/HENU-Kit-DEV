# Self Check

## 当前阶段

Phase 15：移动端优化（已完成基础版）

## 本阶段目标

- 保证学生在手机上好用。
- 首页、课程列表、课程详情、资料详情、刷题、错题本和登录页可正常浏览。
- 按钮足够大。
- 题目阅读舒适。
- 没有横向滚动条。

## 已完成内容

- 使用 Browser 390px 视口巡检关键页面：
  - `/`
  - `/login`
  - `/schools`
  - `/courses`
  - `/courses/discrete-math`
  - `/materials/discrete-note`
  - `/courses/discrete-math/practice`
  - `/practice/result/discrete-math`
  - `/me/wrong-questions`
  - `/me/submissions`
  - `/me/orders`
  - `/admin/analytics`
- 修复过期阶段文案：
  - 首页。
  - 学校页。
  - 404 页面。
  - admin 首页。
- 移动端触控优化：
  - 课程筛选 select 和按钮高度提升到 44px。
  - 刷题 radio 控件增大，选项整行仍可点击。
- 保持桌面布局不重构。

## 未完成内容

- 未新增底部导航。
- 未做全站视觉重构。
- 未新增动画。
- 未对所有后台细分页面逐页做深度移动端设计。
- Computer Use 最终全阶段自检仍未通过，当前插件初始化失败。

## 代码改动摘要

核心更新文件：

- `src/app/page.tsx`
- `src/app/schools/page.tsx`
- `src/app/not-found.tsx`
- `src/app/admin/page.tsx`
- `src/components/course/course-filter-form.tsx`
- `src/components/practice/practice-question-list.tsx`
- `README.md`
- `docs/ROADMAP.md`
- `docs/CHANGELOG.md`
- `docs/SELF_CHECK.md`

## 本地运行检查

- [x] 项目可以安装依赖。
- [x] PostgreSQL 容器健康。
- [x] 项目可以启动，`http://localhost:3000/` 返回 200。
- [x] 核心页面可以访问。
- [x] API 返回正常。
- [x] 数据库迁移未变更。
- [x] seed 数据正常。
- [x] `npm run typecheck` 通过。
- [x] `npm run lint` 通过。
- [x] `npm test` 通过。
- [x] `npm audit` 通过。
- [x] `npm run build` 通过。

## 功能检查

- [x] 首页移动端可浏览。
- [x] 登录页移动端可浏览。
- [x] 学校页移动端可浏览。
- [x] 课程列表移动端可浏览。
- [x] 课程详情移动端可浏览。
- [x] 资料详情移动端可浏览。
- [x] 刷题页面移动端可浏览。
- [x] 练习结果移动端可浏览。
- [x] 错题本移动端可浏览。
- [x] 投稿页移动端可浏览。
- [x] 订单页移动端可浏览。
- [x] 统计页移动端可浏览。

## 权限检查

- [x] 未登录用户权限未改动。
- [x] 学生用户权限未改动。
- [x] admin 用户权限未改动。
- [x] paid 内容没有被本阶段绕过。
- [x] draft 内容不会展示给普通用户。

## UI 检查

- [x] 390px 视口关键页面无横向溢出。
- [x] 没有明显错位。
- [x] 重要按钮清楚。
- [x] 课程筛选控件高度足够。
- [x] 刷题选项阅读和点击区域可用。
- [x] 空状态友好。
- [x] 错误状态友好。

## 安全检查

- [x] 本阶段未放宽任何 API 权限。
- [x] 本阶段未改动验证码暴露逻辑。
- [x] 本阶段未改动 paid 下载权限。
- [x] 本阶段未改动 admin 服务端权限。

## 回归检查

- [x] Phase 1 页面没有损坏。
- [x] Phase 2 数据库未损坏。
- [x] Phase 3 登录流程仍可用。
- [x] Phase 4 课程/资料 API 未损坏。
- [x] Phase 5 下载权限未改坏。
- [x] Phase 6 admin 权限仍有效。
- [x] Phase 7 刷题测试通过。
- [x] Phase 8 错题测试通过。
- [x] Phase 9 paid entitlement 测试通过。
- [x] Phase 10 支付签名测试通过。
- [x] Phase 11 投稿规则测试通过。
- [x] Phase 12 AI job 测试通过。
- [x] Phase 13 PDF 水印测试通过。
- [x] Phase 14 analytics 测试通过。

## 本轮运行命令

- `npm run typecheck`
- `npm run lint`
- `npm test`
- `npm audit`
- `npm run build`
- Browser：390px 视口批量访问 12 个关键页面。

## 检查结果

通过：

- 12 个关键页面 390px 移动端无横向溢出。
- 过期阶段文案已清理。
- 课程筛选控件和刷题单选控件已做基础触控优化。
- `npm run typecheck`、`npm run lint`、`npm test`、`npm audit`、`npm run build` 通过。

待注意：

- `npm run build` 成功，但仍有既有 Turbopack NFT tracing warning，来源仍指向下载 route 的本地文件读取。
- Computer Use 初始化仍失败：`Package subpath './dist/project/cua/sky_js/src/targets/windows/internal/computer_use_client_base.js' is not defined by "exports"`。

## 风险

- 后台深层表单仍是基础版，后续需要按真实运营频率继续优化移动端。
- 未新增底部导航，移动端主导航仍采用顶部换行布局。
- 移动端验收使用 390px 视口，后续可补充 320px 和更大 Android 视口。

## 下一步建议

后续最小任务：

1. 修复 Turbopack NFT tracing warning。
2. 为 admin 课程、资料、题库补齐表单 UI。
3. 增加课程访问日志和趋势统计。
4. 接入真实 AI worker 前先设计审核队列。
5. 做真实支付商户联调前补充订单对账字段。
