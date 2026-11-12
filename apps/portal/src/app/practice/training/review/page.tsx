import type { Metadata } from "next";
import TrainingReviewItems from "@/components/practice/training-review-items";

export const metadata: Metadata = { title: "待复习训练", robots: { index: false, follow: false } };

export default function TrainingReviewPage() {
  return <TrainingReviewItems />;
}
