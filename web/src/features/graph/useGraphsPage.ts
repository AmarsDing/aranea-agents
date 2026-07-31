import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useGraphStore } from '../../stores/graph';
import { useTeamsStore } from '../../stores/teams';
import type { GraphDefinition, GraphExecutionSummary, GraphTemplateInfo } from './types';
import { NODE_TYPE_STYLES } from './types';
import { buildCompositionChips, deriveGraphStatus, relativeTime } from './utils';
import { listGraphExecutions } from './api';
import { useGraphExecute } from './useGraphExecute';
import type { ContextMenuItem } from '../../components/graph/GraphContextMenu.vue';

export function useGraphsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const teamsStore = useTeamsStore();
  const graphExecute = useGraphExecute(router);
  const { graphs: rows, loading, graphsNextPageToken } = storeToRefs(graphStore);

  const isDark = computed(() => $q.dark.isActive);
  const error = ref('');
  const runDialogGraph = ref<GraphDefinition | null>(null);
  const hasNextPage = computed(() => Boolean(graphsNextPageToken.value));
  const loadingMore = ref(false);

  const selectedGraphId = ref<string | null>(null);
  const ctxMenuVisible = ref(false);
  const ctxMenuX = ref(0);
  const ctxMenuY = ref(0);
  const ctxMenuGraph = ref<GraphDefinition | null>(null);

  const SORT_OPTIONS = computed(() => [
    { label: t('graphs.sortUpdatedAt'), value: 'updatedAt' },
    { label: t('graphs.sortName'), value: 'name' },
    { label: t('graphs.sortNodes'), value: 'nodes' },
  ]);

  const ENGINE_FILTER_OPTIONS = computed(() => [
    { label: t('graphs.engineFilterAll'), value: '' },
    { label: t('graphs.engineBSP'), value: 'bsp' },
    { label: t('graphs.engineDAG'), value: 'dag' },
  ]);

  // ── M53 Phase 11 F5：Team 关联过滤（''=全部 / independent=独立 / team=Team 关联） ──
  const TEAM_FILTER_OPTIONS = computed(() => [
    { label: t('graphs.teamFilterAll'), value: '' },
    { label: t('graphs.teamFilterIndependent'), value: 'independent' },
    { label: t('graphs.teamFilterLinked'), value: 'team' },
  ]);

  const selectedGraph = computed(() => {
    if (!selectedGraphId.value) return null;
    return rows.value.find((g) => g.id === selectedGraphId.value) ?? null;
  });

  /**
   * team-owned 判定镜像后端 metadata.team_owned（team_id 对 linked_external 也会
   * 回填，不能作为 owned 判定，见后端 isTeamOwnedGraph 注释）。
   */
  function isTeamOwned(graph: GraphDefinition): boolean {
    return graph.metadata?.team_owned === true;
  }

  /** 属主/关联 Team 展示名：teams 列表映射，未加载或找不到时回退 teamId。 */
  function teamDisplayName(teamId: string): string {
    const id = String(teamId || '').trim();
    if (!id) return '';
    return teamsStore.teams.find((tm) => tm.id === id)?.display_name || id;
  }

  const ctxMenuItems = computed<ContextMenuItem[]>(() => {
    const items: ContextMenuItem[] = [
      { icon: '✏️', label: t('graphs.ctxEdit'), shortcut: 'Enter', action: 'edit' },
      { icon: '▶️', label: t('graphs.ctxRun'), action: 'run', success: true },
      { icon: '📋', label: t('graphs.ctxDuplicate'), shortcut: 'Ctrl+D', action: 'duplicate' },
    ];
    // F5：Team 关联图（owned 或 linked_external）提供编排页跳转
    if (ctxMenuGraph.value?.teamId) {
      items.push({ icon: '🧭', label: t('graphs.ctxOpenTeam'), action: 'open-team' });
    }
    items.push({ icon: '🗑️', label: t('graphs.ctxDelete'), shortcut: 'Del', action: 'delete', danger: true });
    return items;
  });

  const searchQuery = ref('');
  const engineFilter = ref('');
  const teamFilter = ref('');
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
    if (teamFilter.value === 'team') {
      list = list.filter((g) => Boolean(g.teamId));
    } else if (teamFilter.value === 'independent') {
      list = list.filter((g) => !g.teamId);
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

  // --- R2-A 卡片数据派生（状态映射 + 构成 chips；执行次数列表页无数据，避免 N+1） ---
  function cardStatus(graph: GraphDefinition) {
    return deriveGraphStatus(graph);
  }

  function compositionChips(graph: GraphDefinition) {
    return buildCompositionChips(countNodesByType(graph));
  }

  onMounted(() => {
    void loadRows();
    // F5：属主 Team 名映射（best-effort，失败回退 teamId 不阻断列表）
    void teamsStore.loadTeams().catch(() => {});
  });

  async function loadRows() {
    error.value = '';
    try {
      await graphStore.loadGraphs();
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('graphs.loadFailed');
    }
  }

  async function loadMore() {
    if (!graphsNextPageToken.value || loadingMore.value) return;
    error.value = '';
    loadingMore.value = true;
    try {
      await graphStore.loadGraphs(undefined, true);
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载更多 Graph 失败';
    } finally {
      loadingMore.value = false;
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
        name: t('graphs.copySuffix', { name: graph.name }),
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
      $q.notify({ type: 'positive', message: t('graphs.duplicateSuccess') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.duplicateFailed') });
    }
  }

  function confirmRemoveGraph(graph: GraphDefinition) {
    $q.dialog({
      title: t('graphs.deleteTitle'),
      message: t('graphs.deleteConfirm', { name: graph.name }),
      cancel: true,
      persistent: true,
    }).onOk(() => void doRemoveGraph(graph));
  }

  async function doRemoveGraph(graph: GraphDefinition) {
    try {
      await graphStore.removeGraph(graph.id);
      $q.notify({ type: 'info', message: t('graphs.deleteSuccess') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.deleteFailed') });
    }
  }

  async function reorderGraphs(ids: string[]) {
    try {
      await graphStore.reorderGraphList(ids);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.reorderFailed') });
    }
  }

  function selectGraph(id: string) {
    selectedGraphId.value = selectedGraphId.value === id ? null : id;
  }

  // --- R2-6 详情面板：执行历史预览（最近 5 条 + hasMore） ---
  const selectedExecutions = ref<GraphExecutionSummary[]>([]);
  const selectedExecutionsHasMore = ref(false);

  watch(selectedGraphId, async (id) => {
    selectedExecutions.value = [];
    selectedExecutionsHasMore.value = false;
    if (!id) return;
    try {
      const result = await listGraphExecutions(id, 6);
      selectedExecutions.value = result.items.slice(0, 5);
      selectedExecutionsHasMore.value = result.items.length > 5 || Boolean(result.nextPageToken);
    } catch {
      // 执行历史预览失败不阻断详情面板展示
    }
  });

  async function exportGraphJson(graph: GraphDefinition) {
    try {
      const result = await graphStore.exportGraphDefinition(graph.id);
      const blob = new Blob([result.json], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `${graph.name || graph.id}.graph.json`;
      anchor.click();
      URL.revokeObjectURL(url);
      $q.notify({ type: 'positive', message: t('graphs.assetExported') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetExportFailed') });
    }
  }

  // R2-6 行内定位：跳转编辑器并聚光灯目标节点
  function locateNodeInEditor(nodeId: string) {
    if (!selectedGraphId.value) return;
    router.push({ name: 'graph-editor', params: { id: selectedGraphId.value }, query: { spotlight: nodeId } });
  }

  // R2-8 入口：跳转编辑器并打开 State Schema 抽屉
  function manageSchemaInEditor() {
    if (!selectedGraphId.value) return;
    router.push({ name: 'graph-editor', params: { id: selectedGraphId.value }, query: { schema: '1' } });
  }

  function viewSelectedExecutions() {
    if (!selectedGraphId.value) return;
    router.push({ name: 'graph-executions', params: { id: selectedGraphId.value } });
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
      case 'open-team':
        // F5：跳转 Team 编排页（teamId 非空才出现该菜单项）
        if (graph.teamId) {
          router.push({ name: 'team-orchestrate', params: { teamId: graph.teamId } });
        }
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
      $q.notify({ type: 'negative', message: t('graphs.loadTemplatesFailed') });
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
    const tpl = templates.value.find((tt) => tt.id === selectedTemplateId.value);
    if (!tpl) return;
    templateCreating.value = true;
    try {
      const created = await graphStore.instantiateTemplate(selectedTemplateId.value, tpl.name, tpl.description ?? '');
      templateDialogOpen.value = false;
      $q.notify({ type: 'positive', message: t('graphs.createFromTemplateSuccess') });
      router.push({ name: 'graph-editor', params: { id: created.id } });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('graphs.createFromTemplateFailed'),
      });
    } finally {
      templateCreating.value = false;
    }
  }

  async function quickCreateFromTemplate(tpl: GraphTemplateInfo) {
    templateCreating.value = true;
    try {
      const created = await graphStore.instantiateTemplate(tpl.id, tpl.name, tpl.description ?? '');
      $q.notify({ type: 'positive', message: t('graphs.quickCreateSuccess', { name: tpl.name }) });
      router.push({ name: 'graph-editor', params: { id: created.id } });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('graphs.createFromTemplateFailed'),
      });
    } finally {
      templateCreating.value = false;
    }
  }

  return {
    isDark,
    rows,
    filteredRows,
    loading,
    loadingMore,
    hasNextPage,
    error,
    searchQuery,
    engineFilter,
    teamFilter,
    sortKey,
    sortOrder,
    SORT_OPTIONS,
    ENGINE_FILTER_OPTIONS,
    TEAM_FILTER_OPTIONS,
    isTeamOwned,
    teamDisplayName,
    nodeTypeBorderColor,
    countNodesByType,
    cardStatus,
    compositionChips,
    relativeTime,
    runDialogOpen: graphExecute.runDialogOpen,
    runDialogGraph,
    runSessionId: graphExecute.runSessionId,
    runInitialState: graphExecute.runInitialState,
    runLoading: graphExecute.runLoading,
    selectedGraphId,
    selectedGraph,
    selectedExecutions,
    selectedExecutionsHasMore,
    exportGraphJson,
    locateNodeInEditor,
    manageSchemaInEditor,
    viewSelectedExecutions,
    ctxMenuVisible,
    ctxMenuX,
    ctxMenuY,
    ctxMenuItems,
    loadRows,
    loadMore,
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
