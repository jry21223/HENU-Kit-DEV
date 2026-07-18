<script setup lang="ts">
import { cva } from "class-variance-authority";

import type { ModuleStatus } from "@/data/modules";

const props = defineProps<{ status: ModuleStatus | "ok" }>();

const variants = cva("inline-flex min-h-7 items-center rounded-full px-2.5 text-sm font-bold", {
  variants: {
    status: {
      ok: "status-success",
      loading: "status-neutral",
      empty: "status-info",
      partial: "status-warning",
      stale: "status-warning",
      unavailable: "status-danger",
      denied: "status-neutral",
    },
  },
});
</script>

<template><span :class="variants({ status: props.status })"><slot /></span></template>

<style scoped>
.status-success { background: var(--hk-ink-green-soft); color: var(--hk-success); }
.status-info { background: color-mix(in srgb, var(--hk-info) 10%, var(--hk-paper-raised)); color: var(--hk-info); }
.status-warning { background: var(--hk-wheat-gold-soft); color: var(--hk-warning); }
.status-danger { background: color-mix(in srgb, var(--hk-danger) 10%, var(--hk-paper-raised)); color: var(--hk-danger); }
.status-neutral { background: var(--hk-paper-line); color: var(--hk-ink); }
</style>
