<script setup lang="ts">import type { HTMLAttributes } from "vue"; import { cn } from "@/lib/utils"; defineProps<{class?:HTMLAttributes["class"]}>();</script>
<template><th :class="cn('h-10 px-3 text-left align-middle text-xs font-medium text-muted-foreground', $props.class)"><slot /></th></template>
