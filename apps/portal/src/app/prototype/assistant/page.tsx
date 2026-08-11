import { Suspense } from "react";
import AssistantPrototype from "@/components/prototype/assistant-prototype";

export default function AssistantPrototypePage() {
  return (
    <Suspense fallback={<main className="min-h-screen bg-paper" />}>
      <AssistantPrototype />
    </Suspense>
  );
}
