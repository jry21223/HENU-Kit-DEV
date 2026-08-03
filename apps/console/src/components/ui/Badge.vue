<script setup lang="ts">
import type { HTMLAttributes } from "vue";

import { badgeVariants, type BadgeVariants } from "./badge";
import { cn } from "@/lib/utils";

interface Props {
  variant?: BadgeVariants["variant"];
  class?: HTMLAttributes["class"];
}

const props = withDefaults(defineProps<Props>(), {
  variant: "default",
});
</script>

<template>
  <span :class="cn(badgeVariants({ variant }), props.class)">
    <slot />
  </span>
</template>
