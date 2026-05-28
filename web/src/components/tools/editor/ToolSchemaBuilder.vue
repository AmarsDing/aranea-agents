<template>
  <div class="column q-gutter-sm tool-schema-builder">
    <div class="row items-center justify-between q-gutter-sm">
      <div class="col min-width-0">
        <div class="text-subtitle2">{{ title }}</div>
        <p class="app-registry-muted-caption q-mb-none q-mt-xs">{{ hint }}</p>
      </div>
      <div class="row q-gutter-xs col-auto">
        <q-btn flat dense no-caps size="sm" label="添加字段" icon="add" :disable="readonly" @click="addRow" />
        <q-btn flat dense no-caps size="sm" :label="showJson ? '字段视图' : 'JSON'" @click="toggleJson" />
      </div>
    </div>

    <div v-if="!showJson" class="column q-gutter-sm">
      <q-banner rounded dense class="settings-info-banner">
        字段视图仅支持扁平 <code>object.properties</code>（string / number / integer / boolean）。
        嵌套 object、array、oneOf 等请切换到 JSON 模式编辑，切回字段视图可能丢失复杂结构。
      </q-banner>
      <div
        v-for="(row, idx) in rows"
        :key="row.key || `row-${idx}`"
        class="tool-schema-builder__field-card"
      >
        <div class="app-form-field-grid app-form-field-grid--2col">
          <q-input
            :model-value="row.key"
            dense
            outlined
            label="字段 Key"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { key: String($event ?? '') })"
          />
          <q-select
            :model-value="row.type"
            dense
            outlined
            emit-value
            map-options
            label="类型"
            :readonly="readonly"
            :disable="readonly"
            :options="typeOptions"
            @update:model-value="updateRow(idx, { type: $event as SchemaFieldType })"
          />
          <q-input
            :model-value="row.title"
            dense
            outlined
            label="标题"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { title: String($event ?? '') })"
          />
          <q-toggle
            :model-value="row.required"
            label="必填"
            dense
            :disable="readonly"
            @update:model-value="updateRow(idx, { required: Boolean($event) })"
          />
          <q-input
            class="app-grid-span-full"
            :model-value="row.description"
            dense
            outlined
            label="描述"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { description: String($event ?? '') })"
          />
          <q-input
            v-if="row.type === 'string'"
            class="app-grid-span-full"
            :model-value="row.enumValues"
            dense
            outlined
            label="枚举（逗号分隔，可选）"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { enumValues: String($event ?? '') })"
          />
        </div>
        <div v-if="!readonly" class="row justify-end q-mt-xs">
          <q-btn flat dense round icon="delete" size="sm" class="app-registry-icon-btn" @click="removeRow(idx)" />
        </div>
      </div>
      <div v-if="!rows.length" class="app-registry-muted-caption">暂无字段；无参工具可留空对象 {}。</div>
    </div>

    <q-input
      v-else
      :model-value="modelValue"
      type="textarea"
      outlined
      autogrow
      dense
      class="app-field-long"
      :label="title + ' (JSON)'"
      :readonly="readonly"
      @update:model-value="$emit('update:modelValue', String($event ?? '{}'))"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import {
  buildSchemaFromFields,
  emptySchemaField,
  parseSchemaFields,
  type SchemaFieldRow,
  type SchemaFieldType
} from "../../../features/tools/jsonSchemaBuilder";

const props = defineProps<{
  modelValue: string;
  title: string;
  hint: string;
  readonly?: boolean;
}>();

const emit = defineEmits<{ "update:modelValue": [value: string] }>();

const showJson = ref(false);
const rows = ref<SchemaFieldRow[]>([]);
const dirty = ref(false);

const typeOptions = [
  { label: "字符串", value: "string" },
  { label: "数字", value: "number" },
  { label: "整数", value: "integer" },
  { label: "布尔", value: "boolean" }
];

function syncRowsFromJson(json: string) {
  rows.value = parseSchemaFields(json);
  dirty.value = false;
}

watch(
  () => props.modelValue,
  (v) => {
    if (!showJson.value && !dirty.value) syncRowsFromJson(v);
  },
  { immediate: true }
);

function emitSchema() {
  dirty.value = true;
  emit("update:modelValue", buildSchemaFromFields(rows.value));
}

function updateRow(idx: number, patch: Partial<SchemaFieldRow>) {
  const row = rows.value[idx];
  if (!row) return;
  Object.assign(row, patch);
  emitSchema();
}

function addRow() {
  rows.value.push(emptySchemaField());
  emitSchema();
}

function removeRow(idx: number) {
  rows.value.splice(idx, 1);
  emitSchema();
}

function toggleJson() {
  if (!showJson.value) {
    emitSchema();
  } else {
    syncRowsFromJson(props.modelValue);
  }
  showJson.value = !showJson.value;
}
</script>
