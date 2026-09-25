"use client";

import { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";

function getWidgetUrl() {
  const configuredUrl = process.env.NEXT_PUBLIC_LANGBOT_WIDGET_URL?.trim();
  if (!configuredUrl) {
    return null;
  }

  try {
    const parsedUrl = new URL(configuredUrl);
    const allowsProtocol =
      parsedUrl.protocol === "https:" ||
      (process.env.NODE_ENV !== "production" && parsedUrl.protocol === "http:");

    return allowsProtocol ? parsedUrl.toString() : null;
  } catch {
    return null;
  }
}

const subscribe = () => () => {};
function noPendingBinding() {
  try { return !sessionStorage.getItem("henukit-qq-binding"); }
  catch { return false; }
}

export default function LangBotWidget() {
  const pathname = usePathname();
  // SSR is fail closed; only inspect tab storage after hydration. The binding
  // and login entry links perform full document navigation.
  const safe = useSyncExternalStore(subscribe, noPendingBinding, () => false);
  const widgetUrl = getWidgetUrl();
  if (!widgetUrl || !safe || pathname.startsWith("/account") || pathname.startsWith("/bind/")) {
    return null;
  }

  return (
    <script
      id="henukit-langbot-widget"
      src={widgetUrl}
      data-title="HENU-Kit AI 助手"
      async
    />
  );
}
