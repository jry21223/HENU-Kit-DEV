import type { Metadata } from "next";
import { Suspense } from "react";

import { careerMetadata } from "@/lib/seo";
import CareerPageClient from "./page-client";

export const metadata: Metadata = careerMetadata;

export default function CareerPage() {
  return (
    <Suspense fallback={null}>
      <CareerPageClient />
    </Suspense>
  );
}
