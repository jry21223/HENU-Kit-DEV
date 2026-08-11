import type { Metadata } from "next";
import NoticeFeed from "@/components/notice/notice-feed";

export const metadata: Metadata = {
  title: "通知 — henukit",
  description: "面向全校学生的站内通知。",
};

export default function NoticePage() {
  return <NoticeFeed />;
}
