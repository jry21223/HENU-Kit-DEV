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
