<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import type { TabsTriggerProps } from "reka-ui";
import { TabsTrigger, useForwardProps } from "reka-ui";
import { cn } from "@/lib/utils";
const props = defineProps<TabsTriggerProps & { class?: HTMLAttributes["class"] }>();
const forwarded = useForwardProps(props);
</script>
<template><TabsTrigger v-bind="forwarded" :class="cn('inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow', props.class)"><slot /></TabsTrigger></template>
