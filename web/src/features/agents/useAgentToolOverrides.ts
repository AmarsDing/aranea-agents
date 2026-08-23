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
  category: string;
  reason: string;
  enabled: boolean;
  effective_state: string;
  effective_requires_confirmation: boolean;
  catalog_requires_confirmation: boolean;
  tool_id: string;
  override: ToolAgentOverride | null;
};

export type AgentToolOverrideGroup = {
  category: string;
  rows: AgentToolOverrideRow[];
  overriddenCount: number;
};

export type AgentToolOverrideForm = {
  mode: string;
  requires_confirmation: boolean;
  config_override_json: string;
};

/** 覆盖是否为「纯模式」覆盖（无确认标记且无 JSON 配置），切回继承时可直接删除。 */
function isBareModeOverride(ov: ToolAgentOverride): boolean {
  const json = (ov.config_override_json ?? '').trim();
  return !ov.requires_confirmation && (!json || json === '{}');
}

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
  /** 行内快捷操作的行级加载标记（按 tool_key）。 */
  const pendingKeys = ref<Set<string>>(new Set());
  const form = ref<AgentToolOverrideForm>({
    mode: 'inherit',
    requires_confirmation: false,
    config_override_json: '{}',
  });

  // 列表过滤状态（原 Panel 本地状态上收，便于分组/过滤与数据同源）
  const search = ref('');
  const stateFilter = ref('');
  const groupFilter = ref('');
  const onlyOverridden = ref(false);

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
        category: it.category || 'custom',
        reason: it.reason,
        enabled: it.enabled,
        effective_state: it.effective_state,
        effective_requires_confirmation: catalogConfirm || Boolean(ov?.requires_confirmation),
        catalog_requires_confirmation: catalogConfirm,
        tool_id: it.tool_key,
        override: ov,
      };
    });
  });

  const toolsEnabled = computed(() => effective.value?.tools_enabled ?? true);

  const overriddenCount = computed(() => rows.value.filter((r) => r.override).length);

  const groupOptions = computed(() => {
    const cats = [...new Set(rows.value.map((r) => r.category))].sort();
    return cats.map((c) => ({ label: c, value: c }));
  });

  const filteredRows = computed(() => {
    const q = search.value.trim().toLowerCase();
    return rows.value.filter((r) => {
      if (stateFilter.value && r.effective_state !== stateFilter.value) return false;
      if (groupFilter.value && r.category !== groupFilter.value) return false;
      if (onlyOverridden.value && !r.override) return false;
      if (q && !r.tool_key.toLowerCase().includes(q) && !r.display_name.toLowerCase().includes(q)) return false;
      return true;
    });
  });

  const groupedRows = computed<AgentToolOverrideGroup[]>(() => {
    const byCat = new Map<string, AgentToolOverrideRow[]>();
    for (const r of filteredRows.value) {
      const list = byCat.get(r.category);
      if (list) list.push(r);
      else byCat.set(r.category, [r]);
    }
    return [...byCat.keys()].sort().map((category) => {
      const groupRows = byCat.get(category) ?? [];
      return {
        category,
        rows: groupRows,
        overriddenCount: groupRows.filter((r) => r.override).length,
      };
    });
  });

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

  /** 行内快捷操作统一出口：upsert/remove + 行级 pending + 错误提示 + 刷新。 */
  async function runQuickAction(row: AgentToolOverrideRow, action: () => Promise<void>) {
    const id = agentId.value?.trim();
    if (!id) return;
    const next = new Set(pendingKeys.value);
    next.add(row.tool_key);
    pendingKeys.value = next;
    try {
      await action();
      await reload();
    } catch (e) {
      $q.notify({ type: 'negative', message: parseKratosApiError(e).message || t('toolsPage.agentTools.saveFailed') });
    } finally {
      const done = new Set(pendingKeys.value);
      done.delete(row.tool_key);
      pendingKeys.value = done;
    }
  }

  /**
   * 行内切换覆盖模式（点击即存）：
   * - allow/deny → upsert，保留已有确认标记与 JSON 配置
   * - inherit → 纯模式覆盖直接删除（回到无覆盖自然态）；含确认/JSON 的降级为 inherit 保留配置
   */
  async function quickSetMode(row: AgentToolOverrideRow, mode: string) {
    const id = agentId.value?.trim();
    if (!id) return;
    const ov = row.override;
    if (mode === 'inherit') {
      if (!ov) return;
      await runQuickAction(row, async () => {
        if (isBareModeOverride(ov)) {
          await toolsStore.removeOverride(row.tool_id, id);
        } else {
          await toolsStore.saveOverride({
            tool_id: row.tool_id,
            agent_id: id,
            enabled: true,
            mode: 'inherit',
            requires_confirmation: ov.requires_confirmation,
            config_override_json: ov.config_override_json,
          });
        }
      });
      return;
    }
    await runQuickAction(row, async () => {
      await toolsStore.saveOverride({
        tool_id: row.tool_id,
        agent_id: id,
        // enabled 列在运行时不参与判定（启停由 mode 决定），恒置 true 归一化存储。
        enabled: true,
        mode,
        requires_confirmation: ov?.requires_confirmation ?? false,
        config_override_json: ov?.config_override_json ?? '{}',
      });
    });
  }

  /** 行内切换需确认（点击即存）：无覆盖行创建 inherit + confirm 覆盖。 */
  async function quickToggleConfirm(row: AgentToolOverrideRow) {
    const id = agentId.value?.trim();
    if (!id) return;
    const ov = row.override;
    await runQuickAction(row, async () => {
      await toolsStore.saveOverride({
        tool_id: row.tool_id,
        agent_id: id,
        enabled: true,
        mode: ov?.mode ?? 'inherit',
        requires_confirmation: !ov?.requires_confirmation,
        config_override_json: ov?.config_override_json ?? '{}',
      });
    });
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
    search,
    stateFilter,
    groupFilter,
    onlyOverridden,
    groupedRows,
    groupOptions,
    overriddenCount,
    pendingKeys,
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
    quickSetMode,
    quickToggleConfirm,
    requestRemoveOverride,
    confirmRemoveOverride,
    cancelRemoveOverride,
  };
}
