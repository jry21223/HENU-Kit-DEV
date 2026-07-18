<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import type { PrimitiveProps } from "reka-ui";
import { Primitive } from "reka-ui";
import type { BadgeVariants } from ".";
import { badgeVariants } from ".";
import { cn } from "@/lib/utils";

interface Props extends PrimitiveProps {
  variant?: BadgeVariants["variant"];
  class?: HTMLAttributes["class"];
}
withDefaults(defineProps<Props>(), { as: "div" });
</script>
<template><Primitive :as="as" :as-child="asChild" :class="cn(badgeVariants({ variant }), $props.class)"><slot /></Primitive></template>
