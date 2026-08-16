import type { Metadata } from "next";

import { careerMetadata } from "@/lib/seo";
import CareerPageClient from "./page-client";

export const metadata: Metadata = careerMetadata;

export default function CareerPage() {
  return <CareerPageClient />;
}
