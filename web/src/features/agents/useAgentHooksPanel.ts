import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  cloneHookRuleConfig,
  defaultHookRuleConfig,
  parseHookConfig,
  type HookRow,
  type HookRuleConfig,
} from '../hooks/types';
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
  const draftName = ref('');
  const draftEnabled = ref(true);
  const editOpen = ref(false);
  const editRow = ref<HookRow | null>(null);
  const editRule = ref<HookRuleConfig>(defaultHookRuleConfig());
  const editSort = ref(0);
  const editName = ref('');
  const editEnabled = ref(true);
  const togglingId = ref('');
  const draftValid = ref(true);
  const editValid = ref(true);

  function ruleOf(row: HookRow) {
    return parseHookConfig(row.config_json);
  }

  function currentAgentRef() {
    return agentId().trim() || agentKey().trim();
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

  const globalRows = computed(() => rows.value.filter((row) => !(ruleOf(row).condition?.agent_id?.trim() ?? '')));

  function resetDraft() {
    draftRule.value = defaultHookRuleConfig(agentId(), agentKey());
    if (!draftRule.value.condition.agent_id) {
      draftRule.value.condition.agent_id = agentId() || agentKey();
    }
    draftSort.value = 0;
    draftName.value = '';
    draftEnabled.value = true;
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

  function upsertLocalRow(row: HookRow) {
    rows.value = [row, ...rows.value.filter((r) => r.id !== row.id)];
  }

  function replaceLocalRow(row: HookRow) {
    rows.value = rows.value.map((r) => (r.id === row.id ? row : r));
  }

  async function createScopedHook() {
    if (!draftValid.value) {
      $q.notify({ type: 'warning', message: t('hooksPage.notifyFixErrors') });
      return;
    }
    const prefix = agentKey() || agentId() || 'hook';
    const key = `${prefix}-hook-${Date.now()}`.replace(/[^a-zA-Z0-9_-]/g, '_');
    const rule = cloneHookRuleConfig(draftRule.value);
    rule.condition.agent_id = currentAgentRef();
    const name = draftName.value.trim() || t('hooksPage.agentPanel.defaultHookName', { prefix });
    saving.value = true;
    try {
      const created = await hooksStore.addHook({
        key,
        name,
        enabled: draftEnabled.value,
        sort_order: draftSort.value,
        rule,
      });
      upsertLocalRow(created);
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
    editName.value = row.name;
    editEnabled.value = row.enabled;
    editOpen.value = true;
  }

  async function saveEdit() {
    if (!editRow.value) return;
    if (!editValid.value) {
      $q.notify({ type: 'warning', message: t('hooksPage.notifyFixErrors') });
      return;
    }
    saving.value = true;
    try {
      const rule = cloneHookRuleConfig(editRule.value);
      rule.condition.agent_id = currentAgentRef();
      const updated = await hooksStore.saveHook(editRow.value.id, {
        name: editName.value.trim() || editRow.value.name,
        enabled: editEnabled.value,
        sort_order: editSort.value,
        rule,
      });
      replaceLocalRow(updated);
      editOpen.value = false;
      $q.notify({ type: 'positive', message: t('hooksPage.agentPanel.notifySaved') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      saving.value = false;
    }
  }

  async function toggleEnabled(row: HookRow, value: boolean) {
    togglingId.value = row.id;
    try {
      const updated = await hooksStore.saveHook(row.id, { enabled: value });
      replaceLocalRow(updated);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      togglingId.value = '';
    }
  }

  function confirmRemove(row: HookRow): Promise<void> {
    return $q
      .dialog({
        title: t('hooksPage.confirmDeleteTitle'),
        message: t('hooksPage.confirmDeleteMessage', { name: row.name }),
        cancel: true,
        persistent: true,
      })
      .onOk(async () => {
        try {
          await hooksStore.removeHook(row.id);
          rows.value = rows.value.filter((r) => r.id !== row.id);
          $q.notify({ type: 'positive', message: t('hooksPage.notifyDeleted') });
        } catch (e) {
          $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
        }
      });
  }

  onMounted(loadRows);

  return {
    loading,
    saving,
    loadError,
    scopedRows,
    globalRows,
    editorExpanded,
    draftRule,
    draftSort,
    draftName,
    draftEnabled,
    editOpen,
    editRow,
    editRule,
    editSort,
    editName,
    editEnabled,
    togglingId,
    draftValid,
    editValid,
    agentId,
    agentKey,
    createScopedHook,
    openEdit,
    saveEdit,
    toggleEnabled,
    confirmRemove,
    reload: loadRows,
  };
}
