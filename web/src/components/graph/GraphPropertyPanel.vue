<template>
  <div :class="['graph-property-panel', { 'is-dark': isDark }]" :style="panelAccentStyle">
    <template v-if="selectedNode">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">节点属性</div>
        <div class="row items-center q-gutter-xs">
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            size="sm"
            icon="settings"
            label="Graph 全局设置"
            @click="$emit('deselect')"
          />
          <q-btn flat dense round icon="close" size="sm" @click="$emit('deselect')" />
        </div>
      </div>
      <q-separator />

      <q-expansion-item
        dense
        expand-separator
        label="基本信息"
        default-opened
        class="graph-property-panel__group graph-property-panel__group--basic"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-input :model-value="selectedNode.id" dense outlined label="节点 ID" disable />
          <q-select
            :model-value="selectedNode.type"
            dense
            outlined
            emit-value
            map-options
            label="节点类型"
            :options="nodeTypeOptions"
            @update:model-value="(v: string) => updateNodeField('type', v as NodeType)"
          />
          <q-input
            :model-value="selectedNode.description"
            dense
            outlined
            autogrow
            type="textarea"
            label="节点描述"
            @update:model-value="(v: string | number | null) => updateNodeField('description', String(v ?? ''))"
          />
          <GraphVariablePicker
            :model-value="selectedNode.instruction"
            label="指令"
            :nodes="allNodes ?? []"
            :state-fields="stateFields ?? []"
            @update:model-value="(v: string) => updateNodeField('instruction', v)"
          />
          <q-input
            v-if="selectedNode.type === 'function' || selectedNode.type === 'router'"
            :model-value="selectedNode.funcRef"
            dense
            outlined
            label="函数引用 (funcRef)"
            @update:model-value="(v: string | number | null) => updateNodeField('funcRef', String(v ?? ''))"
          />
        </div>
      </q-expansion-item>

      <q-expansion-item
        v-if="selectedNode.type === 'router'"
        dense
        expand-separator
        label="条件路由"
        default-opened
        class="graph-property-panel__group graph-property-panel__group--conditional"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <template v-if="routerConditionalEdges.length > 0">
            <div v-for="(ce, ceIdx) in routerConditionalEdges" :key="ceIdx" class="graph-property-panel__section">
              <div class="graph-property-panel__section-title">
                路由 #{{ ceIdx + 1
                }}<span v-if="ce.condFuncRef" class="text-grey-7 q-ml-xs">({{ ce.condFuncRef }})</span>
              </div>
              <q-input
                :model-value="ce.condFuncRef"
                dense
                outlined
                label="条件函数 (condFuncRef)"
                @update:model-value="(v: string | number | null) => updateCondFuncRef(ceIdx, String(v ?? ''))"
              />
              <div v-for="(target, label) in ce.pathMap" :key="label" class="row q-col-gutter-xs items-center q-mb-xs">
                <q-input
                  :model-value="label"
                  class="col-5"
                  dense
                  outlined
                  label="标签"
                  @update:model-value="(v: string | number | null) => updatePathMapLabel(ceIdx, label, String(v ?? ''))"
                />
                <q-select
                  :model-value="target"
                  class="col-5"
                  dense
                  outlined
                  emit-value
                  map-options
                  label="目标节点"
                  :options="destinationOptions"
                  @update:model-value="(v: string) => updatePathMapTarget(ceIdx, label, v)"
                />
                <q-btn
                  class="col-2"
                  flat
                  dense
                  round
                  color="negative"
                  icon="delete"
                  size="sm"
                  @click="removePathMapEntry(ceIdx, label)"
                />
              </div>
              <q-btn
                flat
                dense
                color="primary"
                icon="add"
                label="添加路由分支"
                size="sm"
                @click="addPathMapEntry(ceIdx)"
              />
            </div>
          </template>
          <div v-else class="text-caption text-grey-7">暂无条件路由，点击下方添加</div>
          <q-btn flat dense color="primary" icon="add" label="添加条件路由" size="sm" @click="addConditionalEdge" />
        </div>
      </q-expansion-item>

      <q-expansion-item
        v-if="selectedNode.type === 'llm' || selectedNode.type === 'agent' || selectedNode.type === 'tool'"
        dense
        expand-separator
        label="模型与 Agent"
        default-opened
        class="graph-property-panel__group graph-property-panel__group--model"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-input
            v-if="selectedNode.type === 'llm' || selectedNode.type === 'agent'"
            :model-value="selectedNode.modelName"
            dense
            outlined
            label="模型名称"
            @update:model-value="(v: string | number | null) => updateNodeField('modelName', String(v ?? ''))"
          />
          <q-input
            v-if="selectedNode.type === 'agent'"
            :model-value="selectedNode.agentName"
            dense
            outlined
            label="Agent 名称"
            @update:model-value="(v: string | number | null) => updateNodeField('agentName', String(v ?? ''))"
          />
          <q-select
            v-if="selectedNode.type === 'tool' || selectedNode.type === 'agent'"
            :model-value="selectedNode.toolNames"
            dense
            outlined
            multiple
            use-chips
            label="工具列表"
            :options="availableTools"
            @update:model-value="(v: string[]) => updateNodeField('toolNames', v)"
          />
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        label="中断与审批"
        class="graph-property-panel__group graph-property-panel__group--interrupt"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-toggle
            :model-value="selectedNode.interruptBefore"
            dense
            label="执行前中断 (HITL)"
            @update:model-value="(v: boolean) => updateNodeField('interruptBefore', v)"
          />
          <q-toggle
            :model-value="selectedNode.interruptAfter"
            dense
            label="执行后中断 (HITL)"
            @update:model-value="(v: boolean) => updateNodeField('interruptAfter', v)"
          />
          <template v-if="selectedNode.type === 'hitl' || selectedNode.type === 'agent'">
            <q-separator class="q-my-xs" />
            <div class="graph-property-panel__section-title">审批与指派</div>
            <q-input
              :model-value="selectedNode.requiredRole"
              dense
              outlined
              label="所需角色 (requiredRole)"
              @update:model-value="(v: string | number | null) => updateNodeField('requiredRole', String(v ?? ''))"
            />
            <q-select
              :model-value="selectedNode.assignmentMode"
              dense
              outlined
              emit-value
              map-options
              label="指派模式 (assignmentMode)"
              :options="assignmentModeOptions"
              @update:model-value="(v: string) => updateNodeField('assignmentMode', v)"
            />
            <q-input
              :model-value="selectedNode.assignmentStrategy"
              dense
              outlined
              label="指派策略 (assignmentStrategy)"
              @update:model-value="
                (v: string | number | null) => updateNodeField('assignmentStrategy', String(v ?? ''))
              "
            />
            <q-input
              :model-value="selectedNode.reviewerAgent"
              dense
              outlined
              label="审核 Agent (reviewerAgent)"
              @update:model-value="(v: string | number | null) => updateNodeField('reviewerAgent', String(v ?? ''))"
            />
            <q-input
              :model-value="selectedNode.reviewRules"
              dense
              outlined
              autogrow
              type="textarea"
              label="审核规则 (reviewRules)"
              @update:model-value="(v: string | number | null) => updateNodeField('reviewRules', String(v ?? ''))"
            />
          </template>
          <template v-if="selectedNode.type === 'hitl' || selectedNode.type === 'agent'">
            <q-separator class="q-my-xs" />
            <div class="graph-property-panel__section-title">超时与心跳</div>
            <q-input
              :model-value="selectedNode.timeoutSeconds"
              dense
              outlined
              type="number"
              label="超时秒数 (timeoutSeconds)"
              min="0"
              @update:model-value="(v: string | number | null) => updateNodeField('timeoutSeconds', Number(v ?? 0))"
            />
            <q-input
              :model-value="selectedNode.heartbeatIntervalSeconds"
              dense
              outlined
              type="number"
              label="心跳间隔秒 (heartbeatInterval)"
              min="0"
              @update:model-value="
                (v: string | number | null) => updateNodeField('heartbeatIntervalSeconds', Number(v ?? 0))
              "
            />
            <q-toggle
              :model-value="selectedNode.enableLeaseExtension"
              dense
              label="启用租约延期 (enableLeaseExtension)"
              @update:model-value="(v: boolean) => updateNodeField('enableLeaseExtension', v)"
            />
          </template>
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        label="高级选项"
        class="graph-property-panel__group graph-property-panel__group--advanced"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <div
            v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'"
            class="graph-property-panel__section"
          >
            <div class="graph-property-panel__section-title">RetryPolicy</div>
            <q-input
              :model-value="selectedNode.retryMaxAttempts"
              dense
              outlined
              type="number"
              label="重试次数 max_attempts"
              min="0"
              @update:model-value="(v: string | number | null) => updateNodeField('retryMaxAttempts', Number(v ?? 0))"
            />
            <q-select
              :model-value="selectedNode.failureAction"
              dense
              outlined
              emit-value
              map-options
              label="失败策略 failure_action"
              :options="failureActionOptions"
              @update:model-value="(v: string) => updateNodeField('failureAction', v)"
            />
            <q-input
              :model-value="selectedNode.fallbackAgent"
              dense
              outlined
              label="Fallback Agent"
              @update:model-value="(v: string | number | null) => updateNodeField('fallbackAgent', String(v ?? ''))"
            />
          </div>

          <div
            v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'"
            class="graph-property-panel__section"
          >
            <div class="graph-property-panel__section-title">Destinations</div>
            <q-select
              :model-value="selectedNode.destinations"
              dense
              outlined
              multiple
              use-chips
              emit-value
              map-options
              label="GoTo 目标节点"
              :options="destinationOptions"
              @update:model-value="(v: string[]) => updateNodeField('destinations', v)"
            />
          </div>

          <div v-if="selectedNode.type === 'agent'" class="graph-property-panel__section">
            <div class="graph-property-panel__section-title">Mapper</div>
            <q-input
              :model-value="selectedNode.inputMapperJson"
              dense
              outlined
              autogrow
              type="textarea"
              label="Input Mapper JSON"
              hint='例：{"messages":"messages"}'
              @update:model-value="(v: string | number | null) => updateNodeField('inputMapperJson', String(v ?? ''))"
            />
            <q-input
              :model-value="selectedNode.outputMapperJson"
              dense
              outlined
              autogrow
              type="textarea"
              label="Output Mapper JSON"
              @update:model-value="(v: string | number | null) => updateNodeField('outputMapperJson', String(v ?? ''))"
            />
            <q-toggle
              :model-value="selectedNode.isolatedMessages"
              dense
              label="隔离子 Agent 消息"
              @update:model-value="(v: boolean) => updateNodeField('isolatedMessages', v)"
            />
            <q-toggle
              :model-value="selectedNode.inputFromLastResponse"
              dense
              label="从 last_response 注入输入"
              @update:model-value="(v: boolean) => updateNodeField('inputFromLastResponse', v)"
            />
          </div>

          <q-expansion-item dense expand-separator label="缓存" header-class="text-caption text-weight-bold">
            <div class="q-gutter-sm q-pt-xs">
              <q-toggle
                :model-value="selectedNode.cacheEnabled"
                dense
                label="启用节点缓存"
                @update:model-value="(v: boolean) => updateNodeField('cacheEnabled', v)"
              />
              <q-input
                v-if="selectedNode.cacheEnabled"
                :model-value="selectedNode.cacheTtlSeconds"
                dense
                outlined
                type="number"
                label="缓存 TTL（秒）"
                min="0"
                @update:model-value="(v: string | number | null) => updateNodeField('cacheTtlSeconds', Number(v ?? 0))"
              />
            </div>
          </q-expansion-item>
        </div>
      </q-expansion-item>
    </template>

    <template v-else-if="graphDef">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">Graph 属性</div>
      </div>
      <q-separator />

      <q-expansion-item
        dense
        expand-separator
        label="Graph 属性"
        default-opened
        class="graph-property-panel__group"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-input
            :model-value="graphDef.name"
            dense
            outlined
            label="Graph 名称"
            @update:model-value="(v: string | number | null) => updateGraphField('name', String(v ?? ''))"
          />
          <q-input
            :model-value="graphDef.description"
            dense
            outlined
            autogrow
            type="textarea"
            label="描述"
            @update:model-value="(v: string | number | null) => updateGraphField('description', String(v ?? ''))"
          />
          <q-select
            :model-value="graphDef.entryPoint"
            dense
            outlined
            emit-value
            map-options
            label="入口节点"
            :options="nodeIdOptions"
            @update:model-value="(v: string) => updateGraphField('entryPoint', v)"
          />
          <q-select
            :model-value="graphDef.finishPoint"
            dense
            outlined
            emit-value
            map-options
            label="结束节点"
            :options="nodeIdOptions"
            @update:model-value="(v: string) => updateGraphField('finishPoint', v)"
          />
          <q-select
            :model-value="graphDef.executionEngine"
            dense
            outlined
            emit-value
            map-options
            label="执行引擎"
            :options="engineOptions"
            @update:model-value="(v: string) => updateGraphField('executionEngine', v)"
          />
          <q-toggle
            :model-value="graphDef.enableCheckpoint"
            dense
            label="启用检查点"
            @update:model-value="(v: boolean) => updateGraphField('enableCheckpoint', v)"
          />
          <div v-if="graphDef.version > 0" class="text-caption text-grey-7">当前版本 v{{ graphDef.version }}</div>
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        label="State Schema"
        class="graph-property-panel__group"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body">
          <div v-for="(field, idx) in graphDef.stateFields" :key="idx" class="state-field-row q-mb-sm">
            <div class="row q-col-gutter-xs items-center">
              <q-input
                :model-value="field.name"
                class="col-5"
                dense
                outlined
                label="字段名"
                @update:model-value="(v: string | number | null) => updateStateField(idx, 'name', String(v ?? ''))"
              />
              <q-select
                :model-value="field.type"
                class="col-3"
                dense
                outlined
                emit-value
                map-options
                label="类型"
                :options="fieldTypeOptions"
                @update:model-value="(v: string) => updateStateField(idx, 'type', v)"
              />
              <q-select
                :model-value="field.reducer"
                class="col-3"
                dense
                outlined
                emit-value
                map-options
                label="Reducer"
                :options="reducerOptions"
                @update:model-value="(v: string) => updateStateField(idx, 'reducer', v)"
              />
              <q-btn
                class="col-1"
                flat
                dense
                round
                color="negative"
                icon="delete"
                size="sm"
                @click="removeStateField(idx)"
              />
            </div>
          </div>
          <q-btn flat dense color="primary" icon="add" label="添加字段" @click="addStateField" />
        </div>
      </q-expansion-item>

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
import { computed } from 'vue';
import GraphValidationPanel from './GraphValidationPanel.vue';
import type {
  NodeDef,
  GraphDefinition,
  ReducerType,
  NodeType,
  ValidationError,
  ValidationWarning,
  StateFieldDef,
} from '../../features/graph/types';
import {
  NODE_TYPE_STYLES,
  REDUCER_OPTIONS,
  STATE_FIELD_TYPE_OPTIONS,
  ENGINE_OPTIONS,
  FAILURE_ACTION_OPTIONS,
} from '../../features/graph/types';
import type { useGraphUndoRedo } from '../../features/graph/useGraphUndoRedo';
import { useConditionalRoutes } from '../../features/graph/useConditionalRoutes';
import GraphVariablePicker from './GraphVariablePicker.vue';

