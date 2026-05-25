<template>
  <div class="plugin-schema-form column q-gutter-sm">
    <template v-for="(def, key) in properties" :key="key">
      <q-toggle
        v-if="def.type === 'boolean'"
        :model-value="Boolean(getValue(key))"
        :label="fieldLabel(key, def)"
        dense
        @update:model-value="setValue(key, $event)"
      />
      <q-select
        v-else-if="def.enum"
        :model-value="getValue(key)"
        :label="fieldLabel(key, def)"
        :options="def.enum"
        dense
        outlined
        emit-value
        @update:model-value="setValue(key, $event)"
      />
      <q-input
        v-else-if="def.type === 'number' || def.type === 'integer'"
        :model-value="getInputValue(key)"
        :label="fieldLabel(key, def)"
        type="number"
        dense
        outlined
        @update:model-value="setValue(key, Number($event))"
      />
      <q-input
        v-else-if="def.type === 'array' && stringArrayItem(def)"
        :model-value="arrayText(key)"
        :label="fieldLabel(key, def)"
        type="textarea"
        autogrow
        dense
        outlined
        hint="每行一项"
        @update:model-value="setArrayLines(key, String($event ?? ''))"
      />
      <ModelRouterRulesEditor
        v-else-if="def.type === 'array' && key === 'rules'"
        :model-value="rulesValue(key)"
        :label="fieldLabel(key, def)"
        @update:model-value="setValue(key, $event)"
      />
      <q-input
        v-else-if="def.type === 'array'"
        :model-value="jsonFieldText(key)"
        :label="fieldLabel(key, def)"
        type="textarea"
        autogrow
        dense
        outlined
        hint="JSON 数组"
        @update:model-value="setJSONField(key, String($event ?? ''))"
      />
      <q-input
        v-else
        :model-value="getInputValue(key)"
        :label="fieldLabel(key, def)"
        dense
        outlined
        @update:model-value="setValue(key, $event)"
      />
    </template>
    <div v-if="!hasProperties" class="text-caption text-grey-7">Schema 无可渲染字段，请使用 JSON 模式编辑</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import ModelRouterRulesEditor, { type ModelRouterRulePayload } from "./ModelRouterRulesEditor.vue";

type SchemaProperty = {
  type?: string;
  title?: string;
  description?: string;
  enum?: string[];
  default?: unknown;
  items?: { type?: string };
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

function fieldLabel(key: string, def: SchemaProperty) {
  return def.title || def.description || key;
}

function stringArrayItem(def: SchemaProperty) {
  return def.items?.type === "string";
}

function getValue(key: string): string | number | boolean | null {
  const v = data.value[key] ?? properties.value[key]?.default ?? "";
  if (typeof v === "boolean" || typeof v === "number") return v;
  return v == null ? null : String(v);
}

function getInputValue(key: string): string {
  const v = getValue(key);
  if (typeof v === "boolean") return v ? "1" : "0";
  if (v == null) return "";
  return String(v);
}

function setValue(key: string, val: unknown) {
  const next = { ...data.value, [key]: val };
  emit("update:modelValue", JSON.stringify(next, null, 2));
}

function arrayText(key: string): string {
  const raw = data.value[key];
  if (!Array.isArray(raw)) return "";
  return raw.map((item) => String(item)).join("\n");
}

function setArrayLines(key: string, text: string) {
  const lines = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  setValue(key, lines);
}

function rulesValue(key: string): ModelRouterRulePayload[] {
  const raw = data.value[key];
  if (!Array.isArray(raw)) return [];
  return raw as ModelRouterRulePayload[];
}

function jsonFieldText(key: string): string {
  const raw = data.value[key];
  if (raw == null) return "[]";
  try {
    return JSON.stringify(raw, null, 2);
  } catch {
    return "[]";
  }
}

function setJSONField(key: string, text: string) {
  try {
    const parsed = JSON.parse(text || "[]");
    setValue(key, parsed);
  } catch {
    // keep invalid text in textarea until user fixes JSON mode
  }
}
</script>
