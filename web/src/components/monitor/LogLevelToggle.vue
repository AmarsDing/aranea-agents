<template>
  <div class="monitor-log-level-toggle" role="group" aria-label="最低日志级别">
    <button
      v-for="opt in options"
      :key="opt.value"
      type="button"
      class="monitor-log-level-toggle__btn"
      :class="[
        `monitor-log-level-toggle__btn--${opt.value.toLowerCase()}`,
        { 'monitor-log-level-toggle__btn--active': modelValue === opt.value }
      ]"
      :aria-pressed="modelValue === opt.value"
      @click="emit('update:modelValue', opt.value)"
    >
      {{ opt.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR";

defineProps<{
  modelValue: LogLevel;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: LogLevel];
}>();

const options: { label: string; value: LogLevel }[] = [
  { label: "DEBUG", value: "DEBUG" },
  { label: "INFO", value: "INFO" },
  { label: "WARN", value: "WARN" },
  { label: "ERROR", value: "ERROR" }
];
</script>
