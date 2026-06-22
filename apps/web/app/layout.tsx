import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "软件学院资料库",
  description: "面向软件学院课程 PDF、讲义、真题和复习资料的一站式资料库",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