const props = defineProps<{
  selectedNode: NodeDef | null;
  graphDef: GraphDefinition | null;
  availableTools: string[];
  isDark: boolean;
  validationErrors?: ValidationError[];
  validationWarnings?: ValidationWarning[];
  validationValid?: boolean;
  undoRedo?: ReturnType<typeof useGraphUndoRedo>;
  allNodes?: NodeDef[];
  stateFields?: StateFieldDef[];
}>();

const emit = defineEmits<{
  deselect: [];
  selectNode: [nodeId: string | null];
  change: [];
}>();

function notifyChange() {
  emit('change');
}

function updateNodeField<K extends keyof NodeDef>(field: K, value: NodeDef[K]) {
  if (props.selectedNode) {
    const oldValue = props.selectedNode[field];
    if (props.undoRedo) {
      props.undoRedo.pushSetProperty(props.selectedNode.id, field, oldValue, value);
    } else {
      props.selectedNode[field] = value;
      notifyChange();
    }
  }
}

function updateGraphField<K extends keyof GraphDefinition>(field: K, value: GraphDefinition[K]) {
  if (props.graphDef) {
    const oldValue = props.graphDef[field];
    if (props.undoRedo) {
      props.undoRedo.pushSetGraphProperty(field, oldValue, value);
    } else {
      props.graphDef[field] = value;
      notifyChange();
    }
  }
}

