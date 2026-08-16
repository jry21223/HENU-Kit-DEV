import type { MetadataRoute } from "next";

import { absoluteSiteUrl } from "../lib/seo";

const PUBLIC_ROUTES = [
  { path: "/", changeFrequency: "weekly", priority: 1 },
  { path: "/library", changeFrequency: "daily", priority: 0.9 },
  { path: "/practice", changeFrequency: "weekly", priority: 0.9 },
  { path: "/food", changeFrequency: "daily", priority: 0.8 },
  { path: "/campus", changeFrequency: "daily", priority: 0.8 },
  { path: "/career", changeFrequency: "weekly", priority: 0.7 },
] as const;

export default function sitemap(): MetadataRoute.Sitemap {
  return PUBLIC_ROUTES.map(({ path, changeFrequency, priority }) => ({
    url: absoluteSiteUrl(path),
    changeFrequency,
    priority,
  }));
}
