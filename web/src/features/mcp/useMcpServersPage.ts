import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import type { McpHealthTone, McpServerConfig, McpServerMetadata, McpServerRow } from './types';
import { parseJSON, mcpTestNotify } from './utils';
import { useMcpStore } from '../../stores/mcp';
import { useAuthStore } from '../../stores/auth';

export function useMcpServersPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const mcpStore = useMcpStore();
  const auth = useAuthStore();
  const { user } = storeToRefs(auth);
  const { servers, total, loading: storeLoading, usageSummary } = storeToRefs(mcpStore);

  const search = ref('');
  const page = ref(1);
  const pageSize = ref(20);
  const loading = storeLoading;
  const error = ref('');
  const testingId = ref('');
  const togglingId = ref('');
  const editorOpen = ref(false);
  const editingRow = ref<McpServerRow | null>(null);
  const credDialogOpen = ref(false);
  const credServer = ref<McpServerRow | null>(null);

  const credUserId = computed(() => {
    const id = user.value?.id;
    return id != null && id > 0 ? String(id) : '';
  });

  const credUserLabel = computed(() => {
    const u = user.value;
    if (!u) return '';
    return u.name || u.email || credUserId.value;
  });

  const rows = computed(() => servers.value);
  const enabledCount = computed(() => rows.value.filter((row) => row.enabled).length);
  const filteredRows = computed(() => rows.value);

  // MCP 采纳汇总（使用方数量）：后端 best-effort，可能为 null。
  const usageEnabledAgents = computed(() => usageSummary.value?.enabled_agent_count ?? null);
  const usageTotalAgents = computed(() => usageSummary.value?.total_agent_count ?? null);
  /** 有可用服务器但无 Agent 开门禁 → 显示闲置警告。 */
  const showUnusedWarning = computed(
    () => usageEnabledAgents.value === 0 && enabledCount.value > 0,
  );
  const usageSummaryText = computed(() => {
    if (usageEnabledAgents.value == null || usageTotalAgents.value == null) return '';
    return `${usageEnabledAgents.value} / ${usageTotalAgents.value} 个 Agent 已启用 MCP 能力（broker 或直连）`;
  });
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, total.value) / pageSize.value)));
  const pagedRows = computed(() => rows.value);

  async function loadRows(manual = false) {
    error.value = '';
    try {
      await mcpStore.loadServers({
        page: page.value,
        page_size: pageSize.value,
        search: search.value,
      });
      if (page.value > pageMax.value) page.value = pageMax.value;
      if (manual) {
        $q.notify({ type: 'positive', message: refreshFeedback(), timeout: 2500 });
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 MCP 服务器失败';
    }
  }

  // refreshFeedback summarizes list freshness after a manual refresh: row
  // count + latest background health probe time, so the operator knows how
  // fresh the 健康 column data is (probes run server-side on an interval).
  function refreshFeedback(): string {
    let latest = '';
    for (const row of rows.value) {
      const at = parseJSON<McpServerMetadata>(row.metadata_json, {}).last_health_at ?? '';
      if (at > latest) latest = at;
    }
    const base = `已刷新，共 ${total.value} 个服务器`;
    return latest ? `${base}，最近健康检测：${formatDate(latest)}` : base;
  }

  let skipNextPageWatch = false;
  watch(search, () => {
    if (page.value !== 1) {
      skipNextPageWatch = true;
      page.value = 1;
    }
    void loadRows();
  });
  watch([page, pageSize], () => {
    if (skipNextPageWatch) {
      skipNextPageWatch = false;
      return;
    }
    void loadRows();
  });

  onMounted(loadRows);

  function openCreate() {
    editingRow.value = null;
    editorOpen.value = true;
  }

  function openEdit(row: McpServerRow) {
    editingRow.value = row;
    editorOpen.value = true;
  }

  function openCredentials(row: McpServerRow) {
    if (!credUserId.value) {
      $q.notify({ type: 'warning', message: '请先登录后再配置用户凭据' });
      return;
    }
    credServer.value = row;
    credDialogOpen.value = true;
  }

  function onSaved(_row: McpServerRow) {
    void loadRows();
  }

  async function testRow(row: McpServerRow) {
    testingId.value = row.id;
    try {
      const result = await mcpStore.test(row.id);
      $q.notify(mcpTestNotify(result));
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '测试连接失败' });
    } finally {
      testingId.value = '';
    }
  }

  async function toggleEnabled(row: McpServerRow, enabled: boolean) {
    togglingId.value = row.id;
    try {
      await mcpStore.editServer(row.id, { enabled });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '更新启用状态失败' });
    } finally {
      togglingId.value = '';
    }
  }

  function confirmDelete(row: McpServerRow) {
    const config = parseJSON<McpServerConfig>(row.config_json, {});
    const name = row.name || row.key;
    const prefix = config.tool_prefix || row.key;
    const credentialNote = config.require_user_credentials ? '；所有用户已配置的凭据将一并删除' : '';
    $q.dialog({
      title: '确认删除该 MCP 服务器？',
      message: `删除「${name}」后，依赖该服务器的工具（mcp_${prefix}__*）将不可用${credentialNote}。`,
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await mcpStore.removeServer(row.id);
        $q.notify({ type: 'positive', message: 'MCP 服务器已删除' });
        await loadRows();
      } catch (err) {
        $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
      }
    });
  }

  function healthTone(row: McpServerRow): McpHealthTone {
    const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
    if (metadata.health_status === 'ok') return 'ok';
    if (metadata.health_status === 'error') return 'error';
    if (metadata.health_status === 'degraded') return 'degraded';
    if (metadata.last_error_message) return 'error';
    return 'unknown';
  }

  function healthTooltip(row: McpServerRow) {
    const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
    let base: string;
    if (metadata.last_error_message) base = metadata.last_error_message;
    else if (metadata.health_status === 'ok' && metadata.last_health_at)
      base = t('mcpPage.lastHealthSuccess', { at: formatDate(metadata.last_health_at) }, 'Last success: {at}');
    else if (!row.enabled) base = t('mcpPage.notEnabledNotTested');
    else base = t('mcpPage.notTested');
    // P2：工具发现摘要与连通性结论正交，作为附加信息尾随展示。
    if (typeof metadata.tool_count === 'number') {
      base += t('mcpPage.healthToolsDiscovered', { n: metadata.tool_count }, '; discovered {n} tools');
    } else if (metadata.tools_error_message) {
      base += t('mcpPage.healthToolsFailed', '; tool discovery failed');
    }
    return base;
  }

  function formatDate(value: string) {
    if (!value) return '-';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  return {
    rows,
    search,
    page,
    pageSize,
    pageMax,
    pagedRows,
    total,
    loading,
    error,
    testingId,
    togglingId,
    editorOpen,
    editingRow,
    credDialogOpen,
    credServer,
    credUserId,
    credUserLabel,
    enabledCount,
    filteredRows,
    usageEnabledAgents,
    usageTotalAgents,
    showUnusedWarning,
    usageSummaryText,
    loadRows,
    openCreate,
    openEdit,
    openCredentials,
    onSaved,
    testRow,
    toggleEnabled,
    confirmDelete,
    healthTone,
    healthTooltip,
  };
}
