import type { Metadata } from "next";

import { libraryMetadata } from "@/lib/seo";
import LibraryPageClient from "./page-client";

export const metadata: Metadata = libraryMetadata;

export default function LibraryPage() {
  return <LibraryPageClient />;
}
