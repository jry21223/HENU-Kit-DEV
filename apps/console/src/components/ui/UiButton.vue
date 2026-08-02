<script setup lang="ts">
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "reka-ui";
import type { HTMLAttributes } from "vue";

import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-[var(--hk-radius-control)] text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--hk-focus-ring)] disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-[var(--hk-ink)] px-4 text-white hover:bg-[var(--hk-accent)]",
        ghost: "px-3 text-[var(--hk-ink)] hover:bg-[var(--hk-accent-soft)]",
        "ghost-inverse": "px-3 text-white hover:bg-white/10",
      },
      size: {
        default: "h-11",
        icon: "size-11 p-0",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

type ButtonVariants = VariantProps<typeof buttonVariants>;

withDefaults(
  defineProps<{
    asChild?: boolean;
    class?: HTMLAttributes["class"];
    variant?: ButtonVariants["variant"];
    size?: ButtonVariants["size"];
  }>(),
  { asChild: false, variant: "default", size: "default" },
);
</script>

<template>
  <component :is="asChild ? Slot : 'button'" :class="cn(buttonVariants({ variant, size }), $props.class)">
    <slot />
  </component>
</template>
