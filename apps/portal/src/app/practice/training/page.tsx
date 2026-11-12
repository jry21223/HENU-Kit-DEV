import type { Metadata } from "next";
import TrainingCatalog from "@/components/practice/training-catalog";

export const metadata: Metadata = { title: "英语听力与完形训练", robots: { index: false, follow: false } };

export default function TrainingPage() {
  return <TrainingCatalog />;
}
