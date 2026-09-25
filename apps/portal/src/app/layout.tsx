import type { Metadata } from "next";
import GatewayBootstrap from "@/components/gateway-bootstrap";
import LangBotWidget from "@/components/langbot-widget";
import ScrollMemory from "@/components/scroll-memory";
import { siteMetadata, websiteStructuredData } from "@/lib/seo";
import { fontVariables } from "./fonts";
import "./globals.css";

export const metadata: Metadata = siteMetadata;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const structuredData = JSON.stringify(websiteStructuredData()).replace(
    /</g,
    "\\u003c"
  );

  return (
    <html
      lang="zh-CN"
      className={`${fontVariables} h-full antialiased`}
    >
      <body className="min-h-full">
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: structuredData }}
        />
        <GatewayBootstrap />
        <ScrollMemory />
        {children}
        <LangBotWidget />
      </body>
    </html>
  );
}
