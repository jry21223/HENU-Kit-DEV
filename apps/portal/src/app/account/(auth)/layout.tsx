import type { Metadata } from "next";
import type { ReactNode } from "react";
import SiteFooter from "@/components/site-footer";

export const metadata: Metadata = {
  referrer: "no-referrer",
};

export default function AccountAuthLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <>
      {children}
      <SiteFooter />
    </>
  );
}
