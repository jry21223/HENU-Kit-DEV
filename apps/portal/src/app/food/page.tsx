import type { Metadata } from "next";

import { foodMetadata } from "@/lib/seo";
import FoodPageClient from "./page-client";

export const metadata: Metadata = foodMetadata;

export default function FoodPage() {
  return <FoodPageClient />;
}
