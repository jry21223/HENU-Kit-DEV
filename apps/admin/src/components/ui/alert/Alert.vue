<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import type { AlertVariants } from ".";
import { alertVariants } from ".";
import { cn } from "@/lib/utils";
defineProps<{ class?: HTMLAttributes["class"]; variant?: AlertVariants["variant"] }>();
</script>
<template><div role="alert" :class="cn(alertVariants({ variant }), $props.class)"><slot /></div></template>
