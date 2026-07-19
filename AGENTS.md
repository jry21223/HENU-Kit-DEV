# henukit 项目规范（AGENTS.md）

henukit（Keep In Touch）是面向校园的综合性学生平台。技术栈：Next.js（App Router）+ TypeScript + Tailwind CSS v4 + GSAP（ScrollTrigger/Observer/ScrollTo）+ Three.js（仅首页 Hero）。

## 视觉标准：工业极简（Industrial Minimal）

全站统一，任何新页面不得偏离：

- **设计令牌**（`src/app/globals.css` `@theme`）：
  - `--color-paper: #F2F0EA` 主背景（暖纸白）
  - `--color-ink: #161513` 主文字/深色区块
  - `--color-accent: #FF4D00` 安全橙强调色，只用于关键节点（编号、CTA、激活态、反馈）
  - `--color-line / --color-line-dark` 1px 结构线（浅/深底）
  - 功能色（仅用于难度分等数据标识，禁止进装饰层）：
    `--color-easy: #3E7C4F`（难度 < 4.0）/ `--color-mid: #C79A2A`（4.0–6.9）/ `--color-hard: #C2401F`（≥ 7.0）
- **字体**：拉丁展示 Space Grotesk、等宽标签 IBM Plex Mono（next/font 变量注入）、中文系统栈。
- **语言元素**：1px 结构线、十字对位标记（+）、mono 编号（01–04 / Q-01 / L-01 等）、工程图纸网格（`.bg-blueprint`）、大字排版 + 小号 mono 标签的强对比。
- **禁忌**：渐变紫、玻璃拟态、圆角卡片堆叠、页面间白屏跳转。

## 动效约定

- GSAP 动画一律经 `useGSAP`（`@/lib/gsap` 统一注册插件与导出）；事件处理器不用 `contextSafe` 包 ref 闭包（Next 16 `react-hooks/refs` 规则会误报），卸载时 `killTweensOf` 清理。
- 页面间导航使用形变过渡系统（`src/components/practice/transition/`）：延迟导航 + 共享元素形变 + 通用塌缩/展开；浏览器前进/后退仅播进入动画；`prefers-reduced-motion` 时瞬时导航、一切循环/揭示动画静止。
- 首页 md+ 由 GSAP Observer 接管整屏滚动（`snap-scroll.tsx`）；子站（/practice）为工具页：普通滚动、无吸附；WebGL 仅用于首页 Hero 与题库页 Hero 的 3D 石膏像（`bank-hero-3d.tsx`，dynamic ssr:false + 可见性控制 frameloop）。
- 滚动入场动画统一 `start: "top 60%"`。

## 数据与渲染

- 纯前端 mock：所有数据在 `src/lib/practice/mock.ts`，随机性必须用种子化伪随机（mulberry32），SSR 与客户端输出一致；SVG 坐标计算固定两位小数，杜绝水合不匹配。
- 计时器等动态值挂载后再渲染（初始值与 SSR 一致）。
- 难度分全站统一：数值 1.0–10.0 一位小数，<4 绿 / 4–6.9 黄 / ≥7 红（`diff-badge.tsx`）。
- 图片体系：`src/lib/image.ts`（fileToDataUrl，JPG/PNG/WebP ≤2MB，事件回调内使用）+ `src/components/ui/img.tsx`（加载失败回退图纸占位块）；mock seed 图用 picsum 固定种子确定性外链；不用 next/image。美食/互助发布页支持上传（会话内存，刷新还原）；资料库不加图。
- 邮箱验证码（mock）：演示码固定 `427819`，发送按钮 60s 倒计时（`use-email-code.ts` + `code-field.tsx`），用于注册/验证码登录/修改密码。
- 宽度标准：工具页容器 `mx-auto max-w-[1440px] px-5 md:px-8`；阅读型正文限文字列宽 `max-w-[70ch]`。
- 子站 hero（library/food/campus 首页）：`components/site-hero/` 统一骨架（编号+大字+标语+计数 + 图纸画板 SVG 循环场景），不接 WebGL（3D 仅题库站与首页）。

## 工程

- 验证：`npm run build`、`npm run lint` 必须通过。
- 不做 git 提交等变更操作，除非用户明确要求。

## 子站路由清单

- `/`：首页（整屏模块展示，md+ Observer 接管滚动）。
- `/practice`：刷题子站（题库/题单/刷题/排行榜/数据面板；形变过渡系统导航；WebGL 仅题库页 Hero 石膏像）。
- `/account`：账号子站（mock 认证，登录态存 localStorage `henukit.session`，跨站通用）：
  - `/account/login`（登录/注册）、`/account/recover`（找回密码，演示验证码 427819）
  - 控制台（客户端守卫，未登录重定向 `/account/login?next=<路径>`）：`/account`（概览）、`/account/security`、`/account/wallet`、`/account/membership`、`/account/tickets`、`/account/notifications`、`/account/posts`（美食文章管理）
  - 账号站为轻量工具页：普通滚动、无吸附、无 WebGL、不接入 practice 形变过渡系统；认证 store 用 `useSyncExternalStore`（server snapshot 恒未登录），控制台操作数据仅存会话内存（刷新重置）。
- `/food`：美食子站（三校区：明伦 minglun / 金明 jinming / 龙子湖 longzihu）：
  - `/food`（校区入口）、`/food/campus/[campus]`（列表）、`/food/post/[id]`（详情 + Leaflet 地图）、`/food/publish`（发布/编辑，需登录）、`/food/leaderboard`（必吃排行）
  - `foodStore`（`src/lib/food/mock.ts`）useSyncExternalStore 单例驱动点赞/收藏/评论/发布/编辑/隐藏/删除，列表/详情/排行/控制台实时联动；hidden 文章列表与排行过滤
  - 地图：Leaflet + OSM，`next/dynamic` ssr:false，divIcon 自绘方块，瓦片失败/超时显示结构线兜底；富文本为结构化块（h2/p/quote/list），发布用 mini-markdown（`#`/`-`/`>`），禁止渲染原始 HTML
- `/campus`：互助平台子站（闲鱼模式 C2C 市集）：
  - `/campus`（市集：搜索 + 6 分类 + 求助/闲置双类型，CSS columns 双列瀑布流）、`/campus/item/[id]`（详情：卖家信用卡/想要/留言/平台担保流程 SVG）、`/campus/publish`（发布/编辑，需登录）、`/campus/deals`（我的交易，需登录）
  - `campusStore`（`src/lib/campus/mock.ts`）驱动想要/留言/发布/接单/确认完成/隐藏/删除，市集/详情/我的交易/控制台 `/account/deals` 实时联动
  - 订单状态流 open 待接单 → ongoing 进行中 → done 已完成（+ hidden）；担保流程五步：发布 → 赏金托管 → 接单服务 → 确认完成 → 平台结算
- `/library`：资料库子站（在线图书平台模式）：
  - `/library`（书库：搜索 + 五类 + 免费/收费 + 科目筛选）、`/library/item/[id]`（详情：三形态操作条）、`/library/read/[id]`（阅读器：分页/键盘 ←→/进度条/试读锁定墙）、`/library/shelf`（我的书架，需登录）
  - 三种商品形态：免费全文可读；收费标价格；收费带前 N 页试读 + 锁定墙
  - 购买走账号积分：`libraryStore.purchase` → `accountStore.spendPoints`（余额不足返回 false）→ 钱包余额/流水实时联动
