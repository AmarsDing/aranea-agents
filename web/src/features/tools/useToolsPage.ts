import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  buildToolSummaryCards,
  categoryFilterOptions,
  enabledTriStateOptions,
  riskLevelOptions,
  sourceFilterOptions,
} from '../../components/tools/toolUi';
import { useToolDetailStore } from '../../stores/tools/toolDetail';
import { useToolEditorStore } from '../../stores/tools/toolEditor';
import { useToolToggle } from './useToolToggle';
import { patchToolForm, toolToUpsertInput } from './toolFormPatch';
import type { Tool, ToolAgentOverride } from './types';
import { useToolsStore } from '../../stores/tools';
import { parseKratosApiError } from '../../utils/kratosError';

export function useToolsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const toolsStore = useToolsStore();
  const detailStore = useToolDetailStore();
  const editorStore = useToolEditorStore();
  const { tools: rows, total, summary, loading } = storeToRefs(toolsStore);

  const search = ref('');
  const category = ref('');
  const source = ref('');
  const riskLevel = ref('');
  const enabled = ref<boolean | null>(null);
  const abnormal = ref(false);
  const page = ref(1);
  const pageSize = ref(20);
  const error = ref('');
  const selected = ref<Tool[]>([]);

  const categoryOptions = categoryFilterOptions;
  const sourceOptions = computed(() => sourceFilterOptions());
  const riskOptions = computed(() => riskLevelOptions());
  const enabledOptions = computed(() => enabledTriStateOptions());

  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const summaryCards = computed(() => buildToolSummaryCards(summary.value));

  async function loadRows() {
    error.value = '';
    try {
      await toolsStore.loadTools({
        search: search.value,
        category: category.value,
        source: source.value,
        risk_level: riskLevel.value,
        enabled: enabled.value,
        abnormal: abnormal.value,
        page: page.value,
        page_size: pageSize.value,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('toolsPage.list.loadFailed');
    }
  }

  const { busyId, toggleEnabled, removeTool } = useToolToggle(loadRows);

  function resetFilters() {
    // 合并 watch 后同一 tick 的多次赋值只触发一次 loadRows；
    // 筛选本就处于默认值时不产生 watch 触发，列表维持现状（刷新请用刷新按钮）。
    search.value = '';
    category.value = '';
    source.value = '';
    riskLevel.value = '';
    enabled.value = null;
    abnormal.value = false;
    page.value = 1;
  }

  function openDetail(tool: Tool) {
    // 立即用列表行数据打开抽屉（header/概览所需字段行内已全），
    // 全量详情后台静默补齐；失败时行数据亦足够展示。
    detailStore.openDetail(tool);
    void toolsStore
      .fetchTool(tool.id || tool.key)
      .then((fetched) => {
        const cur = detailStore.tool;
        if (cur && (cur.id === fetched.id || cur.key === fetched.key)) {
          detailStore.updateTool(fetched);
        }
      })
      .catch(() => {});
  }

  async function updateRisk(tool: Tool, value: string) {
    if (value === 'critical' || value === 'high') {
      $q.dialog({
        title: t('toolsPage.list.riskChangeTitle'),
        message: t('toolsPage.list.riskChangeMessage', {
          name: tool.display_name || tool.key,
          level: value,
        }),
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
      $q.notify({ type: 'positive', message: t('toolsPage.list.riskUpdated') });
      await loadRows();
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: parseKratosApiError(err).message || t('toolsPage.list.riskUpdateFailed'),
      });
    }
  }

  async function batchToggle(value: boolean) {
    const targets = selected.value.slice();
    if (targets.length === 0) return;

    if (value) {
      const highRisk = targets.filter((tool) => tool.risk_level === 'high' || tool.risk_level === 'critical');
      if (highRisk.length > 0) {
        const names = highRisk
          .slice(0, 5)
          .map((tool) => tool.display_name || tool.key)
          .join('、');
        const confirmed = await new Promise<boolean>((resolve) => {
          $q.dialog({
            title: t('toolsPage.list.batchHighRiskTitle'),
            message: t('toolsPage.list.batchHighRiskMessage', {
              names,
              count: highRisk.length,
              extra: highRisk.length > 5 ? t('toolsPage.list.batchHighRiskMore', { count: highRisk.length }) : '',
            }),
            cancel: true,
            persistent: true,
          })
            .onOk(() => resolve(true))
            .onCancel(() => resolve(false))
            .onDismiss(() => resolve(false));
        });
        if (!confirmed) return;
      }
    }

    let ok = 0;
    let failed = 0;
    for (const tool of targets) {
      try {
        const intent =
          value && (tool.risk_level === 'high' || tool.risk_level === 'critical') ? 'I_UNDERSTAND_RISK' : undefined;
        await toolsStore.toggle(tool.id || tool.key, value, intent);
        ok += 1;
      } catch {
        failed += 1;
      }
    }
    selected.value = [];
    await loadRows();
    if (failed > 0) {
      $q.notify({
        type: 'warning',
        message: t(value ? 'toolsPage.list.batchEnablePartial' : 'toolsPage.list.batchDisablePartial', { ok, failed }),
      });
    } else {
      $q.notify({
        type: 'positive',
        message: t(value ? 'toolsPage.list.batchEnableDone' : 'toolsPage.list.batchDisableDone', { count: ok }),
      });
    }
  }

  function batchRemove() {
    const readonlyCount = selected.value.filter((tool) => tool.readonly).length;
    const targets = selected.value.filter((tool) => !tool.readonly);
    const count = targets.length;
    if (count === 0) {
      $q.notify({ type: 'warning', message: t('toolsPage.list.batchRemoveAllReadonly') });
      return;
    }
    $q.dialog({
      title: t('toolsPage.list.batchRemoveTitle'),
      message:
        readonlyCount > 0
          ? t('toolsPage.list.batchRemoveMessageSkipReadonly', { count, readonly: readonlyCount })
          : t('toolsPage.list.batchRemoveMessage', { count }),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      let ok = 0;
      let failed = 0;
      for (const tool of targets) {
        try {
          await toolsStore.remove(tool.id || tool.key);
          ok += 1;
        } catch {
          failed += 1;
        }
      }
      selected.value = [];
      await loadRows();
      if (failed > 0) {
        $q.notify({ type: 'warning', message: t('toolsPage.list.batchRemovePartial', { ok, failed }) });
      } else {
        $q.notify({ type: 'positive', message: t('toolsPage.list.batchRemoveDone', { count: ok }) });
      }
    });
  }

  function onEditTool(tool: Tool) {
    detailStore.closeDetail();
    editorStore.openEdit(tool);
  }

  function onPatchForm(p: Record<string, unknown>) {
    patchToolForm(editorStore.form, p);
  }

  // ---- 编辑器 / 详情抽屉的 UI 反馈编排（store 保持纯数据流，红线 #4） ----

  /** 编辑器请求关闭：有未保存更改时在 Page 层弹确认，确认后丢弃。 */
  function onEditorRequestClose() {
    $q.dialog({
      title: t('toolsPage.editor.discardTitle'),
      message: t('toolsPage.editor.discardMessage'),
      cancel: { label: t('toolsPage.editor.discardCancel'), flat: true, noCaps: true },
      ok: { label: t('toolsPage.editor.discardOk'), noCaps: true, color: 'negative' },
      persistent: true,
    }).onOk(() => {
      editorStore.closeEditor();
    });
  }

  async function saveEditor() {
    try {
      const { created } = await editorStore.save();
      if (created) {
        await loadRows();
        $q.dialog({
          title: t('toolsPage.editor.createdTitle'),
          message: t('toolsPage.editor.createdMessage', { name: created.display_name || created.key }),
          cancel: { label: t('toolsPage.editor.createdLater'), flat: true, noCaps: true },
          ok: {
            label: t('toolsPage.editor.createdOpenDetail'),
            noCaps: true,
            unelevated: true,
            class: 'app-registry-primary-btn',
          },
          persistent: false,
        }).onOk(async () => {
          const fetched = await toolsStore.fetchTool(created.id || created.key);
          detailStore.openDetail(fetched);
        });
      } else {
        $q.notify({ type: 'positive', message: t('toolsPage.editor.saved') });
        await loadRows();
      }
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: parseKratosApiError(err).message || t('toolsPage.editor.saveFailed'),
      });
    }
  }

  async function onRunTest() {
    try {
      const result = await detailStore.runToolTest();
      if (result.status === 'success') {
        $q.notify({ type: 'positive', message: t('toolsPage.detail.testSuccess') });
      }
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: parseKratosApiError(err).message || t('toolsPage.detail.testFailed'),
      });
    }
  }

  async function onSaveConfig() {
    try {
      await detailStore.saveConfig();
      $q.notify({ type: 'positive', message: t('toolsPage.detail.configSaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: parseKratosApiError(err).message });
    }
  }

  async function onSaveConfigSchema(schemaJson: string) {
    try {
      await detailStore.saveConfigSchema(schemaJson);
      $q.notify({ type: 'positive', message: t('toolsPage.detail.schemaSaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: parseKratosApiError(err).message });
    }
  }

  async function onSaveOverride() {
    try {
      await detailStore.saveOverride();
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: parseKratosApiError(err).message || t('toolsPage.override.saveFailed'),
      });
    }
  }

  function onRemoveOverride(o: ToolAgentOverride) {
    $q.dialog({
      title: t('toolsPage.override.removeTitle'),
      message: t('toolsPage.override.removeMessage', { agent: o.agent_id }),
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await detailStore.removeOverride(o);
      } catch (err) {
        $q.notify({
          type: 'negative',
          message: parseKratosApiError(err).message || t('toolsPage.override.removeFailed'),
        });
      }
    });
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

  // 单 watch 合并筛选 + 分页：筛选变化先归一到第 1 页（page 变化复用同一 watch），避免双 watch 重复请求。
  watch([search, category, source, riskLevel, enabled, abnormal, page, pageSize], (newVals, oldVals) => {
    const filtersChanged = newVals.slice(0, 6).some((v, i) => v !== oldVals[i]);
    if (filtersChanged && page.value !== 1) {
      page.value = 1;
      return;
    }
    void loadRows();
  });
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
    source,
    riskLevel,
    enabled,
    abnormal,
    page,
    pageSize,
    error,
    selected,
    // computed
    pageMax,
    summaryCards,
    categoryOptions,
    sourceOptions,
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
    onEditorRequestClose,
    saveEditor,
    onRunTest,
    onSaveConfig,
    onSaveConfigSchema,
    onSaveOverride,
    onRemoveOverride,
  };
}
