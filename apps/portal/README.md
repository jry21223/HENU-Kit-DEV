# henukit — Keep In Touch

面向校园的综合性学生平台，集成刷题、美食、互助、资料库四大子站。

## 技术栈

| 层级 | 技术 |
|---|---|
| 框架 | Next.js (App Router) |
| 语言 | TypeScript |
| 样式 | Tailwind CSS v4 |
| 动画 | GSAP (ScrollTrigger / Observer / ScrollTo) |
| 3D | Three.js + React Three Fiber（仅首页 Hero 与题库页 Hero） |
| 地图 | Leaflet + OSM（仅美食详情页） |
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

## 子站路由

### 首页 `/`
整屏模块展示，md+ 视口由 GSAP Observer 接管吸附滚动，WebGL 3D 场景。

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
| `/food/post/[id]` | 文章详情 + Leaflet 地图 |
| `/food/publish` | 发布 / 编辑（需登录） |
| `/food/leaderboard` | 必吃排行榜 |

### 互助 `/campus`
| 路由 | 说明 |
|---|---|
| `/campus` | 市集（搜索 + 6 分类 + 求助/闲置双类型，瀑布流布局） |
| `/campus/item/[id]` | 商品详情（卖家卡片 / 想要 / 留言 / 担保流程） |
| `/campus/publish` | 发布 / 编辑（需登录） |
| `/campus/deals` | 我的交易（需登录） |

订单状态：open 待接单 → ongoing 进行中 → done 已完成。担保流程五步：发布 → 赏金托管 → 接单服务 → 确认完成 → 平台结算。

### 资料库 `/library`
| 路由 | 说明 |
|---|---|
| `/library` | 书库（搜索 + 五类 + 免费/收费 + 科目筛选） |
| `/library/item/[id]` | 书籍详情 |
| `/library/read/[id]` | 阅读器（分页 / 键盘导航 / 进度条 / 试读锁定墙） |
| `/library/shelf` | 我的书架（需登录） |

三种形态：免费全文、收费标价、收费带试读 + 锁定墙。购买走账号积分系统。

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
