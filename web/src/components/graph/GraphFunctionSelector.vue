<template>
  <q-select
    :model-value="modelValue"
    dense
    outlined
    use-input
    input-debounce="300"
    label="函数引用 (funcRef)"
    :options="filteredOptions"
    emit-value
    map-options
    :rules="rules"
    @filter="onFilter"
    @update:model-value="(v: string | number | null) => $emit('update:modelValue', String(v ?? ''))"
  >
    <template #no-option>
      <q-item>
        <q-item-section class="text-grey">无匹配函数</q-item-section>
      </q-item>
    </template>
  </q-select>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    options?: Array<{ label: string; value: string }>;
    rules?: Array<(val: string | number | null | undefined) => boolean | string>;
  }>(),
  {
    rules: () => [],
  },
);

defineEmits<{
  'update:modelValue': [value: string];
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
