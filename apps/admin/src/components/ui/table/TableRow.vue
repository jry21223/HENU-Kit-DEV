<script setup lang="ts">import type { HTMLAttributes } from "vue"; import { cn } from "@/lib/utils"; defineProps<{class?:HTMLAttributes["class"]}>();</script>
<template><tr :class="cn('border-b transition-colors hover:bg-muted/50', $props.class)"><slot /></tr></template>
