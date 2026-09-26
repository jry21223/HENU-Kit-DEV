import type { Metadata } from "next";
import type { ReactNode } from "react";
import SiteShell from "@/components/site-shell";

export const metadata: Metadata = {
  referrer: "no-referrer",
};

export default function AccountAuthLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <SiteShell>{children}</SiteShell>;
}
