import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import type { GraphDefinition, NodeDef, ValidationError, ValidationIssue, ValidationWarning } from './types';
import { applyAutoLayout } from './editor/graphLayout';
import { useGraphStore } from '../../stores/graph';
import { useToolsStore } from '../../stores/tools';
import { useGraphEditorAssets } from './useGraphEditorAssets';
import { useGraphExecute } from './useGraphExecute';
import { useGraphUndoRedo } from './useGraphUndoRedo';
import { useGraphLocalValidation } from './useGraphLocalValidation';
import { buildValidationIssues } from './validationIssues';
import { useGraphValidationDock } from './useGraphValidationDock';

export function useGraphEditorPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();
  const toolsStore = useToolsStore();
  const graphExecute = useGraphExecute(router);

  const isDark = computed(() => $q.dark.isActive);
  const isNew = computed(() => route.name === 'graph-editor-new');
  const graphId = computed(() => (route.params.id as string) ?? '');

  const saving = ref(false);
  const dirty = ref(false);
  const selectedNodeId = ref<string | null>(null);
  const availableTools = ref<string[]>([]);
  const validationErrors = ref<ValidationError[]>([]);
  const validationWarnings = ref<ValidationWarning[]>([]);
  const validationValid = ref(true);

  const graphDef = reactive<GraphDefinition>({
    id: '',
    name: '',
    description: '',
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  });

  const assets = useGraphEditorAssets(graphDef, () => isNew.value);
  const undoRedo = useGraphUndoRedo(graphDef, markDirty);
  const localValidation = useGraphLocalValidation(computed(() => graphDef));

  const selectedNode = computed<NodeDef | null>(() => {
    if (!selectedNodeId.value) return null;
    return graphDef.nodes.find((n) => n.id === selectedNodeId.value) ?? null;
  });

  const canSave = computed(() => Boolean(graphDef.name && graphDef.nodes.length > 0 && dirty.value));

  const mergedValidationErrors = computed<ValidationError[]>(() => {
    const serverKeys = new Set(validationErrors.value.map((e) => `${e.code}:${e.nodeId}:${e.field}`));
    const filtered = localValidation.localErrors.value.filter(
      (e) => !serverKeys.has(`${e.code}:${e.nodeId}:${e.field}`),
    );
    return [...validationErrors.value, ...filtered];
  });

  const mergedValidationWarnings = computed<ValidationWarning[]>(() => {
    const serverKeys = new Set(validationWarnings.value.map((w) => `${w.code}:${w.nodeId}:${w.field}`));
    const filtered = localValidation.localWarnings.value.filter(
      (w) => !serverKeys.has(`${w.code}:${w.nodeId}:${w.field}`),
    );
    return [...validationWarnings.value, ...filtered];
  });

  const mergedValidationValid = computed(() => validationValid.value && localValidation.localValid.value);

  // R2-7：统一校验问题（本地+服务端合并、去重、排序）→ 校验 dock + 节点错误态 + 聚光灯
  const validationIssues = computed<ValidationIssue[]>(() =>
    buildValidationIssues(mergedValidationErrors.value, mergedValidationWarnings.value, graphDef.nodes),
  );

  async function revalidateGraph() {
    if (!graphDef.id) return;
    await runValidation(graphDef.id);
  }

  const validationDock = useGraphValidationDock(validationIssues, { onRevalidate: revalidateGraph });

  async function loadToolOptions() {
    try {
      const result = await toolsStore.loadTools({ page_size: 200 });
      availableTools.value = (result.items ?? []).map((tool) => tool.key).filter(Boolean);
    } catch {
      availableTools.value = [];
    }
  }

  onMounted(async () => {
    await loadToolOptions();
    if (!isNew.value && graphId.value) {
      await loadGraphDefinition(graphId.value);
    }
  });

  watch(graphId, async (id, prev) => {
    if (isNew.value || !id || id === prev) return;
    await loadGraphDefinition(id);
  });

  async function loadGraphDefinition(id: string) {
    try {
      const g = await graphStore.fetchGraph(id);
      Object.assign(graphDef, g);
      dirty.value = false;
      undoRedo.clear();
      if (graphDef.id) {
        await runValidation(graphDef.id);
      }
    } catch {
      $q.notify({ type: 'negative', message: '加载 Graph 失败' });
    }
  }

  async function runValidation(id: string) {
    const result = await graphStore.validateGraphDefinition(id);
    validationErrors.value = result.errors ?? [];
    validationWarnings.value = result.warnings ?? [];
    validationValid.value = result.valid;
    return result;
  }

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
  }

  function onFocusPropertyPanel(nodeId: string, panelEl?: HTMLElement | null) {
    selectedNodeId.value = nodeId;
    nextTick(() => {
      const panel = panelEl ?? document.querySelector('.graph-property-panel');
      if (panel) {
        const firstInput = panel.querySelector('input, textarea, select') as HTMLElement | null;
        if (firstInput) {
          firstInput.focus();
        }
      }
    });
  }

  function markDirty() {
    dirty.value = true;
  }

  async function save() {
    saving.value = true;
    try {
      let persisted: GraphDefinition;
      if (isNew.value || !graphDef.id) {
        persisted = await graphStore.addGraph(graphDef);
        Object.assign(graphDef, persisted);
        dirty.value = false;
        router.replace({ name: 'graph-editor', params: { id: persisted.id } });
      } else {
        persisted = await graphStore.editGraph(graphDef.id, graphDef);
        Object.assign(graphDef, persisted);
        dirty.value = false;
      }

      const validation = await runValidation(persisted.id);
      if (!validation.valid) {
        $q.notify({ type: 'warning', message: 'Graph 已保存，但校验未通过，请查看右侧面板' });
        return;
      }
      $q.notify({ type: 'positive', message: 'Graph 已保存' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      saving.value = false;
    }
  }

  async function requestTemplates() {
    await graphStore.loadTemplates();
  }

  async function createFromTemplate(templateId: string) {
    const name = graphDef.name.trim() || `graph-${Date.now()}`;
    try {
      const created = await graphStore.instantiateTemplate(templateId, name, graphDef.description);
      Object.assign(graphDef, created);
      if (created.id) {
        await runValidation(created.id);
      }
      dirty.value = false;
      undoRedo.clear();
      $q.notify({ type: 'positive', message: '已从模板创建 Graph' });
      router.replace({ name: 'graph-editor', params: { id: created.id } });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '模板创建失败' });
    }
  }

  function openRunDialog() {
    if (!graphDef.id) return;
    graphExecute.openRunDialog(graphDef.id);
  }

  async function executeRun() {
    if (!graphDef.id) return;
    await graphExecute.executeRun(graphDef.id);
  }

  function autoLayout() {
    if (graphDef.nodes.length === 0) return;
    const moves = applyAutoLayout(graphDef);
    if (moves.length > 0) {
      if (undoRedo) {
        undoRedo.pushMoveNodes(moves);
      }
      markDirty();
    }
  }

  function goBack() {
    router.push({ name: 'graphs' });
  }

  async function openVersionDialog() {
    await assets.openVersionDialog();
  }

  async function rollbackVersion(version: number) {
    await assets.rollbackVersion(version, async () => {
      dirty.value = false;
      undoRedo.clear();
      if (graphDef.id) {
        await runValidation(graphDef.id);
      }
    });
  }

  async function onImportFile(event: Event) {
    await assets.onImportFile(event);
    dirty.value = false;
    undoRedo.clear();
  }

  function isEditableTarget(el: EventTarget | null): boolean {
    if (!el || !(el instanceof HTMLElement)) return false;
    const tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (el.isContentEditable) return true;
    return false;
  }

  function onGlobalKeydown(e: KeyboardEvent) {
    if (isEditableTarget(e.target)) return;
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      if (canSave.value && !saving.value) {
        save();
      }
      return;
    }
    // 大小写不敏感：部分环境（IME/远程桌面/E2E）Ctrl+Shift+Z 派发小写 key='z'
    const key = e.key.toLowerCase();
    if ((e.ctrlKey || e.metaKey) && ((e.shiftKey && key === 'z') || key === 'y')) {
      e.preventDefault();
      undoRedo.redo();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && key === 'z') {
      e.preventDefault();
      undoRedo.undo();
      return;
    }
  }

  onMounted(() => {
    document.addEventListener('keydown', onGlobalKeydown);
  });

  onUnmounted(() => {
    document.removeEventListener('keydown', onGlobalKeydown);
  });

  onBeforeRouteLeave((_to, _from, next) => {
    if (dirty.value) {
      $q.dialog({
        title: '未保存的更改',
        message: '当前 Graph 有未保存修改，确定离开吗？',
        cancel: true,
        persistent: true,
      })
        .onOk(() => next())
        .onCancel(() => next(false));
    } else {
      next();
    }
  });

  return {
    isDark,
    isNew,
    saving,
    dirty,
    runDialogOpen: graphExecute.runDialogOpen,
    runSessionId: graphExecute.runSessionId,
    runInitialState: graphExecute.runInitialState,
    runLoading: graphExecute.runLoading,
    selectedNodeId,
    availableTools,
    validationErrors,
    validationWarnings,
    validationValid,
    mergedValidationErrors,
    mergedValidationWarnings,
    mergedValidationValid,
    validationIssues,
    validationDock,
    versionDialogOpen: assets.versionDialogOpen,
    versions: assets.versions,
    versionsLoading: assets.versionsLoading,
    rollingBackVersion: assets.rollingBackVersion,
    templateDialogOpen: assets.templateDialogOpen,
    templateName: assets.templateName,
    templateCategory: assets.templateCategory,
    templateSaving: assets.templateSaving,
    importInputRef: assets.importInputRef,
    templates: computed(() => graphStore.templates),
    templatesLoading: computed(() => graphStore.templatesLoading),
    graphDef,
    selectedNode,
    canSave,
    onSelectNode,
    onFocusPropertyPanel,
    markDirty,
    save,
    requestTemplates,
    createFromTemplate,
    openRunDialog,
    executeRun,
    openVersionDialog,
    rollbackVersion,
    exportCurrentGraph: assets.exportCurrentGraph,
    triggerImport: assets.triggerImport,
    onImportFile,
    openTemplateDialog: assets.openTemplateDialog,
    saveTemplate: assets.saveTemplate,
    goBack,
    autoLayout,
    goToExecutions: () => router.push({ name: 'graph-executions', params: { id: graphDef.id || route.params.id } }),
    canUndo: undoRedo.canUndo,
    canRedo: undoRedo.canRedo,
    undo: undoRedo.undo,
    redo: undoRedo.redo,
    undoRedo,
  };
}
