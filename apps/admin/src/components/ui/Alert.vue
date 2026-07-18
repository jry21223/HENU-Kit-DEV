<template><div :class="cn('ui-alert', `ui-alert--${variant}`)" role="status"><slot /></div></template>
<script setup lang="ts">
import { cn } from "../../lib/utils";
withDefaults(defineProps<{ variant?: "info" | "warning" | "danger" }>(), { variant: "info" });
</script>
