"use client";

import { useEffect } from "react";
import { hasGateway } from "@/lib/api/client";
import { initAllGateways } from "@/lib/gateway-init";

/**
 * Root client bootstrap: warm gateway adapters once on first paint.
 * Safe no-op when gateway is not configured.
 */
export default function GatewayBootstrap() {
  useEffect(() => {
    if (!hasGateway) return;
    void initAllGateways();
  }, []);
  return null;
}
