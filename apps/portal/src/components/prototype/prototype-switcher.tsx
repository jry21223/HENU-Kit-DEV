"use client";

import { ArrowLeft, ArrowRight } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";

export type PrototypeVariant = "A" | "B" | "C";

const VARIANTS: Array<{ key: PrototypeVariant; name: string }> = [
  { key: "A", name: "浮动抽屉" },
  { key: "B", name: "命令带 + 双栏" },
  { key: "C", name: "上下文工作台" },
];

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.matches("input, textarea, [contenteditable='true']") ||
    Boolean(target.closest("input, textarea, [contenteditable='true']"))
  );
}

export default function PrototypeSwitcher({ current }: { current: PrototypeVariant }) {
  const router = useRouter();
  const searchParams = useSearchParams();

  function select(direction: -1 | 1) {
    const currentIndex = VARIANTS.findIndex((variant) => variant.key === current);
    const nextIndex = (currentIndex + direction + VARIANTS.length) % VARIANTS.length;
    const params = new URLSearchParams(searchParams.toString());
    params.set("variant", VARIANTS[nextIndex].key);
    router.replace(`?${params.toString()}`, { scroll: false });
  }

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (isEditableTarget(event.target)) return;
      if (event.key === "ArrowLeft") select(-1);
      if (event.key === "ArrowRight") select(1);
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  if (process.env.NODE_ENV === "production") return null;

  const selected = VARIANTS.find((variant) => variant.key === current) ?? VARIANTS[0];

  return (
    <div
      className="fixed bottom-4 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-1 rounded-full border border-white/15 bg-black px-1.5 py-1.5 text-white shadow-2xl"
      aria-label="原型方案切换器"
    >
      <button
        type="button"
        onClick={() => select(-1)}
        className="grid size-9 place-items-center rounded-full hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
        aria-label="上一个方案"
      >
        <ArrowLeft size={16} />
      </button>
      <span className="min-w-36 px-2 text-center font-mono text-xs tracking-wide">
        {selected.key} · {selected.name}
      </span>
      <button
        type="button"
        onClick={() => select(1)}
        className="grid size-9 place-items-center rounded-full hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
        aria-label="下一个方案"
      >
        <ArrowRight size={16} />
      </button>
    </div>
  );
}
