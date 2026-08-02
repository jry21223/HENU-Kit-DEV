import type { Metadata } from "next";
import { Space_Grotesk, IBM_Plex_Mono } from "next/font/google";
import GatewayBootstrap from "@/components/gateway-bootstrap";
import LangBotWidget from "@/components/langbot-widget";
import "./globals.css";

const spaceGrotesk = Space_Grotesk({
  variable: "--font-space-grotesk",
  subsets: ["latin"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  title: "henukit — Keep In Touch",
  description:
    "henukit 是面向校园的综合性学生平台：资料库、AI 智能刷题、美食排行榜、校园互助，一个站点全部搞定。",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="zh-CN"
      className={`${spaceGrotesk.variable} ${plexMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <GatewayBootstrap />
        {children}
        <LangBotWidget />
      </body>
    </html>
  );
}
