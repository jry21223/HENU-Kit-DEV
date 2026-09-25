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

/**
 * 配置 NEXT_PUBLIC_LANGBOT_WIDGET_URL 会在每个页面加载第三方聊天脚本。启用前先在隐私政策
 * （src/app/(legal)/privacy/page.tsx）的第三方与 Cookie 部分写明它，去留与加固见 #220。
 */
export default function LangBotWidget() {
  const widgetUrl = getWidgetUrl();
  if (!widgetUrl) {
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
