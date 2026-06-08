import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useGraphStore } from '../../stores/graph';
import type { GraphDefinition, GraphTemplateInfo, NodeType } from './types';
import { NODE_TYPE_STYLES } from './types';
import { relativeTime } from './utils';
import { useGraphExecute } from './useGraphExecute';
import type { ContextMenuItem } from '../../components/graph/GraphContextMenu.vue';

const SORT_OPTIONS = [
  { label: '更新时间', value: 'updatedAt' },
  { label: '名称', value: 'name' },
  { label: '节点数', value: 'nodes' },
];

const ENGINE_FILTER_OPTIONS = [
  { label: '全部引擎', value: '' },
  { label: 'BSP（默认）', value: 'bsp' },
  { label: 'DAG（并行）', value: 'dag' },
];

const NODE_TYPE_EMOJI: Record<NodeType, string> = {
  agent: '🤖',
  llm: '🧠',
  router: '🔀',
  function: '⚙️',
  tool: '🔧',
  join: '🔗',
  hitl: '✋',
};

export function useGraphsPage() {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const graphExecute = useGraphExecute(router);
  const { graphs: rows, loading } = storeToRefs(graphStore);

  const isDark = computed(() => $q.dark.isActive);
  const error = ref('');
  const runDialogGraph = ref<GraphDefinition | null>(null);

  const selectedGraphId = ref<string | null>(null);
  const ctxMenuVisible = ref(false);
  const ctxMenuX = ref(0);
  const ctxMenuY = ref(0);
  const ctxMenuGraph = ref<GraphDefinition | null>(null);

  const selectedGraph = computed(() => {
    if (!selectedGraphId.value) return null;
    return rows.value.find((g) => g.id === selectedGraphId.value) ?? null;
  });

  const ctxMenuItems = computed<ContextMenuItem[]>(() => [
    { icon: '✏️', label: '编辑', shortcut: 'Enter', action: 'edit' },
    { icon: '▶️', label: '执行', action: 'run', success: true },
    { icon: '📋', label: '复制', shortcut: 'Ctrl+D', action: 'duplicate' },
    { icon: '🗑️', label: '删除', shortcut: 'Del', action: 'delete', danger: true },
  ]);

  const searchQuery = ref('');
  const engineFilter = ref('');
  const sortKey = ref('updatedAt');
  const sortOrder = ref('desc');

  const filteredRows = computed(() => {
    let list = rows.value.slice();
    const q = searchQuery.value.trim().toLowerCase();
    if (q) {
      list = list.filter((g) => g.name.toLowerCase().includes(q) || (g.description ?? '').toLowerCase().includes(q));
    }
    if (engineFilter.value) {
      list = list.filter((g) => g.executionEngine === engineFilter.value);
    }
    if (sortKey.value === 'updatedAt' && sortOrder.value === 'desc') {
      list.sort(
        (a, b) => a.sortOrder - b.sortOrder || new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
      );
    } else {
      const dir = sortOrder.value === 'asc' ? 1 : -1;
      list.sort((a, b) => {
        switch (sortKey.value) {
          case 'name':
            return dir * a.name.localeCompare(b.name);
          case 'nodes':
            return dir * ((a.nodes?.length ?? 0) - (b.nodes?.length ?? 0));
          case 'updatedAt':
          default:
            return dir * (new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime());
        }
      });
    }
    return list;
  });

  function countNodesByType(graph: GraphDefinition) {
    const counts: Partial<Record<NodeType, number>> = {};
    for (const node of graph.nodes ?? []) {
      counts[node.type] = (counts[node.type] ?? 0) + 1;
    }
    return counts;
  }

  function nodeTypeBorderColor(type: string): string {
    return (NODE_TYPE_STYLES as Record<string, { borderColor: string }>)[type]?.borderColor ?? 'var(--color-accent)';
  }

  onMounted(() => void loadRows());

  async function loadRows() {
    error.value = '';
    try {
      await graphStore.loadGraphs();
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Graph 列表失败';
    }
  }

  function openCreate() {
    router.push({ name: 'graph-editor-new' });
  }

  function openEditor(id: string) {
    router.push({ name: 'graph-editor', params: { id } });
  }

  function openRunDialog(graph: GraphDefinition) {
    runDialogGraph.value = graph;
    graphExecute.openRunDialog(graph.id);
  }

  async function executeRun() {
    if (!runDialogGraph.value) return;
    await graphExecute.executeRun(runDialogGraph.value.id);
  }

  async function duplicateGraph(graph: GraphDefinition) {
    try {
      await graphStore.addGraph({
        name: `${graph.name} (副本)`,
        description: graph.description,
        stateFields: graph.stateFields,
        nodes: graph.nodes,
        edges: graph.edges,
        conditionalEdges: graph.conditionalEdges,
        subgraphs: graph.subgraphs,
        entryPoint: graph.entryPoint,
        finishPoint: graph.finishPoint,
        enableCheckpoint: graph.enableCheckpoint,
        executionEngine: graph.executionEngine,
        interruptBefore: graph.interruptBefore,
        interruptAfter: graph.interruptAfter,
        metadata: graph.metadata,
      });
      $q.notify({ type: 'positive', message: 'Graph 已复制' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '复制失败' });
    }
  }

  function confirmRemoveGraph(graph: GraphDefinition) {
    $q.dialog({
      title: '删除 Graph',
      message: `确定删除「${graph.name}」？此操作不可撤销。`,
      cancel: true,
      persistent: true,
    }).onOk(() => void doRemoveGraph(graph));
  }

  async function doRemoveGraph(graph: GraphDefinition) {
    try {
      await graphStore.removeGraph(graph.id);
      $q.notify({ type: 'info', message: 'Graph 已删除' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    }
  }

  async function reorderGraphs(ids: string[]) {
    try {
      await graphStore.reorderGraphList(ids);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '排序保存失败' });
    }
  }

  function selectGraph(id: string) {
    selectedGraphId.value = selectedGraphId.value === id ? null : id;
  }

  function onCardContextMenu(e: MouseEvent, graph: GraphDefinition) {
    e.preventDefault();
    selectedGraphId.value = graph.id;
    ctxMenuGraph.value = graph;
    ctxMenuX.value = e.clientX;
    ctxMenuY.value = e.clientY;
    ctxMenuVisible.value = true;
  }

  function closeCtxMenu() {
    ctxMenuVisible.value = false;
  }

  function onCtxMenuAction(action: string) {
    const graph = ctxMenuGraph.value;
    closeCtxMenu();
    if (!graph) return;
    switch (action) {
      case 'edit':
        openEditor(graph.id);
        break;
      case 'run':
        openRunDialog(graph);
        break;
      case 'duplicate':
        duplicateGraph(graph);
        break;
      case 'delete':
        confirmRemoveGraph(graph);
        break;
    }
  }

  // --- Template dialog ---
  const templateDialogOpen = ref(false);
  const selectedTemplateId = ref('');
  const templateCreating = ref(false);
  const templatesLoading = ref(false);
  const templates = ref<GraphTemplateInfo[]>([]);

  async function loadTemplates() {
    templatesLoading.value = true;
    try {
      await graphStore.loadTemplates();
      templates.value = graphStore.templates;
    } catch {
      $q.notify({ type: 'negative', message: '加载模板失败' });
    } finally {
      templatesLoading.value = false;
    }
  }

  watch(templateDialogOpen, async (open) => {
    if (open) {
      selectedTemplateId.value = '';
      await loadTemplates();
    }
  });

  async function createFromTemplate() {
    if (!selectedTemplateId.value) return;
    const tpl = templates.value.find((t) => t.id === selectedTemplateId.value);
    if (!tpl) return;
    templateCreating.value = true;
    try {
      const created = await graphStore.instantiateTemplate(selectedTemplateId.value, tpl.name, tpl.description ?? '');
      templateDialogOpen.value = false;
      $q.notify({ type: 'positive', message: 'Graph 已从模板创建' });
      router.push({ name: 'graph-editor', params: { id: created.id } });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '从模板创建失败' });
    } finally {
      templateCreating.value = false;
    }
  }

  async function quickCreateFromTemplate(tpl: GraphTemplateInfo) {
    templateCreating.value = true;
    try {
      const created = await graphStore.instantiateTemplate(tpl.id, tpl.name, tpl.description ?? '');
      $q.notify({ type: 'positive', message: `已从「${tpl.name}」模板创建 Graph` });
      router.push({ name: 'graph-editor', params: { id: created.id } });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '从模板创建失败' });
    } finally {
      templateCreating.value = false;
    }
  }

  return {
    isDark,
    rows,
    filteredRows,
    loading,
    error,
    searchQuery,
    engineFilter,
    sortKey,
    sortOrder,
    SORT_OPTIONS,
    ENGINE_FILTER_OPTIONS,
    NODE_TYPE_EMOJI,
    nodeTypeBorderColor,
    countNodesByType,
    relativeTime,
    runDialogOpen: graphExecute.runDialogOpen,
    runDialogGraph,
    runSessionId: graphExecute.runSessionId,
    runInitialState: graphExecute.runInitialState,
    runLoading: graphExecute.runLoading,
    selectedGraphId,
    selectedGraph,
    ctxMenuVisible,
    ctxMenuX,
    ctxMenuY,
    ctxMenuItems,
    loadRows,
    openCreate,
    openEditor,
    openRunDialog,
    executeRun,
    duplicateGraph,
    confirmRemoveGraph,
    reorderGraphs,
    selectGraph,
    onCardContextMenu,
    closeCtxMenu,
    onCtxMenuAction,
    templateDialogOpen,
    selectedTemplateId,
    templateCreating,
    templatesLoading,
    templates,
    createFromTemplate,
    quickCreateFromTemplate,
    loadTemplates,
  };
}
