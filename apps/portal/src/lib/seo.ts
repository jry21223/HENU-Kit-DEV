import type { Metadata } from "next";

export const DEFAULT_SITE_ORIGIN = "https://henukit.cn";
export const SITE_NAME = "HENU Kit";
export const SITE_TITLE = "HENU Kit — 河南大学校园工具";
export const SITE_DESCRIPTION =
  "HENU Kit 是学生自主运营的非官方河南大学校园工具，提供资料库、智能刷题、美食榜、校园互助和求职雷达入口；信息以河南大学及各学院官方来源为准。";

const TITLE_TEMPLATE = `%s | ${SITE_NAME}`;

/** 标题里的模块名：用首页一级入口那套「模块名」，不是子站导航里的书库、榜单、市集。 */
const MODULE_TITLES = {
  library: "资料库",
  practice: "智能刷题",
  food: "美食榜",
  campus: "互助平台",
  career: "求职雷达",
  account: "账户中心",
} as const;

export type TitleModule = keyof typeof MODULE_TITLES;

/**
 * 全站唯一的标题格式：模块内页 `页面名 — 模块名`，模块首页和不属于任何模块的页面只写
 * 页面名。品牌由标题模板补上（根布局，以及用 moduleLayoutTitle 接着传模板的模块布局），
 * 这里不写，免得重复。
 */
export function pageTitle(page: string, module?: TitleModule): string {
  return module ? `${page} — ${MODULE_TITLES[module]}` : page;
}

/** 同一格式补上品牌后的完整标题：给 og/twitter 卡片，以及数据到达后才写的 document.title。 */
export function documentTitle(page: string, module?: TitleModule): string {
  // 用函数替换：内容名来自用户投稿，字符串替换会把其中的 `$$`、`$&` 等当成替换模式展开。
  return TITLE_TEMPLATE.replace("%s", () => pageTitle(page, module));
}

/**
 * 模块布局的标题：模块名兜底本模块里没有自己标题的页面。布局一旦写了标题，Next 就不再把
 * 根布局的模板传给更深的页面，所以同一个模板在这里接着往下传。
 */
export function moduleLayoutTitle(module: TitleModule): NonNullable<Metadata["title"]> {
  return { default: MODULE_TITLES[module], template: TITLE_TEMPLATE };
}

export function siteOrigin(
  configuredOrigin = process.env.NEXT_PUBLIC_SITE_URL
): string {
  if (!configuredOrigin) return DEFAULT_SITE_ORIGIN;

  const url = new URL(configuredOrigin);
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    (url.pathname !== "/" && url.pathname !== "")
  ) {
    throw new Error(
      "NEXT_PUBLIC_SITE_URL must be an HTTP(S) origin without credentials, path, query, or fragment"
    );
  }

  return url.origin;
}

export function absoluteSiteUrl(path = "/"): string {
  if (path === "/") return siteOrigin();
  return new URL(path, `${siteOrigin()}/`).toString();
}

const HOME_OPEN_GRAPH: NonNullable<Metadata["openGraph"]> = {
  type: "website",
  locale: "zh_CN",
  siteName: SITE_NAME,
  title: SITE_TITLE,
  description: SITE_DESCRIPTION,
};

export const siteMetadata: Metadata = {
  metadataBase: new URL(siteOrigin()),
  applicationName: SITE_NAME,
  title: {
    default: SITE_TITLE,
    template: TITLE_TEMPLATE,
  },
  description: SITE_DESCRIPTION,
  keywords: [
    "河南大学",
    "HENU",
    "校园工具",
    "学习资料",
    "智能刷题",
    "校园互助",
    "求职雷达",
  ],
  creator: "HENU Kit 社区维护者",
  publisher: "HENU Kit 社区维护者",
  category: "education",
  openGraph: HOME_OPEN_GRAPH,
  twitter: {
    card: "summary",
    title: SITE_TITLE,
    description: SITE_DESCRIPTION,
  },
};

export const homeMetadata: Metadata = {
  alternates: { canonical: "/" },
  openGraph: {
    ...HOME_OPEN_GRAPH,
    url: "/",
  },
};

function pageMetadata(path: string, title: string, description: string): Metadata {
  // Next.js does not apply the siteMetadata title template to og:title, so the
  // share cards take the full title from documentTitle to stay consistent with
  // the document title template.
  return {
    title,
    description,
    alternates: { canonical: path },
    openGraph: {
      ...HOME_OPEN_GRAPH,
      title: documentTitle(title),
      description,
      url: path,
    },
    twitter: {
      card: "summary",
      title: documentTitle(title),
      description,
    },
  };
}

export const libraryMetadata: Metadata = pageMetadata(
  "/library",
  MODULE_TITLES.library,
  "HENU Kit 资料库收录河南大学公开免费学习资料，可按科目和类型浏览、搜索与筛选；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const practiceMetadata: Metadata = pageMetadata(
  "/practice",
  MODULE_TITLES.practice,
  "HENU Kit 智能刷题可按科目搜索题库，提供随机、难题、章节、收藏四种练习；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const foodMetadata: Metadata = pageMetadata(
  "/food",
  MODULE_TITLES.food,
  "HENU Kit 美食榜是学生视角的河南大学校园美食五档榜单，档内按最新投稿展示，不接受充值，不接受公关；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const campusMetadata: Metadata = pageMetadata(
  "/campus",
  MODULE_TITLES.campus,
  "HENU Kit 互助平台可浏览同校互助与闲置信息；发布、接单和结算暂未开放。学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const careerMetadata: Metadata = pageMetadata(
  "/career",
  MODULE_TITLES.career,
  "HENU Kit 求职雷达设定求职画像后，在后台扫描已收录的官方招聘来源，匹配结果与命中原因一目了然；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const privacyMetadata: Metadata = pageMetadata(
  "/privacy",
  "隐私政策",
  "HENU Kit 隐私政策：说明资料库、刷题、美食榜、互助平台、求职雷达和账户中心收集哪些个人信息、如何使用和保护，以及你的权利。学生自主运营，非河南大学官方项目。"
);

export const termsMetadata: Metadata = pageMetadata(
  "/terms",
  "用户协议",
  "HENU Kit 用户协议：账户、使用规范、用户发布的内容、终身会员与支付等约定。学生自主运营，非河南大学官方项目。"
);

export function websiteStructuredData() {
  const origin = siteOrigin();

  return {
    "@context": "https://schema.org",
    "@graph": [
      {
        "@type": "Organization",
        "@id": `${origin}/#community`,
        name: "HENU Kit 社区维护者",
        url: origin,
        description: "学生自主运营的非官方社区维护团队，不代表河南大学或任何学院。",
      },
      {
        "@type": "WebSite",
        "@id": `${origin}/#website`,
        url: origin,
        name: SITE_NAME,
        alternateName: "河南大学校园工具",
        description: SITE_DESCRIPTION,
        inLanguage: "zh-CN",
        isAccessibleForFree: true,
        publisher: { "@id": `${origin}/#community` },
      },
    ],
  };
}
