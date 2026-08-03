<script setup lang="ts">
import type { HTMLAttributes } from "vue";

import { cn } from "@/lib/utils";

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    class?: HTMLAttributes["class"];
  }>(),
  { modelValue: "" },
);

const emit = defineEmits<{
  "update:modelValue": [value: string | number];
}>();
</script>

<template>
  <input
    :value="props.modelValue"
    :class="
      cn(
        'flex h-11 w-full min-w-0 rounded-md border border-input bg-white px-3 py-2 text-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
    @input="
      emit('update:modelValue', ($event.target as HTMLInputElement).value)
    "
  />
</template>
