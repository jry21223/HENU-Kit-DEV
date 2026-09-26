# henukit — Keep In Touch

面向校园的综合性学生平台，集成资料库、刷题、美食、互助、求职雷达五个模块。

最后更新：2026-09-26。

> HENU Kit 是学生自主运营的非官方项目，不代表河南大学或任何学院；涉及政策和办事要求时，以学校及学院官方来源为准。

## 技术栈

| 层级 | 技术 |
|---|---|
| 框架 | Next.js (App Router) |
| 语言 | TypeScript |
| 样式 | Tailwind CSS v4 |
| 动画 | GSAP (ScrollTrigger / Observer / ScrollTo) |
| 3D | Three.js + React Three Fiber（仅首页 Hero 与题库页 Hero） |
| 字体 | Space Grotesk（展示）/ IBM Plex Mono（标签）/ 中文系统栈 |

## 快速开始

```bash
npm install
npm run dev
```

浏览器打开 [http://localhost:3001](http://localhost:3001)。

## 命令

| 命令 | 说明 |
|---|---|
| `npm run dev` | 本地开发服务器 |
| `npm run build` | 生产构建（必须通过） |
| `npm run lint` | ESLint 检查（必须通过） |
| `npm run start` | 生产服务器 |

## SEO / GEO 基础设施

- `/robots.txt` 允许普通搜索与回答型搜索爬虫访问公开 HTML，仅阻止 API 抓取；账户、写入、个性化和阅读器路由通过 `X-Robots-Tag: noindex, nofollow` 禁止索引。
- `/sitemap.xml` 只列出构建时确定存在的公开入口（含隐私政策与用户协议），不伪造动态详情 URL。
- `/llms.txt` 提供项目定位、公开入口、非官方边界与引用规则。
- 根页面提供 canonical、Open Graph、Twitter Card 与 `WebSite`/社区维护者 JSON-LD。
- 页面标题只有一种格式，由 `src/lib/seo.ts` 的 `pageTitle` 生成：模块内页 `页面名 — 模块名 | HENU Kit`，模块首页与不属于模块的页面 `页面名 | HENU Kit`；首页保留站点标题 `HENU Kit — 河南大学校园工具`。品牌只由标题模板补一次：布局一旦写了纯字符串标题，Next 就不再把根布局的模板传给更深的页面，所以模块 `layout.tsx` 用 `moduleLayoutTitle` 导出标题，把模板接着传下去，不要写纯字符串标题。客户端页面的标题由同一段的服务端 `layout.tsx` 导出；详情页的内容在客户端到达后，由 `useDocumentTitle` 把内容名补进标题。题库收藏夹的页头显示题库名时（来自收藏夹列表链接上的 `?name=`），标题同样带上它：`高等数学 收藏夹 — 智能刷题 | HENU Kit`；没有题库名时保留静态标题“题库收藏夹”。
- canonical origin 由构建变量 `NEXT_PUBLIC_SITE_URL` 决定，默认 `https://henukit.cn`；变量必须是无路径、查询或片段的 HTTP(S) origin。

站长平台提交、WAF 验证、内容版本规则和验收方法见 [`../../docs/product/seo-geo.md`](../../docs/product/seo-geo.md)。

## 子站路由

### 子站页头
五个子站共用 `SubSiteNav`（`src/components/sub-site-nav.tsx`）：返回上一级、子站字标、标签行和账户入口。只有一个标签的子站（资料库）不渲染标签行；未开放的功能标签不可点，旁边直接标“未开放”。手机上标签行放不下时横向滑动，当前标签和键盘聚焦的标签会被整个滑进可见范围；标签的焦点框画在标签里面，不会被滑动行裁掉。当前标签带 `aria-current="page"`，已登录时账户入口读作“昵称的账户概览”。返回上一级（`src/components/back-link.tsx`）默认就是 44px 高的点击区，正文里的回退入口只补间距和边框。

首页、子站页头和页面正文共用同一个内容框 `max-w-site`（1440px，左右留白 `px-5 md:px-8`），返回链接与正文标题左缘对齐；规则见 [`DESIGN_SYSTEM.md`](../../docs/product/DESIGN_SYSTEM.md) 的“内容框”。

### 首页 `/`
首屏提供“找资料”“开始刷题”“看岗位”，分别进入 `/library`、`/practice`、`/career`；下方介绍与导航对应的五个模块，只写已上线的能力。刷题区块介绍按科目搜索题库、随机 / 难题 / 章节 / 收藏四种练习、作答后的参考答案与题库自带解析，以及按题库计算的掌握度；右侧解析面板标为示例。AI 推题（[#530](https://github.com/jry21223/HENU-Kit-DEV/issues/530)）上线前不做相关宣传。保留米白、网格、墨黑和橙色视觉，以及 md+ 视口由 GSAP Observer 接管的吸附滚动和 WebGL 3D 场景。各模块只在 md+ 占满一屏，与吸附滚动同时启用；md 以下是普通滚动，模块按内容高度排列，不留整屏空白（[#542](https://github.com/jry21223/HENU-Kit-DEV/issues/542)）。

md 以下，首页导航收进右上角的菜单按钮（`src/components/navbar.tsx`）：打开期间页面锁住滚动、面板下方铺遮罩，Tab 只在菜单里循环，读屏软件也读不到遮罩下面的页面；Esc 或点遮罩关闭，焦点回到菜单按钮；窗口拉宽到 md 起菜单自动收起。面板里每一行的焦点框画在行里面，不会被面板裁掉（美食五档榜单导览和账户中心菜单同样，共用 `src/lib/navigation/scroller-focus.ts`）。规则见 [`DESIGN_SYSTEM.md`](../../docs/product/DESIGN_SYSTEM.md) 的“首页手机菜单”。

首屏入口、互助开放状态和手机资料查找体验的范围见 [Issue #482](https://github.com/jry21223/HENU-Kit-DEV/issues/482)。

### 刷题 `/practice`
| 路由 | 说明 |
|---|---|
| `/practice` | 题库总览 |
| `/practice/lists/[id]` | 旧题单入口，重定向至题库总览 |
| `/practice/quiz` | 刷题模式 |
| `/practice/leaderboard` | 排行榜 |
| `/practice/stats` | 数据面板 |

题库页 Hero 右侧是按掌握度生成的知识点结构图（桌面 WebGL 3D，减少动态时为静态图）；lg 以下隐藏这块图并压缩标题区，手机首屏留给搜索框和第一组题库（[#542](https://github.com/jry21223/HENU-Kit-DEV/issues/542)），各题库掌握度在 `/practice/stats` 查看。页面间使用形变过渡系统导航。

### 美食 `/food`
| 路由 | 说明 |
|---|---|
| `/food` | 五档榜单（从夯到拉，可按校区筛选） |
| `/food/campus/[campus]` | 旧校区入口，重定向至 `/food` |
| `/food/post/[id]` | 美食投稿详情 |
| `/food/publish` | 发布投稿（需登录） |
| `/food/leaderboard` | 旧榜单入口，重定向至 `/food` |

投稿详情只展示投稿里有的信息：价格、营业时间没填就不占格子；没有图片时，“图片与环境”折叠成一行说明，不留大块占位。

### 互助 `/campus`
| 路由 | 说明 |
|---|---|
| `/campus` | 市集（搜索 + “单子类型”“分类”两组带组名的筛选，瀑布流布局） |
| `/campus/item/[id]` | 互助与闲置内容详情 |
| `/campus/publish` | 发布 / 编辑入口（尚未开放提交） |
| `/campus/deals` | 交易说明（尚未开放交易） |

互助当前提供内容浏览，发布、接单和结算尚未开放。页面说明与这一状态保持一致，不承诺实名验证、保证接单或完整交易服务；内容加载中、成功但无内容和加载失败分别呈现。

与资料库做法一致，lg 以下压缩顶部标题区、隐藏装饰插图，手机首屏能看到搜索框和第一条内容（[#542](https://github.com/jry21223/HENU-Kit-DEV/issues/542)）。

### 资料库 `/library`
| 路由 | 说明 |
|---|---|
| `/library` | 公开免费资料目录（搜索、类型与科目筛选） |
| `/library/item/[id]` | 资料详情、原始标题与下载入口 |
| `/library/read/[id]` | 旧阅读入口，重定向至资料详情 |
| `/library/slides/[id]` | 旧幻灯片入口，重定向至资料详情 |
| `/library/shelf` | 我的书架（暂未开放） |

手机端优先显示搜索和科目、类型筛选，压缩顶部装饰内容；桌面保留图纸风格。资料卡分层展示课程、类型和易读标题，详情以“原始标题”展示来源提供的完整标题；详情左侧封面只标类型与科目，标题只在右侧出现：H1 是易读标题，“原始标题”一行是完整标题。当前资料接口没有原始文件名，不能把原始标题当作文件名。

易读标题（`src/lib/library/material-title.ts`）去掉与课程、类型完全一致的前缀，并把分隔用的下划线显示为间隔号（`2020级 · 第1章 · 扫描版`）；两个 ASCII 字母或数字之间的下划线属于标识符（如 `primary_key`），原样保留。易读标题仅由 Portal 用于展示，不改写来源标题、实际文件或下载地址。

公开目录只收免费资料：契约里 `price` 恒为 0，Portal Gateway 也固定返回 0（ADR-0027）。因此资料卡不逐张标“免费”，筛选栏没有价格一组；收藏上线前，详情页也不放“即将上线”的占位按钮。搜索匹配原始标题、卡片上显示的易读标题与课程；筛选只改变可见资料，不改变 Library 提供的全目录收录数量和累计 Download Start 统计。资料仅提供既有同源下载入口，不提供在线阅读或试读；加载、真实空列表与失败状态保留各自含义。

### 求职雷达 `/career`
| 路由 | 说明 |
|---|---|
| `/career` | 求职雷达，按登录、会员和求职资料状态展示对应入口 |
| `/career/history` | 求职扫描历史 |

### 账号 `/account`
| 路由 | 说明 |
|---|---|
| `/account/login` | 登录 / 注册 |
| `/account/recover` | 找回密码 |
| `/account` | 控制台概览（积分、会员、通知、工单四张卡片分别进入对应页面） |
| `/account/security` | 安全设置 |
| `/account/wallet` | 积分钱包 |
| `/account/membership` | 会员状态 |
| `/account/tickets` | 工单 |
| `/account/notifications` | 通知 |

账户控制台只使用 Portal Gateway 建立的 HttpOnly 会话；概览、积分、会员、通知和工单均从真实 Account Portfolio 接口读取。服务不可用时显示可恢复错误，绝不以 localStorage、会话 mock 或示例数据伪造成功状态；文章和交易入口尚未交付，故不在账户导航中暴露。

登录 / 注册表单的字段错误出现时作为 `role="alert"` 读出，并用 `aria-describedby` 与 `aria-invalid` 挂在对应输入框上，回到输入框时错误跟着读；“验证码已进入发送队列”一句放在常驻的 `role="status"` 区里，发出后读屏软件会读出。

### 协议与页脚
| 路由 | 说明 |
|---|---|
| `/privacy` | 隐私政策 |
| `/terms` | 用户协议 |

页脚包含短版非官方声明、用户协议与隐私政策链接，以及 ICP 备案号：
- 首页用自己的大页脚；
- 子站、账户中心、登录页、QQ 绑定页（`/bind/qq`）与协议页的布局都套用 `SiteShell`，由它渲染 `SiteFooter`；
- 两种页脚渲染同一个 `LegalNotice`；
- 404 与出错兜底页不经过子站布局，由它们共用的 `FallbackPage` 自己套上 `SiteShell`（见下节）。

声明、链接、备案号和协议更新日期只在 `src/lib/site-legal.ts` 配置一处；备案号须与工信部备案系统中的记录逐字一致，并链接到 beian.miit.gov.cn。登录、注册与终身会员购买在操作前用 `LegalConsent` 再次说明主体，并告知继续即表示同意两份协议。

协议正文逐条对应系统当前的数据处理。字段、有效期、服务方或存储位置变化时，先改协议再上线，并更新 `LEGAL_UPDATED_AT`。

### 404 与出错兜底
- 没有对应页面的地址返回 404，显示“页面不存在”（`src/app/not-found.tsx`），提供回首页和资料库、智能刷题、美食榜三个常用入口。
- 页面渲染出错时显示“页面出错了”（`src/app/error.tsx`；根布局本身出错时为 `src/app/global-error.tsx`），提供“重试”与“回首页”，不展示错误原文、digest 或堆栈。
- `global-error.tsx` 替换根布局，所以自带 `<html>`、全局样式和字体；字体实例定义在 `src/app/fonts.ts`，与根布局共用。

### 数据按需加载
- 根布局不预取任何模块数据：每个页面只在进入时请求自己要用的数据，登录页不请求资料库、美食、互助或求职数据（[#546](https://github.com/jry21223/HENU-Kit-DEV/issues/546)）。
- 全量资料目录经 `loadLibraryMaterials`（`src/lib/library/gateway.ts`）共享：首页资料库区块与资料详情的“相关资料”共用同一次请求，本次页面会话内缓存；失败不缓存，也不在背后重试：首页区块由用户点“重试”，详情页只是不展示“相关资料”，详情本身照常。`/library` 列表每次进入都实时读取（收录与下载统计要最新），读到后写入这份缓存，从列表点进详情不再下载一遍。
- 互助列表读到的单子同样写入共享缓存，从列表点进详情时，详情读取失败可回退到列表里的这条单子。
- 求职数据只在已登录后、进入求职雷达或账户的求职资料页时读取（`/career` 先确认会员）；未登录访问任何页面都不请求。
- `tests/module-data-requests.spec.ts` 记录各页面发出的 `/api/v1/*` 请求来检查以上约定。

### 数据加载失败
- 接口失败时，页面只展示中文提示：说明发生了什么、可以怎么做，不展示接口路径、HTTP 状态文本或内部组件名。提示统一由 `formatPortalError`（`src/lib/api/client.ts`）按错误类别映射：网络失败、需要登录、服务不可用（含非 JSON 响应，例如网关错误页或 WAF 挑战页）。错误对象的原始 message 只用于排查，不上屏。
- 需要特定提示的流程（每日投稿上限、终身会员门、支付通道未开放、工单版本冲突等）先按 status / errorCode 分支，再用自己的文案，不再叠一句通用提示：支付通道未开放时只说通道尚未开放、这次没有创建订单也不会扣款，以及开放后可回到本页开通。
- QQ 绑定页（`/bind/qq`）自己发请求、不经过 `formatPortalError`，同样只展示中文、原始报错不上屏：断网时显示“网络连接失败，请检查网络后重试。”，回来的不是绑定服务的 JSON（网关错误页、WAF 挑战页）时显示“绑定服务暂时不可用，请稍后重试。”（读登录状态时为“登录状态暂时无法读取，请稍后刷新重试。”）；绑定服务自己返回的中文 `error.message` 原样展示。
- 列表加载失败时只由 `ErrorBanner` 说一次，列表区不再叠一句空状态，筛选行的英文进度标签（美食榜的 `SYNCING`）也不再挂着（资料库、互助、美食榜、题库一致）。
- 详情页分清“不存在”和“暂时读不到”（资料、美食、互助单详情一致）：接口回 404 时只显示 404 页，不叠一句错误；服务不可用、回来的不是 JSON 或断网时显示 `ErrorBanner` 与“重试”，不说内容不存在。互助单详情先回退到列表缓存里的这条单子，回退不到才进入这两种状态。
- `ErrorBanner` 只展示一条主信息和“重试”。有请求编号时（`portalErrorRequestId`）显示为“错误编号”，方便用户提交工单时附上；目前首页资料库区块和 `/library` 会传入请求编号。

### 空状态
- 加载中（`LoadingBlock`）、真实为空（`EmptyBlock`）、加载失败（`ErrorBanner`）分别呈现，中文文案后面不追加 ` / EMPTY`、` / LOADING` 这类英文后缀；资料详情和互助单详情的加载中同样只写“加载中…”。单独使用的英文等宽状态标签（如账户控制台的 `AUTH CHECK…`）属于图纸风格的视觉语言，照常保留。
- `EmptyBlock` 说明为什么没有内容，并可带一个下一步 `action`：去别处用链接，就地改条件用按钮。
- `LoadingBlock` 与 `EmptyBlock` 的说明带 `role="status"`（礼貌播报），列表换成加载中或空状态时焦点不用挪；`EmptyBlock` 的下一步不在播报里。这个 status 跟着内容一起插进页面，支持的读屏软件会读出，但并非所有读屏软件都会读；一定要读出的结果放在常驻的 status 区里（如登录页的“验证码已进入发送队列”）。详情页里随整页一起出现的固定段落占位（美食详情的“暂无学生补充”等）不是结果变化，用 `announce={false}` 不播报。调用方不要再套一层 `aria-live`，否则会读两遍。
- 收藏夹为空时给“去题库”（收藏夹总览和单个题库的收藏夹都是）；学习数据未开放或还没有记录、排行榜未开放或还没有人上榜时给“去刷题”；资料库与互助平台有搜索或筛选条件却没有结果时给“清除筛选”，恢复列表并把搜索框和筛选控件复位；题库目录搜索不到题库时给“清除搜索”；按钮随空状态消失，焦点交给搜索与筛选区（不直接聚焦搜索框，免得手机弹出输入法）。没有任何条件的真实空列表不显示“清除筛选”；互助平台一条单子都没有时（有没有筛选条件都一样）给“回首页”，不承诺发布。

### 用户可见文案
- 从用户视角写：说“解析”“答对 / 答错”“暂未开放”“同科目或同类型的资料”，不写“服务端”“接口”“持久化”、内部服务或组件名（如 Account Portfolio、Redis）、实现方式（异步、受控来源）或面向评审的承诺（如“不会以本地数据替代真实记录”）。功能未就绪时照样不展示示例数据，只是换成用户能看懂的说明。
- `src/app/public-copy.test.ts` 用 TypeScript 语法树读取 `src/` 下非测试源码的 JSX 文本和字符串 / 模板字面量（注释和标识符不算），`public/llms.txt` 整份按文本检查，出现 `INTERNAL_TERMS` 里的词即失败；发现新的同类措辞时加进这份清单。

## 设计系统

**工业极简（Industrial Minimal）** 视觉风格。颜色来自 `packages/design-tokens/tokens.json`：`scripts/generate-theme.mjs` 按它生成 Tailwind 颜色主题 `src/app/theme.css`，`globals.css` 引入这份主题，自己不另写色值（[#536](https://github.com/jry21223/HENU-Kit-DEV/issues/536)）。主题里是算好的色值而不是 `var(--hk-*)`：Tailwind 只有拿到字面色值，才能给 `bg-accent/5`、`text-ink/60` 这类透明度写法预先算出回退，不支持 `color-mix()` 的浏览器才不会拿到实心的橙或墨。改色值时改 `tokens.json` 与 `tokens.css`，再运行 `pnpm --filter @henukit/portal generate:theme`。

| 令牌 | 来自 | 值 | 用途 |
|---|---|---|---|
| `--color-paper` | `surface.paper` | `#F2F0EA` | 主背景（暖纸白） |
| `--color-ink` | `text.primary` | `#161513` | 主文字 / 深色区块 |
| `--color-accent` | `brand.accent` | `#FF4D00` | 安全橙：色块（CTA / 激活态 / 反馈）、墨色底上的文字 |
| `--color-accent-text` | `brand.accent_text` | `#BB3800` | 浅色底上的橙字，不论大小（编号、眉标、链接、大号数字），类名 `text-accent-text` |
| `--color-easy` | `semantic.success` | `#3E7C4F` | 难度 < 4.0 |
| `--color-mid` | `semantic.warning` | `#C79A2A` | 难度 4.0–6.9 |
| `--color-hard` | `semantic.danger` | `#C2401F` | 难度 ≥ 7.0 |
| `--container-site` | — | `1440px` | 内容框宽度（`max-w-site`），首页、子站页头与正文共用 |

强调橙色块上的文字用墨色，不用纸白；规则见 [`DESIGN_SYSTEM.md`](../../docs/product/DESIGN_SYSTEM.md) 的“文字配色”。`src/app/design-tokens.test.ts` 按 `tokens.json` 检查文字配色的对比度，并检查 `theme.css` 与 `tokens.json` 一致、`globals.css` 不另写 token 里已有的色值、透明度写法在生产构建里有算好的回退、选中文字是橙底墨色字；`tests/readability.spec.ts` 检查首页跑马灯是橙底墨色字。

灰字（`text-ink/NN`）下限 `ink/60`，叠在 5% 色块上（`hover:bg-ink/5`、`bg-accent/5`、首页半透明页头）下限 `ink/65`；墨色底上的纸白字下限 `paper/50`；占位文字同样按这条线。大字（≥24px，或 ≥18.66px 粗体）只要 3:1，首页美食榜的名次用 `ink/50`。更浅的颜色只留给加了 `aria-hidden` 的纯装饰，禁用态控件不受限制。`tests/color-contrast.spec.ts` 用 axe（`@axe-core/playwright`）的 `color-contrast` 规则扫首页每一屏、五个子站首页和登录页，1440 与 390 下都应为 0；扫描前去掉工程图纸网格和读屏隐藏的装饰，文字按真正压着的底色检查。同一个 spec 还在 1440 下悬停磁吸按钮、墨色主按钮、五档导览格子和榜单链接后再扫一次。它跑在题库目录关闭的默认 dev server 上；目录开启时的 /practice（题库卡片与加载失败提示）由 `tests/quizcraft-catalog.spec.ts` 检查（脚本 `test:e2e:quizcraft-catalog`，部署流水线目前不跑这一组）。两处共用 `tests/support/color-contrast.ts`。

字号不小于 12px（`text-xs`），小于 12px 的只留给加了 `aria-hidden` 的纯装饰拉丁标签（最小 10px）。宽字距只加在拉丁 / 等宽文本上，中文用 `tracking-normal`；中英混排的标签把拉丁部分拆进单独的 span，只给它加字距，如 `<span className="tracking-widest">01</span>资料库`。规则见 [`DESIGN_SYSTEM.md`](../../docs/product/DESIGN_SYSTEM.md) 第 4 节。`src/app/typography.test.ts` 按源码检查写死的字号类和文字；`tests/typography.spec.ts` 在首页、五个子站首页和登录页上按计算样式检查，1440 与 390 下都应为 0；题库目录开启时的 /practice 由 `tests/quizcraft-catalog.spec.ts` 检查。`tests/typography.spec.ts` 与 `tests/color-contrast.spec.ts` 打开同一组页面、用同一份网关 mock，都来自 `tests/support/readability-routes.ts`。

语言元素：1px 结构线、十字对位标记、mono 编号、工程图纸网格、大小字强对比排版。

## 动效约定

- GSAP 动画统一经 `useGSAP`，卸载时 `killTweensOf` 清理。
- 页面间导航：形变过渡系统（共享元素形变 + 塌缩/展开）。
- `prefers-reduced-motion`：瞬时导航，循环/揭示动画静止。
- 首屏入场只让内容越来越可见，服务端已画出的内容不在水合后隐藏重播（[#537](https://github.com/jry21223/HENU-Kit-DEV/issues/537)）。首页、子站与题库 Hero 用 `globals.css` 的 `enter-*` CSS keyframes，首帧即开始播放、不等水合，脚本没加载也停在可见态；不要对服务端已画出的内容用 GSAP `from()`。`[data-enter]` 内容块由 `useReveal` 揭示：水合时已画出的块直接显示，之后只揭示客户端新挂上的块。慢 CPU 下由 `tests/first-screen-entrance.spec.ts` 逐帧检查。
- 滚动入场：统一 `start: "top 60%"`。
- mock 数据为固定数据，图片使用 picsum 种子外链，SSR 与客户端输出一致；生产构建不预渲染任何 mock 页面（`npm run build` 会检查）。
- 图片统一用 `components/ui/img.tsx`：默认懒加载、异步解码，首屏关键图由调用方传 `loading="eager"`（主图再加 `fetchPriority="high"`）；调用方用固定宽高或 `aspect-ratio` 占位，加载时不挤动版面（[#548](https://github.com/jry21223/HENU-Kit-DEV/issues/548)，见 `docs/product/DESIGN_SYSTEM.md` §13）。

## 项目结构

```
src/
├── app/                    # 路由页面
│   ├── page.tsx            # 首页
│   ├── (legal)/            # 隐私政策与用户协议
│   ├── account/            # 账号子站
│   ├── bind/               # QQ 绑定页
│   ├── campus/             # 互助子站
│   ├── career/             # 求职雷达子站
│   ├── food/               # 美食子站
│   ├── library/            # 资料库子站
│   ├── practice/           # 刷题子站
│   ├── globals.css         # Tailwind 主题 + 全局样式
│   └── theme.css           # 由 tokens.json 生成的颜色主题（不要手改）
├── components/             # 组件
│   ├── practice/           # 刷题相关组件 + 形变过渡系统
│   ├── site-hero/          # 子站 hero 统一骨架
│   └── ui/                 # 通用 UI 组件
└── lib/                    # 工具库与 mock 数据
    ├── auth/               # 认证 store
    ├── campus/             # 互助 mock
    ├── food/               # 美食 mock
    ├── library/            # 资料库 mock
    └── practice/           # 刷题会话与统计工具
```
