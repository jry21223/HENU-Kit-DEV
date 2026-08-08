import type { Metadata } from "next";
import { Space_Grotesk, IBM_Plex_Mono } from "next/font/google";
import GatewayBootstrap from "@/components/gateway-bootstrap";
import LangBotWidget from "@/components/langbot-widget";
import { siteMetadata, websiteStructuredData } from "@/lib/seo";
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
      className={`${spaceGrotesk.variable} ${plexMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: structuredData }}
        />
        <GatewayBootstrap />
        {children}
        <LangBotWidget />
      </body>
    </html>
  );
}
