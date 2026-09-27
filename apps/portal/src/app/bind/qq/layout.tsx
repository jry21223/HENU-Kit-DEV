import type { Metadata } from "next";
import SiteShell from "@/components/site-shell";

export const metadata: Metadata = {
  title: "绑定 HENU Bot",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default function BindingLayout({ children }: { children: React.ReactNode }) {
  return <SiteShell>{children}</SiteShell>;
}
