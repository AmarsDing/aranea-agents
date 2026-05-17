<template>
  <div :class="['graph-property-panel', { 'is-dark': isDark }]">
    <template v-if="selectedNode">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">节点属性</div>
        <q-btn flat dense round icon="close" size="sm" @click="$emit('deselect')" />
      </div>
      <q-separator />
      <div class="graph-property-panel__body q-gutter-sm">
        <q-input v-model="selectedNode.id" dense outlined label="节点 ID" disable />
        <q-select v-model="selectedNode.type" dense outlined emit-value map-options label="节点类型" :options="nodeTypeOptions" @update:model-value="onTypeChange" />
        <q-input v-model="selectedNode.instruction" dense outlined autogrow type="textarea" label="指令" />
        <q-input v-model="selectedNode.modelName" dense outlined label="模型名称" v-if="selectedNode.type === 'llm' || selectedNode.type === 'agent'" />
        <q-input v-model="selectedNode.agentName" dense outlined label="Agent 名称" v-if="selectedNode.type === 'agent'" />
        <q-select
          v-model="selectedNode.toolNames"
          dense
          outlined
          multiple
          use-chips
          label="工具列表"
          :options="availableTools"
          v-if="selectedNode.type === 'tool' || selectedNode.type === 'agent'"
        />
        <q-input v-model="selectedNode.funcRef" dense outlined label="函数引用 (funcRef)" v-if="selectedNode.type === 'function' || selectedNode.type === 'router'" />
        <q-toggle v-model="selectedNode.interruptBefore" dense label="执行前中断 (HITL)" />
        <q-toggle v-model="selectedNode.interruptAfter" dense label="执行后中断 (HITL)" />
      </div>
    </template>
    <template v-else-if="graphDef">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">Graph 属性</div>
      </div>
      <q-separator />
      <div class="graph-property-panel__body q-gutter-sm">
        <q-input v-model="graphDef.name" dense outlined label="Graph 名称" />
        <q-input v-model="graphDef.description" dense outlined autogrow type="textarea" label="描述" />
        <q-select v-model="graphDef.entryPoint" dense outlined emit-value map-options label="入口节点" :options="nodeIdOptions" />
        <q-select v-model="graphDef.finishPoint" dense outlined emit-value map-options label="结束节点" :options="nodeIdOptions" />
        <q-select v-model="graphDef.executionEngine" dense outlined emit-value map-options label="执行引擎" :options="engineOptions" />
        <q-toggle v-model="graphDef.enableCheckpoint" dense label="启用检查点" />
      </div>
      <q-separator class="q-my-md" />
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">State Schema</div>
      </div>
      <div class="graph-property-panel__body">
        <div v-for="(field, idx) in graphDef.stateFields" :key="idx" class="state-field-row q-mb-sm">
          <div class="row q-col-gutter-xs items-center">
            <q-input v-model="field.name" class="col-5" dense outlined label="字段名" />
            <q-select v-model="field.type" class="col-3" dense outlined emit-value map-options label="类型" :options="fieldTypeOptions" />
            <q-select v-model="field.reducer" class="col-3" dense outlined emit-value map-options label="Reducer" :options="reducerOptions" />
            <q-btn class="col-1" flat dense round color="negative" icon="delete" size="sm" @click="removeStateField(idx)" />
          </div>
        </div>
        <q-btn flat dense color="primary" icon="add" label="添加字段" @click="addStateField" />
      </div>
    </template>
    <template v-else>
      <div class="graph-property-panel__empty">
        <q-icon name="touch_app" size="32px" color="grey-6" />
        <div class="text-caption text-grey-7 q-mt-sm">选中节点查看属性，或编辑 Graph 全局设置</div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { NodeDef, GraphDefinition, ReducerType, NodeType } from "../../features/graph/types";
import { NODE_TYPE_STYLES, REDUCER_OPTIONS, STATE_FIELD_TYPE_OPTIONS, ENGINE_OPTIONS } from "../../features/graph/types";

const props = defineProps<{
  selectedNode: NodeDef | null;
  graphDef: GraphDefinition | null;
  availableTools: string[];
  isDark: boolean;
}>();

const emit = defineEmits<{
  deselect: [];
}>();

const nodeTypeOptions = computed(() =>
  Object.entries(NODE_TYPE_STYLES).map(([key, val]) => ({ label: val.label, value: key }))
);

const nodeIdOptions = computed(() =>
  (props.graphDef?.nodes ?? []).map((n) => ({ label: n.id, value: n.id }))
);

const fieldTypeOptions = STATE_FIELD_TYPE_OPTIONS;
const reducerOptions = REDUCER_OPTIONS;
const engineOptions = ENGINE_OPTIONS;

function onTypeChange(type: string) {
  if (props.selectedNode) {
    props.selectedNode.type = type as NodeType;
  }
}

function addStateField() {
  if (props.graphDef) {
    props.graphDef.stateFields.push({
      name: "",
      type: "string",
      reducer: "replace" as ReducerType,
      required: false,
      disableDeepCopy: false,
    });
  }
}

function removeStateField(index: number) {
  props.graphDef?.stateFields.splice(index, 1);
}
</script>

<style scoped>
.graph-property-panel {
  width: 280px;
  padding: 16px 12px;
  border-left: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  background: var(--glass-surface, rgb(255 253 245 / 65%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
  overflow-y: auto;
}

.graph-property-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.graph-property-panel__title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, var(--color-text-secondary));
}

.graph-property-panel__body {
  padding-top: 4px;
}

.graph-property-panel__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  text-align: center;
}

.state-field-row {
  padding: 6px 8px;
  border: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  border-radius: 10px;
  background: rgb(255 255 255 / 40%);
}

.graph-property-panel.is-dark {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}

.graph-property-panel.is-dark .state-field-row {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(30 41 59 / 50%);
}
</style>
