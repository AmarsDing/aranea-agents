import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import {
  buildToolSummaryCards,
  categoryFilterOptions,
  enabledTriStateOptions,
  riskLevelOptions,
} from '../../components/tools/toolUi';
import { useToolDetailStore } from '../../stores/tools/toolDetail';
import { useToolEditorStore } from '../../stores/tools/toolEditor';
import { useToolToggle } from './useToolToggle';
import { patchToolForm, toolToUpsertInput } from './toolFormPatch';
import type { Tool } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolsPage() {
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const detailStore = useToolDetailStore();
  const editorStore = useToolEditorStore();
  const { tools: rows, total, summary, loading } = storeToRefs(toolsStore);

  const search = ref('');
  const category = ref('');
  const riskLevel = ref('');
  const enabled = ref<boolean | null>(null);
  const page = ref(1);
  const pageSize = ref(20);
  const error = ref('');
  const selected = ref<Tool[]>([]);

  const categoryOptions = categoryFilterOptions;
  const riskOptions = riskLevelOptions;
  const enabledOptions = enabledTriStateOptions;

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const summaryCards = computed(() => buildToolSummaryCards(summary.value));

  async function loadRows() {
    error.value = '';
    try {
      await toolsStore.loadTools({
        search: search.value,
        category: category.value,
        risk_level: riskLevel.value,
        enabled: enabled.value,
        page: page.value,
        page_size: pageSize.value,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Tools 失败';
    }
  }

  const { busyId, toggleEnabled, removeTool } = useToolToggle(loadRows);

  editorStore.setCallbacks({
    onSaved: loadRows,
    onCreated: async (tool: Tool) => {
      const fetched = await toolsStore.fetchTool(tool.id || tool.key);
      detailStore.openDetail(fetched);
    },
  });

  function resetFilters() {
    search.value = '';
    category.value = '';
    riskLevel.value = '';
    enabled.value = null;
    page.value = 1;
    void loadRows();
  }

  async function openDetail(tool: Tool) {
    const fetched = await toolsStore.fetchTool(tool.id || tool.key);
    detailStore.openDetail(fetched);
  }

  async function updateRisk(tool: Tool, value: string) {
    if (value === 'critical' || value === 'high') {
      $q.dialog({
        title: '风险级别变更',
        message: `确定将「${tool.display_name || tool.key}」的风险级别设为「${value}」？这可能影响工具的调用策略。`,
        cancel: true,
        persistent: true,
      }).onOk(() => doUpdateRisk(tool, value));
      return;
    }
    await doUpdateRisk(tool, value);
  }

  async function doUpdateRisk(tool: Tool, value: string) {
    try {
      await toolsStore.editTool(tool.id || tool.key, toolToUpsertInput(tool, { risk_level: value }));
      $q.notify({ type: 'positive', message: '风险级别已更新' });
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新风险级别失败' });
    }
  }

  async function batchToggle(value: boolean) {
    for (const tool of selected.value) {
      try {
        await toolsStore.toggle(tool.id || tool.key, value);
      } catch {
        // continue
      }
    }
    selected.value = [];
    await loadRows();
  }

  function batchRemove() {
    const count = selected.value.length;
    $q.dialog({
      title: '批量删除',
      message: `确认删除选中的 ${count} 个 Tool？`,
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      for (const tool of selected.value) {
        try {
          await toolsStore.remove(tool.id || tool.key);
        } catch {
          // continue
        }
      }
      selected.value = [];
      await loadRows();
    });
  }

  function onEditTool(tool: Tool) {
    detailStore.closeDetail();
    editorStore.openEdit(tool);
  }

  function onPatchForm(p: Record<string, unknown>) {
    patchToolForm(editorStore.form, p);
  }

  watch(
    () => editorStore.open,
    (isOpen) => {
      if (!isOpen && editorStore.editingId && detailStore.tool) {
        nextTick(() => {
          detailStore.openDetail(detailStore.tool!);
        });
      }
    },
  );

  watch([search, category, riskLevel, enabled], () => {
    page.value = 1;
    void loadRows();
  });
  watch([page, pageSize], () => void loadRows());
  onMounted(loadRows);

  return {
    // stores
    detailStore,
    editorStore,
    // reactive data
    rows,
    total,
    summary,
    loading,
    search,
    category,
    riskLevel,
    enabled,
    page,
    pageSize,
    error,
    selected,
    // computed
    pageMax,
    summaryCards,
    categoryOptions,
    riskOptions,
    enabledOptions,
    // from useToolToggle
    busyId,
    // methods
    loadRows,
    toggleEnabled,
    removeTool,
    updateRisk,
    resetFilters,
    openDetail,
    batchToggle,
    batchRemove,
    onEditTool,
    onPatchForm,
  };
}
