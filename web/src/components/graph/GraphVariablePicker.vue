<template>
  <q-input
    ref="inputRef"
    :model-value="modelValue"
    dense
    outlined
    :label="label"
    autogrow
    type="textarea"
    class="app-field-md"
    @update:model-value="(v: string | number | null) => emit('update:modelValue', String(v ?? ''))"
  >
    <template #append>
      <q-btn flat dense round icon="data_object" size="sm" @click="showPicker = !showPicker">
        <q-tooltip>插入变量</q-tooltip>
      </q-btn>
    </template>
  </q-input>
  <q-menu v-model="showPicker" anchor="bottom right" self="top right" :offset="[0, 4]">
    <q-list dense class="app-menu-min-200 graph-variable-picker">
      <q-item-label header>节点输出</q-item-label>
      <q-item v-for="node in nodes" :key="node.id" clickable @click="insertVariable(node.id, 'output')">
        <q-item-section side>
          <q-badge :color="nodeTypeColor(node.type)" :label="node.type" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ graphNodeDisplayLabel(node) }}</q-item-label>
          <q-item-label caption>{{ '{{' + node.id + '.output}' + '}' }}</q-item-label>
        </q-item-section>
      </q-item>
      <template v-if="stateFields.length > 0">
        <q-separator />
        <q-item-label header>状态字段</q-item-label>
        <q-item v-for="field in stateFields" :key="field.name" clickable @click="insertVariable('state', field.name)">
          <q-item-section side>
            <q-badge color="grey" label="state" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ field.name }}</q-item-label>
            <q-item-label caption>{{ '{{state.' + field.name + '}' + '}' }}</q-item-label>
          </q-item-section>
        </q-item>
      </template>
    </q-list>
  </q-menu>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { NodeDef, StateFieldDef } from '../../features/graph/types';
import { graphNodeDisplayLabel } from '../../features/orchestration/teamNodeDisplay';

const props = defineProps<{
  modelValue: string;
  label?: string;
  nodes: NodeDef[];
  stateFields: StateFieldDef[];
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const showPicker = ref(false);
const inputRef = ref<{ input?: HTMLInputElement } | null>(null);

function nodeTypeColor(type: string): string {
  const map: Record<string, string> = {
    llm: 'blue',
    agent: 'green',
    tool: 'amber',
    function: 'purple',
    router: 'grey',
    join: 'purple',
    hitl: 'orange',
  };
  return map[type] ?? 'grey';
}

function insertVariable(source: string, field: string) {
  const variable = `{{${source}.${field}}}`;
  const el = inputRef.value?.input;
  if (el) {
    const start = el.selectionStart ?? props.modelValue.length;
    const end = el.selectionEnd ?? start;
    const before = props.modelValue.slice(0, start);
    const after = props.modelValue.slice(end);
    emit('update:modelValue', before + variable + after);
  } else {
    emit('update:modelValue', props.modelValue + variable);
  }
  showPicker.value = false;
}
</script>
