import type { Metadata } from "next";

export const DEFAULT_SITE_ORIGIN = "https://henukit.cn";
export const SITE_NAME = "HENU Kit";
export const SITE_TITLE = "HENU Kit — 河南大学校园工具";
export const SITE_DESCRIPTION =
  "HENU Kit 是学生自主运营的非官方河南大学校园工具，提供资料库、智能刷题、美食榜、校园互助和求职雷达入口；信息以河南大学及各学院官方来源为准。";

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
    template: `%s | ${SITE_NAME}`,
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
  // `| SITE_NAME` suffix is repeated here to keep search snippets and share
  // cards consistent with the document title template.
  return {
    title,
    description,
    alternates: { canonical: path },
    openGraph: {
      ...HOME_OPEN_GRAPH,
      title: `${title} | ${SITE_NAME}`,
      description,
      url: path,
    },
    twitter: {
      card: "summary",
      title: `${title} | ${SITE_NAME}`,
      description,
    },
  };
}

export const libraryMetadata: Metadata = pageMetadata(
  "/library",
  "资料库",
  "HENU Kit 资料库收录河南大学公开免费学习资料，可按科目和类型浏览、搜索与筛选；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const practiceMetadata: Metadata = pageMetadata(
  "/practice",
  "智能刷题",
  "HENU Kit 智能刷题提供按学校、专业和科目组织的题库与题单，支持搜索与练习；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const foodMetadata: Metadata = pageMetadata(
  "/food",
  "美食榜",
  "HENU Kit 美食榜是学生视角的河南大学校园美食五档榜单，档内按点赞排序，不接受充值，不接受公关；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const campusMetadata: Metadata = pageMetadata(
  "/campus",
  "互助平台",
  "HENU Kit 互助平台提供同校互助市场：代取快递、搬行李、小项目、出闲置，发单有人接；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
);

export const careerMetadata: Metadata = pageMetadata(
  "/career",
  "求职雷达",
  "HENU Kit 求职雷达设定求职画像后，后台异步扫描受控招聘来源，匹配结果与命中原因一目了然；学生自主运营，非河南大学官方项目，信息以河南大学及各学院官方来源为准。"
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
