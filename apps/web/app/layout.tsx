import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "一站式学习平台 V2",
  description: "高校课程资料、刷题、Wiki 与 AI 学习平台",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
