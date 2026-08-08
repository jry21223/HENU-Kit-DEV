import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Required for multi-stage Docker image (apps/portal/Dockerfile).
  output: "standalone",
  // Playwright reaches the local dev server through 127.0.0.1.  Permit that
  // same-origin HMR connection so browser acceptance tests exercise hydrated
  // client state instead of the SSR-only auth shell.
  allowedDevOrigins: ["127.0.0.1"],
  async headers() {
    const noIndexHeaders = [
      { key: "X-Robots-Tag", value: "noindex, nofollow" },
    ];

    return [
      "/account/:path*",
      "/campus/deals",
      "/campus/publish",
      "/food/publish",
      "/library/read/:path*",
      "/library/shelf",
      "/practice/favorites/:path*",
      "/practice/quiz",
      "/practice/stats",
    ].map((source) => ({ source, headers: noIndexHeaders }));
  },
  async redirects() {
    return [
      // The five-tier board is now the Food home page. Keep the previous
      // board and per-campus URLs reachable so existing links and bookmarks
      // land on the board instead of a 404.
      { source: "/food/leaderboard", destination: "/food", permanent: true },
      { source: "/food/campus/:campus", destination: "/food", permanent: true },
    ];
  },
};

export default nextConfig;
