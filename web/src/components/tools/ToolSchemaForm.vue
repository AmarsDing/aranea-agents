<template>
  <div class="schema-form column q-gutter-sm">
    <template v-for="(def, key) in properties" :key="key">
      <q-toggle
        v-if="def.type === 'boolean'"
        :model-value="getValue(key)"
        :label="def.title || key"
        dense
        @update:model-value="setValue(key, $event)"
      >
        <q-tooltip v-if="def.description">{{ def.description }}</q-tooltip>
      </q-toggle>

      <q-select
        v-else-if="def.enum"
        :model-value="getValue(key)"
        :label="def.title || key"
        :options="def.enum"
        dense
        outlined
        emit-value
        :hint="def.description"
        @update:model-value="setValue(key, $event)"
      />

      <q-input
        v-else-if="def.type === 'number' || def.type === 'integer'"
        :model-value="getInputValue(key)"
        :label="def.title || key"
        type="number"
        dense
        outlined
        :hint="def.description"
        @update:model-value="setValue(key, Number($event))"
      />

      <q-input
        v-else-if="isPassword(def)"
        :model-value="getInputValue(key)"
        :label="def.title || key"
        :type="showPassword[key] ? 'text' : 'password'"
        dense
        outlined
        :hint="def.description"
        @update:model-value="setValue(key, $event)"
      >
        <template #append>
          <q-icon
            :name="showPassword[key] ? 'visibility_off' : 'visibility'"
            class="cursor-pointer"
            @click="togglePassword(key)"
          />
        </template>
      </q-input>

      <template v-else-if="def.type === 'object'">
        <div class="schema-form__object-group">
          <div class="text-subtitle2">{{ def.title || key }}</div>
          <p v-if="def.description" class="text-caption q-ma-none q-mb-xs">{{ def.description }}</p>
          <tool-schema-form
            :schema-json="objectSchemaJson(def)"
            :model-value="getObjectValue(key)"
            @update:model-value="setValue(key, $event)"
          />
        </div>
      </template>

      <template v-else-if="def.type === 'array' && def.items?.type === 'string'">
        <div class="schema-form__array-group">
          <div class="text-subtitle2">{{ def.title || key }}</div>
          <p v-if="def.description" class="text-caption q-ma-none q-mb-xs">{{ def.description }}</p>
          <div class="column q-gutter-xs">
            <div v-for="(_, idx) in getArrayValue(key)" :key="idx" class="row q-gutter-xs items-center">
              <q-input
                :model-value="getArrayValue(key)[idx]"
                dense
                outlined
                class="col"
                @update:model-value="setArrayItem(key, idx, String($event ?? ''))"
              />
              <q-btn
                flat
                dense
                round
                icon="close"
                size="sm"
                class="app-registry-icon-btn"
                @click="removeArrayItem(key, idx)"
              />
            </div>
            <q-btn
              flat
              dense
              no-caps
              icon="add"
              label="添加"
              size="sm"
              class="app-registry-accent-btn"
              @click="addArrayItem(key)"
            />
          </div>
        </div>
      </template>

      <q-input
        v-else
        :model-value="getInputValue(key)"
        :label="def.title || key"
        dense
        outlined
        :hint="def.description"
        @update:model-value="setValue(key, $event)"
      />
    </template>
    <div v-if="!hasProperties" class="text-caption">Schema 无可渲染属性，请使用 JSON 编辑</div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue';

type SchemaProperty = {
  type?: string;
  title?: string;
  description?: string;
  enum?: string[];
  default?: unknown;
  format?: string;
  items?: { type?: string };
  properties?: Record<string, unknown>;
};

const props = defineProps<{
  schemaJson: string;
  modelValue: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const showPassword = reactive<Record<string, boolean>>({});

const schema = computed(() => {
  try {
    return JSON.parse(props.schemaJson || '{}');
  } catch {
    return {};
  }
});

const properties = computed<Record<string, SchemaProperty>>(() => schema.value.properties ?? {});

const hasProperties = computed(() => Object.keys(properties.value).length > 0);

const data = computed<Record<string, unknown>>(() => {
  try {
    return JSON.parse(props.modelValue || '{}');
  } catch {
    return {};
  }
});

function isPassword(def: SchemaProperty): boolean {
  return def.format === 'password' || def.format === 'secret';
}

function togglePassword(key: string) {
  showPassword[key] = !showPassword[key];
}

function getValue(key: string): string | number | boolean | null {
  const v = data.value[key] ?? properties.value[key]?.default ?? '';
  if (typeof v === 'boolean' || typeof v === 'number') return v;
  return v == null ? null : String(v);
}

function getInputValue(key: string): string | number | null {
  const v = getValue(key);
  if (typeof v === 'boolean') return v ? 1 : 0;
  return v;
}

function getArrayValue(key: string): string[] {
  const v = data.value[key];
  if (Array.isArray(v)) return v as string[];
  return [];
}

function getObjectValue(key: string): string {
  const v = data.value[key];
  if (v && typeof v === 'object') return JSON.stringify(v, null, 2);
  return '{}';
}

function objectSchemaJson(def: SchemaProperty): string {
  return JSON.stringify({ type: 'object', properties: def.properties ?? {} });
}

function setValue(key: string, val: unknown) {
  let parsed = val;
  if (typeof val === 'string' && properties.value[key]?.type === 'object') {
    try {
      parsed = JSON.parse(val);
    } catch {
      parsed = val;
    }
  }
  const next = { ...data.value, [key]: parsed };
  emit('update:modelValue', JSON.stringify(next, null, 2));
}

function setArrayItem(key: string, idx: number, val: string) {
  const arr = [...getArrayValue(key)];
  arr[idx] = val;
  const next = { ...data.value, [key]: arr };
  emit('update:modelValue', JSON.stringify(next, null, 2));
}

function addArrayItem(key: string) {
  const arr = [...getArrayValue(key), ''];
  const next = { ...data.value, [key]: arr };
  emit('update:modelValue', JSON.stringify(next, null, 2));
}

function removeArrayItem(key: string, idx: number) {
  const arr = [...getArrayValue(key)];
  arr.splice(idx, 1);
  const next = { ...data.value, [key]: arr };
  emit('update:modelValue', JSON.stringify(next, null, 2));
}
</script>
