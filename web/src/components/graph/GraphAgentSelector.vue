<template>
  <q-select
    :model-value="modelValue"
    dense
    outlined
    use-input
    input-debounce="300"
    label="Agent 名称"
    :options="filteredOptions"
    option-label="label"
    option-value="value"
    emit-value
    map-options
    :rules="rules"
    @filter="onFilter"
    @update:model-value="(v: string | number | null) => $emit('update:modelValue', String(v ?? ''))"
  >
    <template #option="scope">
      <q-item v-bind="scope.itemProps">
        <q-item-section avatar>
          <q-icon
            :name="
              scope.opt.kind === 'a2a_proxy'
                ? 'language'
                : scope.opt.kind === 'system_builtin'
                  ? 'construction'
                  : 'smart_toy'
            "
            size="18px"
            color="grey-7"
          />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ scope.opt.label }}</q-item-label>
          <q-item-label caption>{{
            scope.opt.kind === 'a2a_proxy' ? 'A2A Proxy' : scope.opt.kind === 'system_builtin' ? '系统' : '自建'
          }}</q-item-label>
        </q-item-section>
      </q-item>
    </template>
    <template #no-option>
      <q-item>
        <q-item-section class="text-grey">无匹配 Agent</q-item-section>
      </q-item>
    </template>
  </q-select>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    options?: Array<{ label: string; value: string; kind: string }>;
    rules?: Array<(val: string | number | null | undefined) => boolean | string>;
  }>(),
  {
    options: () => [],
    rules: () => [],
  },
);

defineEmits<{
  'update:modelValue': [value: string];
}>();

const filteredOptions = ref<Array<{ label: string; value: string; kind: string }>>([]);

watch(
  () => props.options,
  (opts) => {
    filteredOptions.value = opts ?? [];
  },
  { immediate: true },
);

function onFilter(val: string, update: (callback: () => void, afterFn?: (ref: unknown) => void) => void) {
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
