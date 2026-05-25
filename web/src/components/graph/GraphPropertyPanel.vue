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
        <q-input v-model="selectedNode.instruction" dense outlined autogrow type="textarea" label="指令" @update:model-value="notifyChange" />
        <q-input v-model="selectedNode.modelName" dense outlined label="模型名称" v-if="selectedNode.type === 'llm' || selectedNode.type === 'agent'" @update:model-value="notifyChange" />
        <q-input v-model="selectedNode.agentName" dense outlined label="Agent 名称" v-if="selectedNode.type === 'agent'" @update:model-value="notifyChange" />
        <q-select
          v-model="selectedNode.toolNames"
          dense
          outlined
          multiple
          use-chips
          label="工具列表"
          :options="availableTools"
          v-if="selectedNode.type === 'tool' || selectedNode.type === 'agent'"
          @update:model-value="notifyChange"
        />
        <q-input v-model="selectedNode.funcRef" dense outlined label="函数引用 (funcRef)" v-if="selectedNode.type === 'function' || selectedNode.type === 'router'" @update:model-value="notifyChange" />
        <q-toggle v-model="selectedNode.interruptBefore" dense label="执行前中断 (HITL)" @update:model-value="notifyChange" />
        <q-toggle v-model="selectedNode.interruptAfter" dense label="执行后中断 (HITL)" @update:model-value="notifyChange" />

        <div v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'" class="graph-property-panel__section">
          <div class="graph-property-panel__section-title">RetryPolicy</div>
          <q-input
            v-model.number="selectedNode.retryMaxAttempts"
            dense
            outlined
            type="number"
            label="重试次数 max_attempts"
            min="0"
            @update:model-value="notifyChange"
          />
          <q-select
            v-model="selectedNode.failureAction"
            dense
            outlined
            emit-value
            map-options
            label="失败策略 failure_action"
            :options="failureActionOptions"
            @update:model-value="notifyChange"
          />
          <q-input v-model="selectedNode.fallbackAgent" dense outlined label="Fallback Agent" @update:model-value="notifyChange" />
        </div>

        <div v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'" class="graph-property-panel__section">
          <div class="graph-property-panel__section-title">Destinations</div>
          <q-select
            v-model="selectedNode.destinations"
            dense
            outlined
            multiple
            use-chips
            emit-value
            map-options
            label="GoTo 目标节点"
            :options="destinationOptions"
            @update:model-value="notifyChange"
          />
        </div>

        <div v-if="selectedNode.type === 'agent'" class="graph-property-panel__section">
          <div class="graph-property-panel__section-title">Mapper</div>
          <q-input
            v-model="selectedNode.inputMapperJson"
            dense
            outlined
            autogrow
            type="textarea"
            label="Input Mapper JSON"
            hint='例：{"messages":"messages"}'
            @update:model-value="notifyChange"
          />
          <q-input
            v-model="selectedNode.outputMapperJson"
            dense
            outlined
            autogrow
            type="textarea"
            label="Output Mapper JSON"
            @update:model-value="notifyChange"
          />
          <q-toggle v-model="selectedNode.isolatedMessages" dense label="隔离子 Agent 消息" @update:model-value="notifyChange" />
          <q-toggle v-model="selectedNode.inputFromLastResponse" dense label="从 last_response 注入输入" @update:model-value="notifyChange" />
        </div>

        <q-expansion-item dense expand-separator label="缓存" header-class="text-caption text-weight-bold">
          <div class="q-gutter-sm q-pt-xs">
            <q-toggle v-model="selectedNode.cacheEnabled" dense label="启用节点缓存" />
            <q-input
              v-if="selectedNode.cacheEnabled"
              v-model.number="selectedNode.cacheTtlSeconds"
              dense
              outlined
              type="number"
              label="缓存 TTL（秒）"
              min="0"
            />
          </div>
        </q-expansion-item>
      </div>
    </template>
    <template v-else-if="graphDef">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">Graph 属性</div>
      </div>
      <q-separator />
      <div class="graph-property-panel__body q-gutter-sm">
        <q-input v-model="graphDef.name" dense outlined label="Graph 名称" @update:model-value="notifyChange" />
        <q-input v-model="graphDef.description" dense outlined autogrow type="textarea" label="描述" @update:model-value="notifyChange" />
        <q-select v-model="graphDef.entryPoint" dense outlined emit-value map-options label="入口节点" :options="nodeIdOptions" @update:model-value="notifyChange" />
        <q-select v-model="graphDef.finishPoint" dense outlined emit-value map-options label="结束节点" :options="nodeIdOptions" @update:model-value="notifyChange" />
        <q-select v-model="graphDef.executionEngine" dense outlined emit-value map-options label="执行引擎" :options="engineOptions" @update:model-value="notifyChange" />
        <q-toggle v-model="graphDef.enableCheckpoint" dense label="启用检查点" @update:model-value="notifyChange" />
        <div v-if="graphDef.version > 0" class="text-caption text-grey-7">当前版本 v{{ graphDef.version }}</div>
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
      <GraphValidationPanel
        v-if="validationIssues.length"
        :issues="validationIssues"
        :valid="validationValid"
        @select-node="$emit('selectNode', $event)"
      />
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
import GraphValidationPanel from "./GraphValidationPanel.vue";
import type { NodeDef, GraphDefinition, ReducerType, NodeType, ValidationError, ValidationWarning } from "../../features/graph/types";
import { NODE_TYPE_STYLES, REDUCER_OPTIONS, STATE_FIELD_TYPE_OPTIONS, ENGINE_OPTIONS, FAILURE_ACTION_OPTIONS } from "../../features/graph/types";

const props = defineProps<{
  selectedNode: NodeDef | null;
  graphDef: GraphDefinition | null;
  availableTools: string[];
  isDark: boolean;
  validationErrors?: ValidationError[];
  validationWarnings?: ValidationWarning[];
  validationValid?: boolean;
}>();

const emit = defineEmits<{
  deselect: [];
  selectNode: [nodeId: string | null];
  change: [];
}>();

function notifyChange() {
  emit("change");
}

const validationIssues = computed(() => [
  ...(props.validationErrors ?? []).map((issue) => ({ ...issue, warning: false as const })),
  ...(props.validationWarnings ?? []).map((issue) => ({ ...issue, warning: true as const })),
]);

const validationValid = computed(() => props.validationValid ?? true);

const nodeTypeOptions = computed(() =>
  Object.entries(NODE_TYPE_STYLES).map(([key, val]) => ({ label: val.label, value: key }))
);

const nodeIdOptions = computed(() =>
  (props.graphDef?.nodes ?? []).map((n) => ({ label: n.id, value: n.id }))
);

const destinationOptions = computed(() =>
  (props.graphDef?.nodes ?? [])
    .filter((n) => n.id !== props.selectedNode?.id)
    .map((n) => ({ label: `${n.id} (${n.type})`, value: n.id }))
);

const fieldTypeOptions = STATE_FIELD_TYPE_OPTIONS;
const reducerOptions = REDUCER_OPTIONS;
const engineOptions = ENGINE_OPTIONS;
const failureActionOptions = FAILURE_ACTION_OPTIONS;

function onTypeChange(type: string) {
  if (props.selectedNode) {
    props.selectedNode.type = type as NodeType;
    notifyChange();
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
    notifyChange();
  }
}

function removeStateField(index: number) {
  props.graphDef?.stateFields.splice(index, 1);
  notifyChange();
}
</script>
