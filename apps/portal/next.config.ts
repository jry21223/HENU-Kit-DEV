import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Required for multi-stage Docker image (apps/portal/Dockerfile).
  output: "standalone",
  // Playwright reaches the local dev server through 127.0.0.1.  Permit that
  // same-origin HMR connection so browser acceptance tests exercise hydrated
  // client state instead of the SSR-only auth shell.
  allowedDevOrigins: ["127.0.0.1"],
};

export default nextConfig;
