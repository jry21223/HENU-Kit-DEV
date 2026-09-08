import type { NextConfig } from "next";

function localPlaywrightOrigin(raw: string | undefined): string {
  if (!raw) return "";
  try {
    const value = new URL(raw);
    if (
      value.protocol !== "http:" ||
      value.hostname !== "127.0.0.1" ||
      value.username ||
      value.password ||
      value.pathname !== "/" ||
      value.search ||
      value.hash
    ) {
      return "";
    }
    return value.origin;
  } catch {
    return "";
  }
}

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
    const accountFormActions = ["'self'", "https://console.henukit.cn"];
    const playwrightConsoleOrigin = localPlaywrightOrigin(
      process.env.PLAYWRIGHT_CONSOLE_ORIGIN
    );
    if (playwrightConsoleOrigin) {
      accountFormActions.push(playwrightConsoleOrigin);
    }

    const accountHeaders = [
      ...noIndexHeaders,
      { key: "Cache-Control", value: "private, no-store, max-age=0" },
      { key: "Referrer-Policy", value: "no-referrer" },
      { key: "X-Content-Type-Options", value: "nosniff" },
      {
        key: "Content-Security-Policy",
        value: `base-uri 'self'; form-action ${accountFormActions.join(" ")}; frame-ancestors 'none'`,
      },
    ];

    return [
      { source: "/account/:path*", headers: accountHeaders },
      ...[
        "/campus/deals",
        "/campus/publish",
        "/food/publish",
        "/library/read/:path*",
        "/library/shelf",
        "/practice/favorites/:path*",
        "/practice/quiz",
        "/practice/stats",
      ].map((source) => ({ source, headers: noIndexHeaders })),
    ];
  },
  async rewrites() {
    const accountAuthFixture = process.env.PLAYWRIGHT_ACCOUNT_AUTH_URL;
    const portalGatewayFixture = process.env.PLAYWRIGHT_PORTAL_GATEWAY_URL;
    const rewrites: Array<{ source: string; destination: string }> = [];
    if (accountAuthFixture) {
      rewrites.push({
        source: "/account-auth/:path*",
        destination: `${accountAuthFixture}/:path*`,
      });
    }
    if (portalGatewayFixture) {
      rewrites.push({
        source: "/api/:path*",
        destination: `${portalGatewayFixture}/api/:path*`,
      });
    }
    return rewrites;
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
