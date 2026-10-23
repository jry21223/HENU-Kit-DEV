# 移动端任务页验收（#608）

2026-10-23。保持现有纸白、墨黑、橙色、细线和网格体系。

## 可见变化

| 页面 | 结果 |
|---|---|
| 资料库 | 完整目录仍用于筛选和全局统计，每批只渲染24份；手机卡片紧凑，同一标签页软导航返回保留条件、显示范围和位置。 |
| 刷题 | 手机五个中文任务完整显示，点击区44px；桌面保留编号，禁用提示与键盘焦点保留。 |
| 美食 | 压缩手机宣传和导览，375px首店完整进入首屏；相同店名只出现一次。 |
| 首页解析 | 全文始终由React渲染，滚动与动效切换不会清空正文。 |
| 加载骨架 | 减少动态效果下静止，正常模式保留加载反馈。 |

## 截图

使用线上匿名公开目录快照（480份资料、10篇美食），在隔离本地预览中验证；登录态和生产写接口未连接。截图来自Chromium视口模拟，不是真机验收；底部Next.js角标是开发环境工具。

- [资料库375×812](library-375.png)：首屏可读到前两份真实资料标题。
- [美食375×812](food-375.png)：首店约位于660–757px，完整进入首屏。
- [刷题导航375×812](practice-nav-375.png)：五个任务均完整可见。
- [资料库1440×900](library-desktop-1440.png)：保留网格和书脊装饰。

## 验证

- `pnpm --filter @henukit/portal typecheck`：通过。
- `pnpm --filter @henukit/portal lint`：0错误，3项既有warning。
- `NODE_OPTIONS=--no-experimental-webstorage pnpm --filter @henukit/portal test`：40文件、311项通过。
- `pnpm --filter @henukit/portal test:unit`：5项通过。
- 核心目录/统计/美食/导航回归50项，加响应式/对比度/触控/焦点/入场/上级返回134项，合计184项通过。原先4条按全量渲染计数的旧用例，改为先通过按钮展开再离开，原慢网与输入接管断言保留并重测通过。
- V2读取开启且目录关闭、V2读取和目录都开启，两种组合的375/390px五路由导航通过；生产开关下完整5项移动任务回归通过。
- `NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1 NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=1 pnpm --filter @henukit/portal build`：生产构建和library/mock公开产物检查通过。

回归接入既有`test:e2e:responsive`与`test:e2e:stats`。目录延迟2秒仍恢复位置；慢请求期间主动滚动、按键、触摸后不会被拉回。硬刷新/新标签使用默认筛选条件。此处记录本地验收，CI及生产发布状态以PR和发布记录为准。
