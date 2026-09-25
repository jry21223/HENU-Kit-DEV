# henukit — Keep In Touch

面向校园的综合性学生平台，集成资料库、刷题、美食、互助、求职雷达五个模块。

最后更新：2026-09-08。

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

浏览器打开 [http://localhost:3000](http://localhost:3000)。

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
- 页面标题只有一种格式，由 `src/lib/seo.ts` 的 `pageTitle` 生成：模块内页 `页面名 — 模块名 | HENU Kit`，模块首页与不属于模块的页面 `页面名 | HENU Kit`；首页保留站点标题 `HENU Kit — 河南大学校园工具`。品牌只由标题模板补一次：布局一旦写了纯字符串标题，Next 就不再把根布局的模板传给更深的页面，所以模块 `layout.tsx` 用 `moduleLayoutTitle` 导出标题，把模板接着传下去，不要写纯字符串标题。客户端页面的标题由同一段的服务端 `layout.tsx` 导出；详情页的内容在客户端到达后，由 `useDocumentTitle` 把内容名补进标题。
- canonical origin 由构建变量 `NEXT_PUBLIC_SITE_URL` 决定，默认 `https://henukit.cn`；变量必须是无路径、查询或片段的 HTTP(S) origin。

站长平台提交、WAF 验证、内容版本规则和验收方法见 [`../../docs/product/seo-geo.md`](../../docs/product/seo-geo.md)。

## 子站路由

### 首页 `/`
首屏提供“找资料”“开始刷题”“看岗位”，分别进入 `/library`、`/practice`、`/career`；下方介绍与导航对应的五个模块。保留米白、网格、墨黑和橙色视觉，以及 md+ 视口由 GSAP Observer 接管的吸附滚动和 WebGL 3D 场景。

首屏入口、互助开放状态和手机资料查找体验的范围见 [Issue #482](https://github.com/jry21223/HENU-Kit-DEV/issues/482)。

### 刷题 `/practice`
| 路由 | 说明 |
|---|---|
| `/practice` | 题库总览 |
| `/practice/lists/[id]` | 旧题单入口，重定向至题库总览 |
| `/practice/quiz` | 刷题模式 |
| `/practice/leaderboard` | 排行榜 |
| `/practice/stats` | 数据面板 |

题库页 Hero 含 WebGL 3D 石膏像，页面间使用形变过渡系统导航。

### 美食 `/food`
| 路由 | 说明 |
|---|---|
| `/food` | 三校区入口（明伦 / 金明 / 龙子湖） |
| `/food/campus/[campus]` | 校区美食列表 |
| `/food/post/[id]` | 美食投稿详情 |
| `/food/publish` | 发布 / 编辑（需登录） |
| `/food/leaderboard` | 必吃排行榜 |

### 互助 `/campus`
| 路由 | 说明 |
|---|---|
| `/campus` | 市集（搜索 + 6 分类 + 求助/闲置双类型，瀑布流布局） |
| `/campus/item/[id]` | 互助与闲置内容详情 |
| `/campus/publish` | 发布 / 编辑入口（尚未开放提交） |
| `/campus/deals` | 交易说明（尚未开放交易） |

互助当前提供内容浏览，发布、接单和结算尚未开放。页面说明与这一状态保持一致，不承诺实名验证、保证接单或完整交易服务；内容加载中、成功但无内容和加载失败分别呈现。

### 资料库 `/library`
| 路由 | 说明 |
|---|---|
| `/library` | 公开免费资料目录（搜索、类型与科目筛选） |
| `/library/item/[id]` | 资料详情、原始标题与下载入口 |
| `/library/read/[id]` | 旧阅读入口，重定向至资料详情 |
| `/library/slides/[id]` | 旧幻灯片入口，重定向至资料详情 |
| `/library/shelf` | 我的书架（需登录） |

手机端优先显示搜索和科目、类型筛选，压缩顶部装饰内容；桌面保留图纸风格。资料卡分层展示课程、类型和易读标题，详情以“原始标题”展示来源提供的完整标题。当前资料接口没有原始文件名，不能把原始标题当作文件名。

易读标题仅由 Portal 用于展示，不改写来源标题、实际文件或下载地址。搜索仍匹配原始标题与课程；筛选只改变可见资料，不改变 Library 提供的全目录收录数量和累计 Download Start 统计。资料仅提供既有同源下载入口，不提供在线阅读或试读；加载、真实空列表与失败状态保留各自含义。

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
| `/account` | 控制台概览 |
| `/account/security` | 安全设置 |
| `/account/wallet` | 积分钱包 |
| `/account/membership` | 会员状态 |
| `/account/tickets` | 工单 |
| `/account/notifications` | 通知 |

账户控制台只使用 Portal Gateway 建立的 HttpOnly 会话；概览、积分、会员、通知和工单均从真实 Account Portfolio 接口读取。服务不可用时显示可恢复错误，绝不以 localStorage、会话 mock 或示例数据伪造成功状态；文章和交易入口尚未交付，故不在账户导航中暴露。

### 协议与页脚
| 路由 | 说明 |
|---|---|
| `/privacy` | 隐私政策 |
| `/terms` | 用户协议 |

页脚包含短版非官方声明、用户协议与隐私政策链接，以及 ICP 备案号：
- 首页用自己的大页脚；
- 子站、账户中心、登录页与协议页的布局都套用 `SiteShell`，由它渲染 `SiteFooter`；
- 两种页脚渲染同一个 `LegalNotice`；
- 404 与出错兜底页不经过子站布局，由它们共用的 `FallbackPage` 自己套上 `SiteShell`（见下节）。

声明、链接、备案号和协议更新日期只在 `src/lib/site-legal.ts` 配置一处；备案号确认之前不显示。登录、注册与终身会员购买在操作前用 `LegalConsent` 再次说明主体，并告知继续即表示同意两份协议。

协议正文逐条对应系统当前的数据处理。字段、有效期、服务方或存储位置变化时，先改协议再上线，并更新 `LEGAL_UPDATED_AT`。

### 404 与出错兜底
- 没有对应页面的地址返回 404，显示“页面不存在”（`src/app/not-found.tsx`），提供回首页和资料库、智能刷题、美食榜三个常用入口。
- 页面渲染出错时显示“页面出错了”（`src/app/error.tsx`；根布局本身出错时为 `src/app/global-error.tsx`），提供“重试”与“回首页”，不展示错误原文、digest 或堆栈。
- `global-error.tsx` 替换根布局，所以自带 `<html>`、全局样式和字体；字体实例定义在 `src/app/fonts.ts`，与根布局共用。

### 数据加载失败
- 接口失败时，页面只展示中文提示：说明发生了什么、可以怎么做，不展示接口路径、HTTP 状态文本或内部组件名。提示统一由 `formatPortalError`（`src/lib/api/client.ts`）按错误类别映射：网络失败、需要登录、服务不可用（含非 JSON 响应，例如网关错误页或 WAF 挑战页）。错误对象的原始 message 只用于排查，不上屏。
- 需要特定提示的流程（每日投稿上限、终身会员门、工单版本冲突等）先按 status / errorCode 分支，再用自己的文案。
- `ErrorBanner` 只展示一条主信息和“重试”。有请求编号时（`portalErrorRequestId`）显示为“错误编号”，方便用户提交工单时附上；目前首页资料库区块和 `/library` 会传入请求编号。

### 空状态
- 加载中（`LoadingBlock`）、真实为空（`EmptyBlock`）、加载失败（`ErrorBanner`）分别呈现，文案只用中文，不追加英文标记；资料详情、阅读页和互助单详情的加载中同样只写“加载中…”。
- `EmptyBlock` 说明为什么没有内容，并可带一个下一步 `action`：去别处用链接，就地改条件用按钮。
- 收藏夹为空时给“去题库”；学习数据未开放或还没有记录、排行榜未开放或还没有人上榜时给“去刷题”；资料库与互助平台有搜索或筛选条件却没有结果时给“清除筛选”，恢复列表并把搜索框和筛选控件复位；按钮随空状态消失，焦点交给搜索与筛选区（不直接聚焦搜索框，免得手机弹出输入法）。没有任何条件的真实空列表不显示“清除筛选”。

### 用户可见文案
- 从用户视角写：说“解析”“答对 / 答错”“暂未开放”“同科目或同类型的资料”，不写“服务端”“接口”“持久化”、内部服务名（如 Account Portfolio）、实现方式（异步、受控来源）或面向评审的承诺（如“不会以本地数据替代真实记录”）。功能未就绪时照样不展示示例数据，只是换成用户能看懂的说明。
- `src/app/public-copy.test.ts` 用 TypeScript 语法树读取 `src/` 下非测试源码的 JSX 文本和字符串 / 模板字面量（注释和标识符不算），`public/llms.txt` 整份按文本检查，出现 `INTERNAL_TERMS` 里的词即失败；发现新的同类措辞时加进这份清单。

## 设计系统

**工业极简（Industrial Minimal）** 视觉风格：

| 令牌 | 值 | 用途 |
|---|---|---|
| `--color-paper` | `#F2F0EA` | 主背景（暖纸白） |
| `--color-ink` | `#161513` | 主文字 / 深色区块 |
| `--color-accent` | `#FF4D00` | 安全橙（仅 CTA / 激活态 / 反馈） |
| `--color-easy` | `#3E7C4F` | 难度 < 4.0 |
| `--color-mid` | `#C79A2A` | 难度 4.0–6.9 |
| `--color-hard` | `#C2401F` | 难度 ≥ 7.0 |

语言元素：1px 结构线、十字对位标记、mono 编号、工程图纸网格、大小字强对比排版。

## 动效约定

- GSAP 动画统一经 `useGSAP`，卸载时 `killTweensOf` 清理。
- 页面间导航：形变过渡系统（共享元素形变 + 塌缩/展开）。
- `prefers-reduced-motion`：瞬时导航，循环/揭示动画静止。
- 滚动入场：统一 `start: "top 60%"`。
- mock 数据为固定数据，图片使用 picsum 种子外链，SSR 与客户端输出一致；生产构建不预渲染任何 mock 页面（`npm run build` 会检查）。

## 项目结构

```
src/
├── app/                    # 路由页面
│   ├── (home)/             # 首页
│   ├── account/            # 账号子站
│   ├── campus/             # 互助子站
│   ├── career/             # 求职雷达子站
│   ├── food/               # 美食子站
│   ├── library/            # 资料库子站
│   └── practice/           # 刷题子站
├── components/             # 组件
│   ├── practice/           # 刷题相关组件 + 形变过渡系统
│   ├── site-hero/          # 子站 hero 统一骨架
│   └── ui/                 # 通用 UI 组件
├── lib/                    # 工具库与 mock 数据
│   ├── auth/               # 认证 store
│   ├── campus/             # 互助 mock
│   ├── food/               # 美食 mock
│   ├── library/            # 资料库 mock
│   └── practice/           # 刷题会话与统计工具
└── globals.css             # 设计令牌 + 全局样式
```
