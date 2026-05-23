import { computed, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import type { GraphDefinition, NodeDef, ValidationError, ValidationWarning } from "./types";
import { useGraphStore } from "../../stores/graph";
import { useToolsStore } from "../../stores/tools";
import { useGraphEditorAssets } from "./useGraphEditorAssets";
import { useGraphExecute } from "./useGraphExecute";

export function useGraphEditorPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();
  const toolsStore = useToolsStore();
  const graphExecute = useGraphExecute(router);

  const isDark = computed(() => $q.dark.isActive);
  const isNew = computed(() => route.name === "graph-editor-new");
  const graphId = computed(() => (route.params.id as string) ?? "");

  const saving = ref(false);
  const dirty = ref(false);
  const selectedNodeId = ref<string | null>(null);
  const availableTools = ref<string[]>([]);
  const validationErrors = ref<ValidationError[]>([]);
  const validationWarnings = ref<ValidationWarning[]>([]);
  const validationValid = ref(true);

  const graphDef = reactive<GraphDefinition>({
    id: "",
    name: "",
    description: "",
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: "",
    finishPoint: "",
    enableCheckpoint: true,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    createdAt: "",
    updatedAt: "",
  });

  const assets = useGraphEditorAssets(graphDef, () => isNew.value);

  const selectedNode = computed<NodeDef | null>(() => {
    if (!selectedNodeId.value) return null;
    return graphDef.nodes.find((n) => n.id === selectedNodeId.value) ?? null;
  });

  const canSave = computed(() => Boolean(graphDef.name && graphDef.nodes.length > 0));

  async function loadToolOptions() {
    try {
      const result = await toolsStore.loadTools({ page_size: 200 });
      availableTools.value = (result.items ?? []).map((tool) => tool.name).filter(Boolean);
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
      if (graphDef.id) {
        await runValidation(graphDef.id);
      }
    } catch {
      $q.notify({ type: "negative", message: "加载 Graph 失败" });
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
        router.replace({ name: "graph-editor", params: { id: persisted.id } });
      } else {
        persisted = await graphStore.editGraph(graphDef.id, graphDef);
        Object.assign(graphDef, persisted);
        dirty.value = false;
      }

      const validation = await runValidation(persisted.id);
      if (!validation.valid) {
        $q.notify({ type: "warning", message: "Graph 已保存，但校验未通过，请查看右侧面板" });
        return;
      }
      $q.notify({ type: "positive", message: "Graph 已保存" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
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
      $q.notify({ type: "positive", message: "已从模板创建 Graph" });
      router.replace({ name: "graph-editor", params: { id: created.id } });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "模板创建失败" });
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

  function goBack() {
    router.push({ name: "graphs" });
  }

  async function openVersionDialog() {
    await assets.openVersionDialog();
  }

  async function rollbackVersion(version: number) {
    await assets.rollbackVersion(version, async () => {
      dirty.value = false;
      if (graphDef.id) {
        await runValidation(graphDef.id);
      }
    });
  }

  async function onImportFile(event: Event) {
    await assets.onImportFile(event);
    dirty.value = false;
  }

  onBeforeRouteLeave((_to, _from, next) => {
    if (dirty.value) {
      $q
        .dialog({
          title: "未保存的更改",
          message: "当前 Graph 有未保存修改，确定离开吗？",
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
  };
}
