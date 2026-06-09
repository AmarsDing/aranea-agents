<template>
  <q-select
    :model-value="modelValue"
    dense
    outlined
    multiple
    use-chips
    use-input
    input-debounce="300"
    label="工具列表"
    :options="filteredOptions"
    emit-value
    map-options
    @filter="onFilter"
    @update:model-value="(v: string[]) => $emit('update:modelValue', v)"
  >
    <template #no-option>
      <q-item>
        <q-item-section class="text-grey">无匹配工具</q-item-section>
      </q-item>
    </template>
  </q-select>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';

const props = defineProps<{
  modelValue: string[];
  options?: Array<{ label: string; value: string }>;
}>();

defineEmits<{
  'update:modelValue': [value: string[]];
}>();

const filteredOptions = ref<Array<{ label: string; value: string }>>([]);

watch(
  () => props.options,
  (opts) => {
    filteredOptions.value = opts ?? [];
  },
  { immediate: true },
);

function onFilter(val: string, update: (callback: () => void, afterFn?: (ref: any) => void) => void) {
  update(() => {
    if (!val) {
      filteredOptions.value = props.options ?? [];
    } else {
      const needle = val.toLowerCase();
      filteredOptions.value = (props.options ?? []).filter(
        (opt) => opt.label.toLowerCase().includes(needle) || opt.value.toLowerCase().includes(needle),
      );
    }
  });
}
</script>
