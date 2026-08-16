import type { Metadata } from "next";

import { campusMetadata } from "@/lib/seo";
import CampusPageClient from "./page-client";

export const metadata: Metadata = campusMetadata;

export default function CampusPage() {
  return <CampusPageClient />;
}
