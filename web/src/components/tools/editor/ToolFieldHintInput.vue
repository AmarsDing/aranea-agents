<template>
  <q-input
    :model-value="modelValue"
    dense
    outlined
    :label="label"
    :hint="hint"
    :disable="disable"
    @update:model-value="$emit('update:modelValue', String($event ?? ''))"
  />
</template>

<script setup lang="ts">
defineProps<{
  modelValue: string;
  label: string;
  hint?: string;
  disable?: boolean;
}>();

defineEmits<{ "update:modelValue": [value: string] }>();
</script>
