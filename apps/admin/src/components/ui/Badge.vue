<template><span :class="cn('ui-badge', `ui-badge--${variant}`)"><slot /></span></template>
<script setup lang="ts">
import { cn } from "../../lib/utils";
withDefaults(defineProps<{ variant?: "default" | "success" | "warning" | "danger" | "muted" }>(), { variant: "default" });
</script>
