<template>
  <q-page :class="['graph-workbench graph-editor-page', { 'is-dark': isDark }]">
    <div class="graph-editor-page__toolbar graph-workbench__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-workbench__toolbar-meta">
        <div class="graph-editor-page__title">{{ isNew ? '新增 Graph' : graphDef.name || '编辑 Graph' }}</div>
        <div class="graph-workbench__subtitle">{{ isNew ? '拖拽组件到画布开始编排' : `v${graphDef.version || 0} · ${graphDef.nodes.length} 节点` }}</div>
      </div>
      <q-space />
      <q-chip v-if="dirty" dense square class="graph-workbench__dirty-chip">未保存</q-chip>
      <q-chip v-if="!validationValid" dense square color="negative" text-color="white">校验未通过</q-chip>
      <q-btn v-if="!isNew" flat dense icon="more_vert">
        <q-menu anchor="bottom right" self="top right">
          <q-list dense style="min-width: 180px">
            <q-item v-close-popup clickable @click="exportCurrentGraph">
              <q-item-section avatar><q-icon name="download" /></q-item-section>
              <q-item-section>导出 JSON</q-item-section>
            </q-item>
            <q-item v-close-popup clickable @click="triggerImport">
              <q-item-section avatar><q-icon name="upload" /></q-item-section>
              <q-item-section>导入 JSON</q-item-section>
            </q-item>
            <q-item v-if="graphDef.id" v-close-popup clickable @click="openVersionDialog">
              <q-item-section avatar><q-icon name="history" /></q-item-section>
              <q-item-section>版本历史</q-item-section>
            </q-item>
            <q-item v-if="graphDef.id" v-close-popup clickable @click="openTemplateDialog">
              <q-item-section avatar><q-icon name="bookmark_add" /></q-item-section>
              <q-item-section>保存为模板</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </q-btn>
      <q-btn flat dense round icon="save" color="primary" :loading="saving" :disable="!canSave" @click="save">
        <q-tooltip>保存</q-tooltip>
      </q-btn>
      <q-btn v-if="!isNew" flat dense round icon="play_arrow" color="positive" @click="openRunDialog">
        <q-tooltip>执行</q-tooltip>
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
        :graph-def="graphDef"
        :is-dark="isDark"
        @select-node="onSelectNode"
        @update-graph="markDirty"
      />
      <GraphPropertyPanel
        :selected-node="selectedNode"
        :graph-def="graphDef"
        :available-tools="availableTools"
        :is-dark="isDark"
        :validation-errors="validationErrors"
        :validation-warnings="validationWarnings"
        :validation-valid="validationValid"
        @deselect="onSelectNode(null)"
        @select-node="onSelectNode"
        @change="markDirty"
      />
    </div>

    <q-dialog v-model="runDialogOpen" persistent>
      <q-card class="graph-run-dialog app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">执行 Graph</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input v-model="runSessionId" class="app-field-md" dense outlined label="Session ID" />
          <q-input v-model="runInitialState" class="app-field-long" dense outlined autogrow type="textarea" label="初始状态 (JSON)" />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat rounded label="取消" @click="runDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="执行" :loading="runLoading" @click="executeRun" />
        </q-card-actions>
      </q-card>
    </q-dialog>

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
import GraphNodePalette from "../components/graph/GraphNodePalette.vue";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphPropertyPanel from "../components/graph/GraphPropertyPanel.vue";
import GraphVersionPanel from "../components/graph/GraphVersionPanel.vue";
import { useGraphEditorPage } from "../features/graph/useGraphEditorPage";

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
  validationErrors,
  validationWarnings,
  validationValid,
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
  onSelectNode,
  markDirty,
  save,
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
} = useGraphEditorPage();
</script>

<style scoped>
.graph-workbench__dirty-chip {
  background: color-mix(in srgb, var(--color-warning) 16%, var(--glass-surface));
  color: var(--color-text-secondary);
}
</style>
