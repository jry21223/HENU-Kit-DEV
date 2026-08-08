import type { MetadataRoute } from "next";

import { absoluteSiteUrl, siteOrigin } from "../lib/seo";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: "/api/",
    },
    host: siteOrigin(),
    sitemap: absoluteSiteUrl("/sitemap.xml"),
  };
}
