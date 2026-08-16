# HENU Kit Portal SEO / GEO 基础设施与内容治理

本文记录 `apps/portal` 当前已经实现的搜索发现契约，以及后续内容进入索引前必须满足的真实性边界。GEO 在这里指让公开内容更容易被回答型搜索检索和准确引用，不代表存在独立于基础搜索质量的排名开关。

## 当前实现

| 入口 | 当前行为 |
|---|---|
| `/robots.txt` | 允许公开 HTML 抓取，仅排除 API；非公开 HTML 必须可被爬虫读取 `X-Robots-Tag` |
| `X-Robots-Tag` | 账户、发布、交易、阅读器及个性化练习页面返回 `noindex, nofollow` |
| `/sitemap.xml` | 仅包含首页、资料库、刷题、美食榜、校园互助和求职雷达六个稳定入口 |
| `/llms.txt` | 声明项目定位、公开入口（含每个入口的一句话摘要）、非官方身份、官方来源优先与引用限制 |
| `/` HTML metadata | 提供 canonical、描述、Open Graph 和 Twitter Card |
| 顶层页面 HTML metadata | 资料库、刷题、美食榜、互助平台和求职雷达五个页面分别提供页面级 canonical、描述、Open Graph 和 Twitter Card |
| `/` JSON-LD | 使用 `WebSite` 和社区维护者 `Organization`；不把河南大学声明为发布者或关联组织 |

`OAI-SearchBot`、Googlebot、Bingbot 和 Baiduspider 均适用 `User-agent: *` 的公开抓取规则。`GPTBot` 是否用于模型训练与搜索曝光不是同一决策；本实现没有为训练型爬虫设置特殊授权。

## Canonical origin

Portal 构建读取：

```env
NEXT_PUBLIC_SITE_URL=https://henukit.cn
```

该值必须是一个 HTTP(S) origin，不得包含用户名、密码、路径、查询或片段。未配置时回退到仓库已确认的生产主站 `https://henukit.cn`。它是构建时值；域名变化后必须重建 Portal 制品，不能只重启容器。

当前首页和五个顶层页面（资料库、刷题、美食榜、互助平台、求职雷达）各发布自己的 canonical。六个页面全部是稳定的公开列表页；canonical 只挂在这些页面自己的 page 级 metadata 上，不写在父级 layout 上。详情页和动态内容仍是 Client Component 且依赖 owner 数据，在它们具备逐页、可验证的服务端 metadata 之前，不用父级 layout 批量写 canonical，避免把不同详情错误规范到同一个列表页。

## Sitemap 纳入门槛

路由只有同时满足以下条件才进入 sitemap：

1. 无需登录即可返回有意义的服务端 HTML；
2. URL 稳定，不依赖个人会话、写入状态或临时筛选参数；
3. 内容来自真实 owner，构建过程能得到完整且可验证的 URL 集合；
4. 页面明确展示来源、更新时间和纠错入口（适用时）；
5. 删除或替代内容有确定的重定向或状态策略。

动态资料、互助帖子和美食详情目前未在构建期提供完整 owner 清单，因此本 PR 不将它们写入静态 sitemap。账户、发布、交易、收藏、测验和阅读器路由不应进入索引。

## 非官方与来源边界

HENU Kit 必须始终描述为“学生自主运营的非官方项目”，不得暗示河南大学或任何学院发布、授权或背书本项目。涉及政策、资格、名额、截止时间和办事要求时：

- 优先链接并引用河南大学或相关学院原始官方来源；
- 不根据缺失信息推断条件、日期、例外或结论；
- 页面必须显示最后核验时间和原始来源；
- 官方原文与整理摘要冲突时，以官方原文为准并立即停止传播错误摘要。

## 政策版本语义

未来若交付经过核验的政策或文档详情，只允许使用以下状态：

| 状态 | 含义 | 索引要求 |
|---|---|---|
| `current` | 已由原始官方来源核验，且未发现更新版本 | 可索引，显示来源与最后核验时间 |
| `superseded` | 已被更新版本替代 | 页面保留历史用途，醒目标注并链接到新版本 |
| `historical` | 仅供历史查阅 | 明确说明不代表当前规则 |
| `unverified` | 尚未完成官方来源核验 | 不得作为确定结论；默认不进入 sitemap |

这些状态是内容发布门槛，不表示当前 Portal 已经拥有政策数据库。数据 owner、审核流、版本关系和公开 API 未落地前，不创建政策详情 URL 或结构化数据。

## 发布后人工验收

本 PR 不注册站长账号、不提交 URL、不改变 WAF 或生产环境。部署后由维护者执行：

1. 请求 `/robots.txt`、`/sitemap.xml`、`/llms.txt` 和首页，确认均为 200 且 canonical origin 正确；
2. 从外部网络分别以常规 User-Agent 与 `OAI-SearchBot` User-Agent 请求，确认 CDN/WAF 未返回挑战页或 403；
3. 在 Google Search Console、Bing Webmaster Tools 和百度搜索资源平台验证站点所有权并提交 sitemap；
4. 用结构化数据验证工具检查 `WebSite` 与维护者实体，不得出现河南大学官方发布者关系；
5. 记录有效收录页、非品牌查询曝光、自然搜索点击、引用页数量和回答型搜索引荐流量。

IndexNow、百度主动推送、动态详情 sitemap 和内容分析事件均不在本 PR 范围；它们需要真实内容 owner、密钥管理、重试与删除通知契约后单独交付。
