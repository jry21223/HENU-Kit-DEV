import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Required for multi-stage Docker image (apps/portal/Dockerfile).
  output: "standalone",
};

export default nextConfig;
