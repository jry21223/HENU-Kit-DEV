import type { Metadata } from "next";

import { practiceMetadata } from "@/lib/seo";
import PracticePageClient from "./page-client";

export const metadata: Metadata = practiceMetadata;

export default function PracticePage() {
  return <PracticePageClient />;
}
