import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { defaultHookRuleConfig, parseHookConfig, type HookRow, type HookRuleConfig } from '../hooks/types';
import { useHooksStore } from '../../stores/hooks';

export function useAgentHooksPanel(agentId: () => string, agentKey: () => string) {
  const { t } = useI18n();
  const $q = useQuasar();
  const hooksStore = useHooksStore();
  const loading = ref(false);
  const saving = ref(false);
  const loadError = ref('');
  const rows = ref<HookRow[]>([]);
  const editorExpanded = ref(true);
  const draftRule = ref<HookRuleConfig>(defaultHookRuleConfig());
  const draftSort = ref(0);
  const editOpen = ref(false);
  const editRow = ref<HookRow | null>(null);
  const editRule = ref<HookRuleConfig>(defaultHookRuleConfig());
  const editSort = ref(0);

  function ruleOf(row: HookRow) {
    return parseHookConfig(row.config_json);
  }

  const scopedRows = computed(() => {
    const id = agentId().trim();
    const key = agentKey().trim();
    return rows.value.filter((row) => {
      const cond = ruleOf(row).condition?.agent_id?.trim() ?? '';
      if (!cond) return false;
      return cond === id || cond === key;
    });
  });

  function resetDraft() {
    draftRule.value = defaultHookRuleConfig(agentId(), agentKey());
    if (!draftRule.value.condition.agent_id) {
      draftRule.value.condition.agent_id = agentId() || agentKey();
    }
    draftSort.value = 0;
  }

  watch(
    () => [agentId(), agentKey()] as const,
    () => resetDraft(),
    { immediate: true },
  );

  async function loadRows() {
    loading.value = true;
    loadError.value = '';
    try {
      rows.value = await hooksStore.loadHooks();
    } catch (e) {
      loadError.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  async function createScopedHook() {
    const prefix = agentKey() || agentId() || 'hook';
    const key = `${prefix}-hook-${Date.now()}`.replace(/[^a-zA-Z0-9_-]/g, '_');
    saving.value = true;
    try {
      await hooksStore.addHook({
        key,
        name: `${prefix} callback`,
        enabled: true,
        sort_order: draftSort.value,
        rule: draftRule.value,
      });
      await loadRows();
      $q.notify({ type: 'positive', message: t('hooksPage.agentPanel.notifyCreated') });
      resetDraft();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  function openEdit(row: HookRow) {
    editRow.value = row;
    editRule.value = ruleOf(row);
    editSort.value = row.sort_order;
    editOpen.value = true;
  }

  async function saveEdit() {
    if (!editRow.value) return;
    saving.value = true;
    try {
      await hooksStore.saveHook(editRow.value.id, { sort_order: editSort.value, rule: editRule.value });
      editOpen.value = false;
      await loadRows();
      $q.notify({ type: 'positive', message: t('hooksPage.agentPanel.notifySaved') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  onMounted(loadRows);

  return {
    loading,
    saving,
    loadError,
    scopedRows,
    editorExpanded,
    draftRule,
    draftSort,
    editOpen,
    editRow,
    editRule,
    editSort,
    agentId,
    agentKey,
    createScopedHook,
    openEdit,
    saveEdit,
    reload: loadRows,
  };
}
