<script setup lang="ts">
import { cva } from "class-variance-authority";

import type { ModuleStatus } from "@/data/modules";

const props = defineProps<{ status: ModuleStatus | "ok" }>();

const variants = cva("inline-flex min-h-7 items-center rounded-full px-2.5 text-[0.7rem] font-bold", {
  variants: {
    status: {
      ok: "bg-emerald-100 text-emerald-800",
      loading: "bg-slate-100 text-slate-600",
      empty: "bg-sky-100 text-sky-800",
      partial: "bg-amber-100 text-amber-900",
      stale: "bg-orange-100 text-orange-900",
      unavailable: "bg-rose-100 text-rose-800",
      denied: "bg-zinc-200 text-zinc-700",
    },
  },
});
</script>

<template><span :class="variants({ status: props.status })"><slot /></span></template>