function updateStateField<K extends keyof StateFieldDef>(idx: number, field: K, value: StateFieldDef[K]) {
  if (props.graphDef && props.graphDef.stateFields[idx]) {
    const oldValue = props.graphDef.stateFields[idx][field];
    if (props.undoRedo) {
      props.undoRedo.pushSetStateProperty(idx, field, oldValue, value);
    } else {
      props.graphDef.stateFields[idx][field] = value;
      notifyChange();
    }
  }
}

const validationIssues = computed(() => [
  ...(props.validationErrors ?? []).map((issue) => ({ ...issue, warning: false as const })),
  ...(props.validationWarnings ?? []).map((issue) => ({ ...issue, warning: true as const })),
]);

const validationValid = computed(() => props.validationValid ?? true);

const panelAccentStyle = computed(() => {
  if (!props.selectedNode) return {};
  const style = NODE_TYPE_STYLES[props.selectedNode.type as NodeType];
  return style ? { '--node-accent': style.borderColor } : {};
});

const nodeTypeOptions = computed(() =>
  Object.entries(NODE_TYPE_STYLES).map(([key, val]) => ({ label: val.label, value: key })),
);

const nodeIdOptions = computed(() => (props.graphDef?.nodes ?? []).map((n) => ({ label: n.id, value: n.id })));

