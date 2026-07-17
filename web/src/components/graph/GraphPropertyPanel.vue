<template>
  <div :class="['graph-property-panel', { 'is-dark': isDark }]" :style="panelAccentStyle">
    <template v-if="selectedNode">
      <div class="graph-property-panel__header">
        <div class="graph-property-panel__title">{{ t('graphs.propertyTitle') }}</div>
        <div class="row items-center q-gutter-xs">
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            size="sm"
            icon="settings"
            :label="t('graphs.propertyGlobalSettings')"
            @click="$emit('deselect')"
          />
          <q-btn flat dense round icon="close" size="sm" @click="$emit('deselect')" />
        </div>
      </div>
      <q-separator />

      <q-expansion-item
        dense
        expand-separator
        :label="t('graphs.propertyGroupBasic')"
        default-opened
        class="graph-property-panel__group graph-property-panel__group--basic"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-input :model-value="selectedNode.id" dense outlined :label="t('graphs.fieldNodeId')" disable />
          <q-select
            :model-value="selectedNode.type"
            dense
            outlined
            emit-value
            map-options
            :label="t('graphs.fieldNodeType')"
            :options="nodeTypeOptions"
            @update:model-value="(v: string) => updateNodeField('type', v as NodeType)"
          />
          <q-input
            :model-value="selectedNode.description"
            dense
            outlined
            autogrow
            type="textarea"
            :label="t('graphs.fieldNodeDesc')"
            @update:model-value="(v: string | number | null) => updateNodeField('description', String(v ?? ''))"
          />
          <GraphVariablePicker
            :model-value="selectedNode.instruction"
            :label="t('graphs.fieldInstruction')"
            :nodes="allNodes ?? []"
            :state-fields="stateFields ?? []"
            @update:model-value="(v: string) => updateNodeField('instruction', v)"
          />
          <q-input
            v-if="selectedNode.type === 'function' || selectedNode.type === 'router'"
            :model-value="selectedNode.funcRef"
            dense
            outlined
            :label="t('graphs.fieldFuncRefFull')"
            @update:model-value="(v: string | number | null) => updateNodeField('funcRef', String(v ?? ''))"
          />
        </div>
      </q-expansion-item>

      <q-expansion-item
        v-if="selectedNode.type === 'router'"
        dense
        expand-separator
        :label="t('graphs.groupConditionalRoute')"
        default-opened
        class="graph-property-panel__group graph-property-panel__group--conditional"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <template v-if="routerConditionalEdges.length > 0">
            <div v-for="(ce, ceIdx) in routerConditionalEdges" :key="ceIdx" class="graph-property-panel__section">
              <div class="graph-property-panel__section-title">
                {{ t('graphs.routeNumberLabel', { n: ceIdx + 1 })
                }}<span v-if="ce.condFuncRef" class="text-grey-7 q-ml-xs">({{ ce.condFuncRef }})</span>
              </div>
              <q-input
                :model-value="ce.condFuncRef"
                dense
                outlined
                :label="t('graphs.fieldCondFuncRef')"
                @update:model-value="(v: string | number | null) => updateCondFuncRef(ceIdx, String(v ?? ''))"
              />
              <div v-for="(target, label) in ce.pathMap" :key="label" class="row q-col-gutter-xs items-center q-mb-xs">
                <q-input
                  :model-value="label"
                  class="col-5"
                  dense
                  outlined
                  :label="t('graphs.fieldPathMapLabel')"
                  @update:model-value="(v: string | number | null) => updatePathMapLabel(ceIdx, label, String(v ?? ''))"
                />
                <q-select
                  :model-value="target"
                  class="col-5"
                  dense
                  outlined
                  emit-value
                  map-options
                  :label="t('graphs.fieldPathMapTarget')"
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
                :label="t('graphs.buttonAddRouteBranch')"
                size="sm"
                @click="addPathMapEntry(ceIdx)"
              />
            </div>
          </template>
          <div v-else class="text-caption text-grey-7">{{ t('graphs.emptyConditionalRoute') }}</div>
          <q-btn
            flat
            dense
            color="primary"
            icon="add"
            :label="t('graphs.buttonAddConditionalRoute')"
            size="sm"
            @click="addConditionalEdge"
          />
        </div>
      </q-expansion-item>

      <q-expansion-item
        v-if="selectedNode.type === 'llm' || selectedNode.type === 'agent' || selectedNode.type === 'tool'"
        dense
        expand-separator
        :label="t('graphs.groupModelAgent')"
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
            :label="t('graphs.fieldModelName')"
            @update:model-value="(v: string | number | null) => updateNodeField('modelName', String(v ?? ''))"
          />
          <q-input
            v-if="selectedNode.type === 'agent'"
            :model-value="selectedNode.agentName"
            dense
            outlined
            :label="t('graphs.fieldAgentName')"
            @update:model-value="(v: string | number | null) => updateNodeField('agentName', String(v ?? ''))"
          />
          <q-select
            v-if="selectedNode.type === 'tool' || selectedNode.type === 'agent'"
            :model-value="selectedNode.toolNames"
            dense
            outlined
            multiple
            use-chips
            :label="t('graphs.fieldToolNames')"
            :options="availableTools"
            @update:model-value="(v: string[]) => updateNodeField('toolNames', v)"
          />
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        :label="t('graphs.propertyGroupInterrupt')"
        class="graph-property-panel__group graph-property-panel__group--interrupt"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-toggle
            :model-value="selectedNode.interruptBefore"
            dense
            :label="t('graphs.fieldInterruptBefore')"
            @update:model-value="(v: boolean) => updateNodeField('interruptBefore', v)"
          />
          <q-toggle
            :model-value="selectedNode.interruptAfter"
            dense
            :label="t('graphs.fieldInterruptAfter')"
            @update:model-value="(v: boolean) => updateNodeField('interruptAfter', v)"
          />
          <template v-if="selectedNode.type === 'hitl' || selectedNode.type === 'agent'">
            <q-separator class="q-my-xs" />
            <div class="graph-property-panel__section-title">{{ t('graphs.sectionApprovalAssign') }}</div>
            <q-input
              :model-value="selectedNode.requiredRole"
              dense
              outlined
              :label="t('graphs.fieldRequiredRole')"
              @update:model-value="(v: string | number | null) => updateNodeField('requiredRole', String(v ?? ''))"
            />
            <q-select
              :model-value="selectedNode.assignmentMode"
              dense
              outlined
              emit-value
              map-options
              :label="t('graphs.fieldAssignmentMode')"
              :options="assignmentModeOptions"
              @update:model-value="(v: string) => updateNodeField('assignmentMode', v)"
            />
            <q-input
              :model-value="selectedNode.assignmentStrategy"
              dense
              outlined
              :label="t('graphs.fieldAssignmentStrategy')"
              @update:model-value="
                (v: string | number | null) => updateNodeField('assignmentStrategy', String(v ?? ''))
              "
            />
            <q-input
              :model-value="selectedNode.reviewerAgent"
              dense
              outlined
              :label="t('graphs.fieldReviewerAgent')"
              @update:model-value="(v: string | number | null) => updateNodeField('reviewerAgent', String(v ?? ''))"
            />
            <q-input
              :model-value="selectedNode.reviewRules"
              dense
              outlined
              autogrow
              type="textarea"
              :label="t('graphs.fieldReviewRules')"
              @update:model-value="(v: string | number | null) => updateNodeField('reviewRules', String(v ?? ''))"
            />
          </template>
          <template v-if="selectedNode.type === 'hitl' || selectedNode.type === 'agent'">
            <q-separator class="q-my-xs" />
            <div class="graph-property-panel__section-title">{{ t('graphs.sectionTimeoutHeartbeat') }}</div>
            <q-input
              :model-value="selectedNode.timeoutSeconds"
              dense
              outlined
              type="number"
              :label="t('graphs.fieldTimeoutSeconds')"
              min="0"
              @update:model-value="(v: string | number | null) => updateNodeField('timeoutSeconds', Number(v ?? 0))"
            />
            <q-input
              :model-value="selectedNode.heartbeatIntervalSeconds"
              dense
              outlined
              type="number"
              :label="t('graphs.fieldHeartbeatIntervalFull')"
              min="0"
              @update:model-value="
                (v: string | number | null) => updateNodeField('heartbeatIntervalSeconds', Number(v ?? 0))
              "
            />
            <q-toggle
              :model-value="selectedNode.enableLeaseExtension"
              dense
              :label="t('graphs.fieldEnableLeaseExtension')"
              @update:model-value="(v: boolean) => updateNodeField('enableLeaseExtension', v)"
            />
          </template>
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        :label="t('graphs.groupAdvanced')"
        class="graph-property-panel__group graph-property-panel__group--advanced"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <div
            v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'"
            class="graph-property-panel__section"
          >
            <div class="graph-property-panel__section-title">{{ t('graphs.sectionRetryPolicy') }}</div>
            <q-input
              :model-value="selectedNode.retryMaxAttempts"
              dense
              outlined
              type="number"
              :label="t('graphs.fieldRetryMaxAttempts')"
              min="0"
              @update:model-value="(v: string | number | null) => updateNodeField('retryMaxAttempts', Number(v ?? 0))"
            />
            <q-select
              :model-value="selectedNode.failureAction"
              dense
              outlined
              emit-value
              map-options
              :label="t('graphs.fieldFailureAction')"
              :options="failureActionOptions"
              @update:model-value="(v: string) => updateNodeField('failureAction', v)"
            />
            <q-input
              :model-value="selectedNode.fallbackAgent"
              dense
              outlined
              :label="t('graphs.fieldFallbackAgent')"
              @update:model-value="(v: string | number | null) => updateNodeField('fallbackAgent', String(v ?? ''))"
            />
          </div>

          <div
            v-if="selectedNode.type === 'agent' || selectedNode.type === 'router'"
            class="graph-property-panel__section"
          >
            <div class="graph-property-panel__section-title">{{ t('graphs.sectionDestinations') }}</div>
            <q-select
              :model-value="selectedNode.destinations"
              dense
              outlined
              multiple
              use-chips
              emit-value
              map-options
              :label="t('graphs.fieldGotoTargets')"
              :options="destinationOptions"
              @update:model-value="(v: string[]) => updateNodeField('destinations', v)"
            />
          </div>

          <div v-if="selectedNode.type === 'agent'" class="graph-property-panel__section">
            <div class="graph-property-panel__section-title">{{ t('graphs.sectionMapper') }}</div>
            <q-input
              :model-value="selectedNode.inputMapperJson"
              dense
              outlined
              autogrow
              type="textarea"
              :label="t('graphs.fieldInputMapperJson')"
              :hint="t('graphs.hintInputMapperExample')"
              @update:model-value="(v: string | number | null) => updateNodeField('inputMapperJson', String(v ?? ''))"
            />
            <q-input
              :model-value="selectedNode.outputMapperJson"
              dense
              outlined
              autogrow
              type="textarea"
              :label="t('graphs.fieldOutputMapperJson')"
              @update:model-value="(v: string | number | null) => updateNodeField('outputMapperJson', String(v ?? ''))"
            />
            <q-toggle
              :model-value="selectedNode.isolatedMessages"
              dense
              :label="t('graphs.fieldIsolatedMessages')"
              @update:model-value="(v: boolean) => updateNodeField('isolatedMessages', v)"
            />
            <q-toggle
              :model-value="selectedNode.inputFromLastResponse"
              dense
              :label="t('graphs.fieldInputFromLastResponse')"
              @update:model-value="(v: boolean) => updateNodeField('inputFromLastResponse', v)"
            />
          </div>

          <q-expansion-item
            dense
            expand-separator
            :label="t('graphs.propertyGroupCache')"
            header-class="text-caption text-weight-bold"
          >
            <div class="q-gutter-sm q-pt-xs">
              <q-toggle
                :model-value="selectedNode.cacheEnabled"
                dense
                :label="t('graphs.fieldEnableNodeCache')"
                @update:model-value="(v: boolean) => updateNodeField('cacheEnabled', v)"
              />
              <q-input
                v-if="selectedNode.cacheEnabled"
                :model-value="selectedNode.cacheTtlSeconds"
                dense
                outlined
                type="number"
                :label="t('graphs.fieldCacheTtl')"
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
        <div class="graph-property-panel__title">{{ t('graphs.propertyGroupGraph') }}</div>
      </div>
      <q-separator />

      <q-expansion-item
        dense
        expand-separator
        :label="t('graphs.propertyGroupGraph')"
        default-opened
        class="graph-property-panel__group"
        header-class="graph-property-panel__group-header"
      >
        <div class="graph-property-panel__group-body q-gutter-sm">
          <q-input
            :model-value="graphDef.name"
            dense
            outlined
            :label="t('graphs.fieldGraphName')"
            @update:model-value="(v: string | number | null) => updateGraphField('name', String(v ?? ''))"
          />
          <q-input
            :model-value="graphDef.description"
            dense
            outlined
            autogrow
            type="textarea"
            :label="t('graphs.fieldDescription')"
            @update:model-value="(v: string | number | null) => updateGraphField('description', String(v ?? ''))"
          />
          <q-select
            :model-value="graphDef.entryPoint"
            dense
            outlined
            emit-value
            map-options
            :label="t('graphs.fieldEntryNode')"
            :options="nodeIdOptions"
            @update:model-value="(v: string) => updateGraphField('entryPoint', v)"
          />
          <q-select
            :model-value="graphDef.finishPoint"
            dense
            outlined
            emit-value
            map-options
            :label="t('graphs.fieldFinishNode')"
            :options="nodeIdOptions"
            @update:model-value="(v: string) => updateGraphField('finishPoint', v)"
          />
          <q-select
            :model-value="graphDef.executionEngine"
            dense
            outlined
            emit-value
            map-options
            :label="t('graphs.fieldExecutionEngine')"
            :options="engineOptions"
            @update:model-value="(v: string) => updateGraphField('executionEngine', v)"
          />
          <q-toggle
            :model-value="graphDef.enableCheckpoint"
            dense
            :label="t('graphs.fieldEnableCheckpoint')"
            @update:model-value="(v: boolean) => updateGraphField('enableCheckpoint', v)"
          />
          <div v-if="graphDef.version > 0" class="text-caption text-grey-7">
            {{ t('graphs.fieldCurrentVersion', { version: graphDef.version }) }}
          </div>
        </div>
      </q-expansion-item>

      <q-expansion-item
        dense
        expand-separator
        :label="t('graphs.propertyGroupStateSchema')"
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
                :label="t('graphs.stateFieldColumnName')"
                @update:model-value="(v: string | number | null) => updateStateField(idx, 'name', String(v ?? ''))"
              />
              <q-select
                :model-value="field.type"
                class="col-3"
                dense
                outlined
                emit-value
                map-options
                :label="t('graphs.stateFieldColumnType')"
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
                :label="t('graphs.stateFieldColumnReducer')"
                :options="reducerOptions"
                @update:model-value="(v: ReducerType) => updateStateField(idx, 'reducer', v)"
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
          <q-btn
            flat
            dense
            color="primary"
            icon="add"
            :label="t('graphs.stateFieldAddButton')"
            @click="addStateField"
          />
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
        <div class="text-caption text-grey-7 q-mt-sm">{{ t('graphs.propertyEmptyHint') }}</div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
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

