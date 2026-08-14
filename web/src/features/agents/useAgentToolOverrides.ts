import { computed, onMounted, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { overrideModeOptions } from '../../components/tools/toolUi';
import type { AgentEffectiveTools, Tool, ToolAgentOverride } from '../tools/types';
import { useToolsStore } from '../../stores/tools';
import { parseKratosApiError } from '../../utils/kratosError';

export type AgentToolOverrideRow = {
  tool_key: string;
  display_name: string;
  enabled: boolean;
  effective_state: string;
  effective_requires_confirmation: boolean;
  tool_id: string;
  override: ToolAgentOverride | null;
};

export type AgentToolOverrideForm = {
  mode: string;
  requires_confirmation: boolean;
  config_override_json: string;
};

/** Agent settings: effective tools matrix + per-agent overrides (store lives here, not in components). */
export function useAgentToolOverrides(agentId: Ref<string>) {
  const $q = useQuasar();
  const { t } = useI18n();
  const toolsStore = useToolsStore();
  const loading = ref(false);
  const saving = ref(false);
  const effective = ref<AgentEffectiveTools | null>(null);
  const overrides = ref<ToolAgentOverride[]>([]);
  const catalogByKey = ref<Record<string, Tool>>({});
  const editorOpen = ref(false);
  const editingRow = ref<AgentToolOverrideRow | null>(null);
  const confirmRemoveOpen = ref(false);
  const pendingRemoveRow = ref<AgentToolOverrideRow | null>(null);
  const form = ref<AgentToolOverrideForm>({
    mode: 'inherit',
    requires_confirmation: false,
    config_override_json: '{}',
  });

  const modeOptions = computed(() => overrideModeOptions());

  const rows = computed<AgentToolOverrideRow[]>(() => {
    const ovByKey = new Map(overrides.value.map((o) => [o.tool_key, o]));
    const items = effective.value?.items ?? [];
    return items.map((it) => {
      const cat = catalogByKey.value[it.tool_key];
      const ov = ovByKey.get(it.tool_key) ?? null;
      const catalogConfirm = Boolean(cat?.requires_confirmation);
      return {
        tool_key: it.tool_key,
        display_name: it.display_name || it.tool_key,
        enabled: it.enabled,
        effective_state: it.effective_state,
        effective_requires_confirmation: catalogConfirm || Boolean(ov?.requires_confirmation),
        tool_id: it.tool_key,
        override: ov,
      };
    });
  });

  const toolsEnabled = computed(() => effective.value?.tools_enabled ?? true);

  function modeLabel(mode: string): string {
    return overrideModeOptions().find((o) => o.value === mode)?.label ?? mode;
  }

  function effectiveStateLabel(state: string): string {
    if (state === 'allowed') return t('toolsPage.agentTools.stateAllowed');
    if (state === 'denied') return t('toolsPage.agentTools.stateDenied');
    return state;
  }

  async function reload() {
    const id = agentId.value?.trim();
    if (!id) return;
    loading.value = true;
    try {
      const [eff, ovs, catalog] = await Promise.all([
        toolsStore.fetchEffectiveTools(id),
        toolsStore.fetchOverridesByAgent(id),
        toolsStore.fetchCatalog({ page: 1, page_size: 500 }),
      ]);
      effective.value = eff;
      overrides.value = ovs;
      const map: Record<string, Tool> = {};
      for (const t of catalog.items ?? []) {
        map[t.key] = t;
      }
      catalogByKey.value = map;
    } catch (e) {
      $q.notify({ type: 'negative', message: parseKratosApiError(e).message || t('toolsPage.agentTools.loadFailed') });
    } finally {
      loading.value = false;
    }
  }

  function openEditor(row: AgentToolOverrideRow) {
    editingRow.value = row;
    const o = row.override;
    form.value = {
      mode: o?.mode ?? 'inherit',
      requires_confirmation: o?.requires_confirmation ?? false,
      config_override_json: o?.config_override_json ?? '{}',
    };
    editorOpen.value = true;
  }

  async function saveOverride() {
    const row = editingRow.value;
    const id = agentId.value?.trim();
    if (!row || !id) return;
    try {
      JSON.parse(form.value.config_override_json.trim() || '{}');
    } catch {
      $q.notify({ type: 'negative', message: t('agentSettings.toolOverrideInvalidJson') });
      return;
    }
    saving.value = true;
    try {
      await toolsStore.saveOverride({
        tool_id: row.tool_id,
        agent_id: id,
        mode: form.value.mode,
        // enabled 列在运行时不参与判定（启停由 mode 决定），恒置 true 归一化存储。
        enabled: true,
        requires_confirmation: form.value.requires_confirmation,
        config_override_json: form.value.config_override_json,
      });
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: t('toolsPage.agentTools.saved') });
      await reload();
    } catch (e) {
      $q.notify({ type: 'negative', message: parseKratosApiError(e).message || t('toolsPage.agentTools.saveFailed') });
    } finally {
      saving.value = false;
    }
  }

  function requestRemoveOverride(row: AgentToolOverrideRow) {
    const id = agentId.value?.trim();
    if (!row.override || !id) return;
    pendingRemoveRow.value = row;
    confirmRemoveOpen.value = true;
  }

  async function confirmRemoveOverride() {
    const row = pendingRemoveRow.value;
    const id = agentId.value?.trim();
    if (!row?.override || !id) return;
    confirmRemoveOpen.value = false;
    try {
      await toolsStore.removeOverride(row.tool_id, id);
      $q.notify({ type: 'positive', message: t('toolsPage.agentTools.removed') });
      await reload();
    } catch (e) {
      $q.notify({ type: 'negative', message: parseKratosApiError(e).message || t('toolsPage.agentTools.removeFailed') });
    } finally {
      pendingRemoveRow.value = null;
    }
  }

  function cancelRemoveOverride() {
    confirmRemoveOpen.value = false;
    pendingRemoveRow.value = null;
  }

  watch(agentId, () => void reload());
  onMounted(() => void reload());

  return {
    loading,
    saving,
    toolsEnabled,
    rows,
    editorOpen,
    editingRow,
    confirmRemoveOpen,
    pendingRemoveRow,
    form,
    modeOptions,
    modeLabel,
    effectiveStateLabel,
    reload,
    openEditor,
    saveOverride,
    requestRemoveOverride,
    confirmRemoveOverride,
    cancelRemoveOverride,
  };
}
