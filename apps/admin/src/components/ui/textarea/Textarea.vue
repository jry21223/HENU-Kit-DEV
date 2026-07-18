<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import { useVModel } from "@vueuse/core";
import { cn } from "@/lib/utils";
const props = defineProps<{ defaultValue?: string; modelValue?: string; class?: HTMLAttributes["class"]; placeholder?: string }>();
const emits = defineEmits<{ "update:modelValue": [payload: string] }>();
const modelValue = useVModel(props, "modelValue", emits, { passive: true, defaultValue: props.defaultValue });
</script>
<template><textarea v-model="modelValue" :placeholder="placeholder" :class="cn('flex min-h-20 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50', $props.class)" /></template>