const destinationOptions = computed(() =>
  (props.graphDef?.nodes ?? [])
    .filter((n) => n.id !== props.selectedNode?.id)
    .map((n) => ({ label: `${n.id} (${n.type})`, value: n.id })),
);

const fieldTypeOptions = STATE_FIELD_TYPE_OPTIONS;
const reducerOptions = REDUCER_OPTIONS;
const engineOptions = ENGINE_OPTIONS;
const failureActionOptions = FAILURE_ACTION_OPTIONS;
const assignmentModeOptions = [
  { label: '自动指派', value: 'auto' },
  { label: '手动指派', value: 'manual' },
  { label: '轮询指派', value: 'round_robin' },
  { label: '最少任务优先', value: 'least_busy' },
];

const {
  routerConditionalEdges,
  updateCondFuncRef,
  updatePathMapLabel,
  updatePathMapTarget,
  removePathMapEntry,
  addPathMapEntry,
  addConditionalEdge,
} = useConditionalRoutes(
  computed(() => props.graphDef),
  computed(() => props.selectedNode?.id ?? null),
  computed(() => props.undoRedo),
  notifyChange,
  destinationOptions,
);

function addStateField() {
  if (props.graphDef) {
    const field = {
      name: '',
      type: 'string' as const,
      reducer: 'replace' as ReducerType,
      required: false,
      disableDeepCopy: false,
    };
    const idx = props.graphDef.stateFields.length;
    props.graphDef.stateFields.push(field);
    if (props.undoRedo) {
      props.undoRedo.pushAddStateField(field, idx);
    } else {
      notifyChange();
    }
  }
}

function removeStateField(index: number) {
  if (props.graphDef) {
    const field = props.graphDef.stateFields[index];
    if (field) {
      props.graphDef.stateFields.splice(index, 1);
      if (props.undoRedo) {
        props.undoRedo.pushRemoveStateField(field, index);
      } else {
        notifyChange();
      }
    }
  }
}
</script>
