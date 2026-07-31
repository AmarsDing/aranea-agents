<template>
  <q-page :class="['graph-workbench graph-editor-page', { 'is-dark': isDark }]">
    <div class="graph-editor-page__toolbar graph-workbench__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-workbench__toolbar-meta">
        <div class="graph-editor-page__title">{{ isNew ? '新增 Graph' : graphDef.name || '编辑 Graph' }}</div>
        <div class="graph-workbench__subtitle">
          {{ isNew ? '拖拽组件到画布开始编排' : `v${graphDef.version || 0} · ${graphDef.nodes.length} 节点` }}
        </div>
      </div>
      <q-space />
      <q-btn flat dense round icon="undo" :disable="!canUndo" @click="undo">
        <q-tooltip>撤销 (Ctrl+Z)</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="redo" :disable="!canRedo" @click="redo">
        <q-tooltip>重做 (Ctrl+Shift+Z)</q-tooltip>
      </q-btn>
      <q-chip v-if="dirty" dense square class="graph-workbench__dirty-chip">未保存</q-chip>
      <q-btn
        v-if="validationIssues.length > 0"
        flat
        dense
        no-caps
        :class="[
          'graph-editor-page__validation-btn',
          validationErrorCount > 0
            ? 'graph-editor-page__validation-btn--error'
            : 'graph-editor-page__validation-btn--warning',
          { 'graph-editor-page__validation-btn--active': validationPanelOpen },
        ]"
        @click="toggleValidationPanel"
      >
        <q-icon :name="validationErrorCount > 0 ? 'error' : 'warning'" size="14px" class="q-mr-xs" />
        <span v-if="validationErrorCount > 0">{{
          t('graphs.validationFailedCount', { n: validationErrorCount })
        }}</span>
        <span v-else>{{ t('graphs.validationWarningsCount', { n: validationWarningCount }) }}</span>
        <q-tooltip>{{ t('graphs.validationPanelTitle') }}</q-tooltip>
      </q-btn>
      <q-chip v-else dense square class="graph-editor-page__validation-pass">
        <q-icon name="check_circle" size="13px" class="q-mr-xs" />{{ t('graphs.validationPassed') }}
      </q-chip>
      <template v-if="!isNew">
        <q-btn flat dense round icon="download" @click="exportCurrentGraph">
          <q-tooltip>导出 JSON</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="upload" @click="triggerImport">
          <q-tooltip>导入 JSON</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="account_tree" @click="autoLayout">
          <q-tooltip>自动布局</q-tooltip>
        </q-btn>
        <q-btn v-if="graphDef.id" flat dense round icon="restore" @click="openVersionDialog">
          <q-tooltip>版本历史</q-tooltip>
        </q-btn>
        <q-btn v-if="graphDef.id" flat dense round icon="bookmark_add" @click="openTemplateDialog">
          <q-tooltip>保存为模板</q-tooltip>
        </q-btn>
      </template>
      <q-btn
        flat
        dense
        round
        icon="save"
        color="primary"
        :loading="saving"
        :disable="!canSave"
        data-test="graph-save"
        @click="onSaveClick"
      >
        <q-tooltip>{{ isTeamOwnedGraph ? '保存（Team 拓扑，需确认）' : '保存' }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="!isNew"
        flat
        dense
        round
        icon="play_arrow"
        color="positive"
        :disable="!mergedValidationValid"
        @click="openRunDialog"
      >
        <q-tooltip>{{ mergedValidationValid ? '执行' : t('graphs.validationRunBlocked') }}</q-tooltip>
      </q-btn>
      <q-btn v-if="!isNew" flat dense round icon="history" @click="goToExecutions">
        <q-tooltip>执行历史</q-tooltip>
      </q-btn>
    </div>

    <div class="graph-workbench__body">
      <GraphNodePalette
        :is-dark="isDark"
        :templates="templates"
        :templates-loading="templatesLoading"
        @request-templates="requestTemplates"
        @create-from-template="createFromTemplate"
      />
      <GraphEditorCanvas
        v-model:graph-def="graphDef"
        :is-dark="isDark"
        :undo-redo="undoRedo"
        :node-issues="nodeIssueMap"
        :spotlight-node-id="spotlightNodeId"
        @select-node="onSelectNode"
        @update-graph="markDirty"
        @request-auto-layout="autoLayout"
        @focus-property-panel="handleFocusPropertyPanel"
        @clear-spotlight="clearSpotlight"
      />
      <GraphPropertyPanel
        ref="propertyPanelRef"
        v-model:selected-node="selectedNode"
        v-model:graph-def="graphDef"
        :available-tools="availableTools"
        :is-dark="isDark"
        :undo-redo="undoRedo"
        :all-nodes="graphDef.nodes"
        :state-fields="graphDef.stateFields"
        @deselect="onSelectNode(null)"
        @select-node="onSelectNode"
        @change="markDirty"
      />
    </div>

    <GraphValidationPanel
      :open="validationPanelOpen"
      :issues="validationIssues"
      :validating="revalidating"
      @locate="onLocateValidationNode"
      @close="closeValidationPanel"
      @revalidate="revalidateGraph"
    />

    <GraphRunDialog
      v-model="runDialogOpen"
      v-model:session-id="runSessionId"
      v-model:initial-state="runInitialState"
      :graph-name="graphDef.name"
      :loading="runLoading"
      @submit="executeRun"
    />

    <input ref="importInputRef" type="file" accept="application/json,.json" hidden @change="onImportFile" />

    <GraphVersionPanel
      v-model="versionDialogOpen"
      :versions="versions"
      :loading="versionsLoading"
      :rolling-back-version="rollingBackVersion"
      @rollback="rollbackVersion"
    />

    <q-dialog v-model="templateDialogOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">保存为用户模板</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input v-model="templateName" dense outlined label="模板名称" />
          <q-input v-model="templateCategory" dense outlined label="分类" />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat rounded label="取消" @click="templateDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="保存" :loading="templateSaving" @click="saveTemplate" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import GraphNodePalette from '../components/graph/GraphNodePalette.vue';
import GraphEditorCanvas from '../components/graph/GraphEditorCanvas.vue';
import GraphPropertyPanel from '../components/graph/GraphPropertyPanel.vue';
import GraphValidationPanel from '../components/graph/GraphValidationPanel.vue';
import GraphVersionPanel from '../components/graph/GraphVersionPanel.vue';
import GraphRunDialog from '../components/graph/GraphRunDialog.vue';
import { useGraphEditorPage } from '../features/graph/useGraphEditorPage';

const { t } = useI18n();

const propertyPanelRef = ref<InstanceType<typeof GraphPropertyPanel> | null>(null);

const {
  isDark,
  isNew,
  saving,
  dirty,
  runDialogOpen,
  runSessionId,
  runInitialState,
  runLoading,
  availableTools,
  mergedValidationValid,
  validationIssues,
  validationDock,
  versionDialogOpen,
  versions,
  versionsLoading,
  rollingBackVersion,
  templateDialogOpen,
  templateName,
  templateCategory,
  templateSaving,
  importInputRef,
  templates,
  templatesLoading,
  graphDef,
  selectedNode,
  canSave,
  isTeamOwnedGraph,
  onSelectNode,
  onFocusPropertyPanel,
  markDirty,
  onSaveClick,
  canUndo,
  canRedo,
  undo,
  redo,
  undoRedo,
  requestTemplates,
  createFromTemplate,
  openRunDialog,
  executeRun,
  openVersionDialog,
  rollbackVersion,
  exportCurrentGraph,
  triggerImport,
  onImportFile,
  openTemplateDialog,
  saveTemplate,
  goBack,
  autoLayout,
  goToExecutions,
} = useGraphEditorPage();

// R2-7：校验 dock 状态（底部面板 + 节点聚光灯）
const {
  panelOpen: validationPanelOpen,
  spotlightNodeId,
  validating: revalidating,
  errorCount: validationErrorCount,
  warningCount: validationWarningCount,
  nodeIssueMap,
  togglePanel: toggleValidationPanel,
  closePanel: closeValidationPanel,
  locateNode: locateValidationNode,
  clearSpotlight,
  revalidate: revalidateGraph,
} = validationDock;

function onLocateValidationNode(nodeId: string) {
  onSelectNode(nodeId);
  locateValidationNode(nodeId);
}

// R2-6：列表页详情面板「定位」跳转 —— 图加载完成后选中并聚光灯目标节点（一次性）
const route = useRoute();
let spotlightConsumed = false;
watch(
  () => graphDef.nodes?.length ?? 0,
  (len) => {
    if (spotlightConsumed || len === 0) return;
    const target = route.query.spotlight;
    if (typeof target === 'string' && target) {
      spotlightConsumed = true;
      onSelectNode(target);
      locateValidationNode(target);
    }
  },
);

function handleFocusPropertyPanel(nodeId: string) {
  onFocusPropertyPanel(nodeId, propertyPanelRef.value?.$el ?? null);
}
</script>
