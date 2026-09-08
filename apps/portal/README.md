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
- `/sitemap.xml` 只列出构建时确定存在的公开入口，不伪造动态详情 URL。
- `/llms.txt` 提供项目定位、公开入口、非官方边界与引用规则。
- 根页面提供 canonical、Open Graph、Twitter Card 与 `WebSite`/社区维护者 JSON-LD。
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
| `/practice/lists/[id]` | 题单详情 |
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
| `/account/recover` | 找回密码（演示验证码 `427819`） |
| `/account` | 控制台概览 |
| `/account/security` | 安全设置 |
| `/account/wallet` | 积分钱包 |
| `/account/membership` | 会员状态 |
| `/account/tickets` | 工单 |
| `/account/notifications` | 通知 |

账户控制台只使用 Portal Gateway 建立的 HttpOnly 会话；概览、积分、会员、通知和工单均从真实 Account Portfolio 接口读取。服务不可用时显示可恢复错误，绝不以 localStorage、会话 mock 或示例数据伪造成功状态；文章和交易入口尚未交付，故不在账户导航中暴露。

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
- 所有 mock 数据使用种子化伪随机（mulberry32），SSR 与客户端输出一致。

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
│   └── practice/           # 刷题 mock
└── globals.css             # 设计令牌 + 全局样式
```
