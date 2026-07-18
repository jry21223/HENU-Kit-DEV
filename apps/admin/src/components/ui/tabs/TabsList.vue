<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import type { TabsListProps } from "reka-ui";
import { TabsList } from "reka-ui";
import { cn } from "@/lib/utils";
const props = defineProps<TabsListProps & { class?: HTMLAttributes["class"] }>();
</script>
<template><TabsList v-bind="props" :class="cn('inline-flex h-9 items-center justify-center rounded-lg bg-muted p-1 text-muted-foreground', props.class)"><slot /></TabsList></template>
