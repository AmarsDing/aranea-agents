<template>
  <div class="column q-gutter-sm tool-schema-builder">
    <div class="row items-center justify-between q-gutter-sm">
      <div class="col min-width-0">
        <div class="text-subtitle2">{{ title }}</div>
        <p class="app-registry-muted-caption q-mb-none q-mt-xs">{{ hint }}</p>
      </div>
      <div class="row q-gutter-xs col-auto">
        <q-btn
          flat
          dense
          no-caps
          size="sm"
          :label="$t('toolsPage.schemaBuilder.addField')"
          icon="add"
          :disable="readonly"
          @click="addRow"
        />
        <q-btn
          flat
          dense
          no-caps
          size="sm"
          :label="showJson ? $t('toolsPage.schemaBuilder.fieldView') : 'JSON'"
          @click="toggleJson"
        />
      </div>
    </div>

    <div v-if="!showJson" class="column q-gutter-sm">
      <q-banner rounded dense class="settings-info-banner">
        {{ $t('toolsPage.schemaBuilder.bannerPre') }}<code>object.properties</code>{{ $t('toolsPage.schemaBuilder.bannerPost') }}
      </q-banner>
      <div v-for="(row, idx) in rows" :key="row.key || `row-${idx}`" class="tool-schema-builder__field-card">
        <div class="app-form-field-grid app-form-field-grid--2col">
          <q-input
            :model-value="row.key"
            dense
            outlined
            :label="$t('toolsPage.schemaBuilder.fieldKey')"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { key: String($event ?? '') })"
          />
          <q-select
            :model-value="row.type"
            dense
            outlined
            emit-value
            map-options
            :label="$t('toolsPage.schemaBuilder.fieldType')"
            :readonly="readonly"
            :disable="readonly"
            :options="typeOptions"
            @update:model-value="updateRow(idx, { type: $event as SchemaFieldType })"
          />
          <q-input
            :model-value="row.title"
            dense
            outlined
            :label="$t('toolsPage.schemaBuilder.fieldTitle')"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { title: String($event ?? '') })"
          />
          <q-toggle
            :model-value="row.required"
            :label="$t('toolsPage.schemaBuilder.fieldRequired')"
            dense
            :disable="readonly"
            @update:model-value="updateRow(idx, { required: Boolean($event) })"
          />
          <q-input
            class="app-grid-span-full"
            :model-value="row.description"
            dense
            outlined
            :label="$t('toolsPage.schemaBuilder.fieldDesc')"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { description: String($event ?? '') })"
          />
          <q-input
            v-if="row.type === 'string'"
            class="app-grid-span-full"
            :model-value="row.enumValues"
            dense
            outlined
            :label="$t('toolsPage.schemaBuilder.fieldEnum')"
            :readonly="readonly"
            @update:model-value="updateRow(idx, { enumValues: String($event ?? '') })"
          />
        </div>
        <div v-if="!readonly" class="row justify-end q-mt-xs">
          <q-btn
            flat
            dense
            round
            icon="delete"
            size="sm"
            class="app-registry-icon-btn"
            :aria-label="$t('toolsPage.schemaBuilder.removeField')"
            @click="removeRow(idx)"
          />
        </div>
      </div>
      <div v-if="!rows.length" class="app-registry-muted-caption">{{ $t('toolsPage.schemaBuilder.empty') }}</div>
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
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  buildSchemaFromFields,
  emptySchemaField,
  parseSchemaFields,
  type SchemaFieldRow,
  type SchemaFieldType,
} from '../../../features/tools/jsonSchemaBuilder';

const props = defineProps<{
  modelValue: string;
  title: string;
  hint: string;
  readonly?: boolean;
}>();

const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const showJson = ref(false);
const rows = ref<SchemaFieldRow[]>([]);
const dirty = ref(false);

const { t } = useI18n();

const typeOptions = computed(() => [
  { label: t('toolsPage.schemaBuilder.typeString'), value: 'string' },
  { label: t('toolsPage.schemaBuilder.typeNumber'), value: 'number' },
  { label: t('toolsPage.schemaBuilder.typeInteger'), value: 'integer' },
  { label: t('toolsPage.schemaBuilder.typeBoolean'), value: 'boolean' },
]);

function syncRowsFromJson(json: string) {
  rows.value = parseSchemaFields(json);
  dirty.value = false;
}

watch(
  () => props.modelValue,
  (v) => {
    if (!showJson.value && !dirty.value) syncRowsFromJson(v);
  },
  { immediate: true },
);

function emitSchema() {
  dirty.value = true;
  emit('update:modelValue', buildSchemaFromFields(rows.value));
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
