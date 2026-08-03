<script setup lang="ts">
import type { HTMLAttributes } from "vue";

import { cn } from "@/lib/utils";

const props = withDefaults(
  defineProps<{
    eyebrow?: string;
    title?: string;
    description?: string;
    titleId?: string;
    class?: HTMLAttributes["class"];
  }>(),
  { eyebrow: "", title: "", description: "", titleId: "" },
);
</script>

<template>
  <header
    :class="
      cn('flex flex-wrap items-end justify-between gap-6', props.class)
    "
  >
    <div class="grid gap-1.5">
      <p
        v-if="eyebrow"
        class="font-mono text-xs font-bold uppercase tracking-[0.14em] text-[var(--hk-accent)]"
      >
        {{ eyebrow }}
      </p>
      <h1 v-if="title" :id="titleId || undefined" class="text-2xl font-bold tracking-tight">
        {{ title }}
      </h1>
      <p
        v-if="description"
        class="max-w-prose text-sm leading-6 text-muted-foreground"
      >
        {{ description }}
      </p>
    </div>
    <slot />
  </header>
</template>
