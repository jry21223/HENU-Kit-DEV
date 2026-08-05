<script setup lang="ts">
import { cva } from "class-variance-authority";

import type { ModuleStatus } from "@/data/modules";

const props = defineProps<{ status: ModuleStatus | "ok" }>();

// Tinted surface + saturated text, so status stays legible on a page that is
// otherwise entirely neutral. Colour is never the only signal — every caller
// pairs the badge with a text label.
const variants = cva(
  "inline-flex min-h-6 items-center rounded-md border px-2 text-xs font-medium whitespace-nowrap",
  {
    variants: {
      status: {
        ok: "border-success/25 bg-success/10 text-success",
        loading: "border-border bg-muted text-muted-foreground",
        empty: "border-info/25 bg-info/10 text-info",
        partial: "border-warning/30 bg-warning/12 text-warning",
        stale: "border-warning/30 bg-warning/12 text-warning",
        unavailable: "border-destructive/25 bg-destructive/10 text-destructive",
        denied: "border-border bg-muted text-muted-foreground",
      },
    },
  },
);
</script>

<template><span :class="variants({ status: props.status })"><slot /></span></template>
