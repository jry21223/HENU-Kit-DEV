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
