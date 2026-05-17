<template>
  <div class="schema-form column q-gutter-sm">
    <template v-for="(def, key) in properties" :key="key">
      <q-toggle
        v-if="def.type === 'boolean'"
        :model-value="getValue(key)"
        :label="def.title || key"
        dense
        @update:model-value="setValue(key, $event)"
      />
      <q-select
        v-else-if="def.enum"
        :model-value="getValue(key)"
        :label="def.title || key"
        :options="def.enum"
        dense
        outlined
        emit-value
        @update:model-value="setValue(key, $event)"
      />
      <q-input
        v-else-if="def.type === 'number' || def.type === 'integer'"
        :model-value="getInputValue(key)"
        :label="def.title || key"
        type="number"
        dense
        outlined
        @update:model-value="setValue(key, Number($event))"
      />
      <q-input
        v-else
        :model-value="getInputValue(key)"
        :label="def.title || key"
        dense
        outlined
        @update:model-value="setValue(key, $event)"
      />
    </template>
    <div v-if="!hasProperties" class="text-caption">Schema 无可渲染属性，请使用 JSON 编辑</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

type SchemaProperty = {
  type?: string;
  title?: string;
  enum?: string[];
  default?: unknown;
};

const props = defineProps<{
  schemaJson: string;
  modelValue: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const schema = computed(() => {
  try {
    return JSON.parse(props.schemaJson || "{}");
  } catch {
    return {};
  }
});

const properties = computed<Record<string, SchemaProperty>>(() => schema.value.properties ?? {});

const hasProperties = computed(() => Object.keys(properties.value).length > 0);

const data = computed<Record<string, unknown>>(() => {
  try {
    return JSON.parse(props.modelValue || "{}");
  } catch {
    return {};
  }
});

function getValue(key: string): string | number | boolean | null {
  const v = data.value[key] ?? properties.value[key]?.default ?? "";
  if (typeof v === "boolean" || typeof v === "number") return v;
  return v == null ? null : String(v);
}

function getInputValue(key: string): string | number | null {
  const v = getValue(key);
  if (typeof v === "boolean") return v ? 1 : 0;
  return v;
}

function setValue(key: string, val: unknown) {
  const next = { ...data.value, [key]: val };
  emit("update:modelValue", JSON.stringify(next, null, 2));
}
</script>
