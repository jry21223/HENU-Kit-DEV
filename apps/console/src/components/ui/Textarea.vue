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
  <textarea
    :value="props.modelValue"
    :class="
      cn(
        'flex min-h-[80px] w-full rounded-md border border-input bg-white px-3 py-2 text-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
    @input="
      emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)
    "
  />
</template>