const { t } = useI18n();

const selectedNode = defineModel<NodeDef | null>('selectedNode', { required: true });
const graphDef = defineModel<GraphDefinition | null>('graphDef', { required: true });

const props = defineProps<{
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
  if (selectedNode.value) {
    const oldValue = selectedNode.value[field];
    if (props.undoRedo) {
      props.undoRedo.pushSetProperty(selectedNode.value.id, field, oldValue, value);
    } else {
      selectedNode.value[field] = value;
      notifyChange();
    }
  }
}

function updateGraphField<K extends keyof GraphDefinition>(field: K, value: GraphDefinition[K]) {
  if (graphDef.value) {
    const oldValue = graphDef.value[field];
    if (props.undoRedo) {
      props.undoRedo.pushSetGraphProperty(field, oldValue, value);
    } else {
      graphDef.value[field] = value;
      notifyChange();
    }
  }
}

function updateStateField<K extends keyof StateFieldDef>(idx: number, field: K, value: StateFieldDef[K]) {
  if (graphDef.value && graphDef.value.stateFields[idx]) {
    const oldValue = graphDef.value.stateFields[idx][field];
    if (props.undoRedo) {
      props.undoRedo.pushSetStateProperty(idx, field, oldValue, value);
    } else {
      graphDef.value.stateFields[idx][field] = value;
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
  if (!selectedNode.value) return {};
  const style = NODE_TYPE_STYLES[selectedNode.value.type as NodeType];
  return style ? { '--node-accent': style.borderColor } : {};
});

const nodeTypeOptions = computed(() =>
  Object.entries(NODE_TYPE_STYLES).map(([key, val]) => ({ label: t(val.labelKey), value: key })),
);

const nodeIdOptions = computed(() => (graphDef.value?.nodes ?? []).map((n) => ({ label: n.id, value: n.id })));

const destinationOptions = computed(() =>
  (graphDef.value?.nodes ?? [])
    .filter((n) => n.id !== selectedNode.value?.id)
    .map((n) => ({ label: `${n.id} (${n.type})`, value: n.id })),
);

const fieldTypeOptions = computed(() =>
  STATE_FIELD_TYPE_OPTIONS.map((opt) => ({ label: t(opt.labelKey), value: opt.value })),
);
const reducerOptions = computed(() => REDUCER_OPTIONS.map((opt) => ({ label: t(opt.labelKey), value: opt.value })));
const engineOptions = computed(() => ENGINE_OPTIONS.map((opt) => ({ label: t(opt.labelKey), value: opt.value })));
const failureActionOptions = computed(() =>
  FAILURE_ACTION_OPTIONS.map((opt) => ({ label: t(opt.labelKey), value: opt.value })),
);
const assignmentModeOptions = computed(() => [
  { label: t('graphs.assignmentModeAuto'), value: 'auto' },
  { label: t('graphs.assignmentModeManual'), value: 'manual' },
  { label: t('graphs.assignmentModeRoundRobin'), value: 'round_robin' },
  { label: t('graphs.assignmentModeLeastBusy'), value: 'least_busy' },
]);

const {
  routerConditionalEdges,
  updateCondFuncRef,
  updatePathMapLabel,
  updatePathMapTarget,
  removePathMapEntry,
  addPathMapEntry,
  addConditionalEdge,
} = useConditionalRoutes(
  computed(() => graphDef.value),
  computed(() => selectedNode.value?.id ?? null),
  computed(() => props.undoRedo),
  notifyChange,
  destinationOptions,
);

function addStateField() {
  if (graphDef.value) {
    const field = {
      name: '',
      type: 'string' as const,
      reducer: 'cover' as ReducerType,
      required: false,
      disableDeepCopy: false,
    };
    const idx = graphDef.value.stateFields.length;
    graphDef.value.stateFields.push(field);
    if (props.undoRedo) {
      props.undoRedo.pushAddStateField(field, idx);
    } else {
      notifyChange();
    }
  }
}

function removeStateField(index: number) {
  if (graphDef.value) {
    const field = graphDef.value.stateFields[index];
    if (field) {
      graphDef.value.stateFields.splice(index, 1);
      if (props.undoRedo) {
        props.undoRedo.pushRemoveStateField(field, index);
      } else {
        notifyChange();
      }
    }
  }
}
</script>
