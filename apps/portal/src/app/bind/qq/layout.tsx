import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "绑定 HENU Bot",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default function BindingLayout({ children }: { children: React.ReactNode }) {
  return children;
}
