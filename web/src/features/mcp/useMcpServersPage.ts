import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import type { McpServerConfig, McpServerMetadata, McpServerRow } from './types';
import { parseJSON } from './utils';
import { useMcpStore } from '../../stores/mcp';
import { useAuthStore } from '../../stores/auth';

export function useMcpServersPage() {
  const $q = useQuasar();
  const mcpStore = useMcpStore();
  const auth = useAuthStore();
  const { user } = storeToRefs(auth);

  const rows = ref<McpServerRow[]>([]);
  const search = ref('');
  const page = ref(1);
  const pageSize = ref(12);
  const loading = ref(false);
  const error = ref('');
  const testingId = ref('');
  const editorOpen = ref(false);
  const editingRow = ref<McpServerRow | null>(null);
  const credDialogOpen = ref(false);
  const credServer = ref<McpServerRow | null>(null);

  const credUserId = computed(() => {
    const id = user.value?.id;
    return id != null && id > 0 ? String(id) : '';
  });

  const enabledCount = computed(() => rows.value.filter((row) => row.enabled).length);
  const filteredRows = computed(() => {
    const keyword = search.value.trim().toLowerCase();
    if (!keyword) return rows.value;
    return rows.value.filter((row) => {
      const config = parseJSON<McpServerConfig>(row.config_json, {});
      const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
      return [
        row.key,
        row.name,
        row.description,
        config.transport,
        config.url,
        config.command,
        config.tool_prefix,
        metadata.health_status,
        metadata.last_error_message,
      ].some((value) =>
        String(value || '')
          .toLowerCase()
          .includes(keyword),
      );
    });
  });

  const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
  const pagedRows = computed(() => {
    const start = (page.value - 1) * pageSize.value;
    return filteredRows.value.slice(start, start + pageSize.value);
  });

  watch(search, () => {
    page.value = 1;
  });

  watch(filteredRows, () => {
    if (page.value > pageMax.value) page.value = pageMax.value;
  });

  onMounted(loadRows);

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      await mcpStore.loadServers();
      rows.value = mcpStore.servers as McpServerRow[];
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 MCP 服务器失败';
    } finally {
      loading.value = false;
    }
  }

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

  function onSaved(row: McpServerRow) {
    const index = rows.value.findIndex((item) => item.id === row.id);
    if (index >= 0) {
      rows.value[index] = row;
    } else {
      rows.value.push(row);
    }
  }

  async function testRow(row: McpServerRow) {
    testingId.value = row.id;
    try {
      const result = await mcpStore.test(row.id);
      $q.notify({ type: result.ok ? 'positive' : 'warning', message: result.message || result.status });
      await loadRows();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '测试连接失败' });
    } finally {
      testingId.value = '';
    }
  }

  function confirmDelete(row: McpServerRow) {
    $q.dialog({
      title: '确认删除该 MCP 服务器？',
      message: '删除后依赖该服务器的工具将不可用。',
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      try {
        await mcpStore.removeServer(row.id);
        rows.value = rows.value.filter((item) => item.id !== row.id);
        $q.notify({ type: 'positive', message: 'MCP 服务器已删除' });
      } catch (err) {
        $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
      }
    });
  }

  function healthTone(row: McpServerRow) {
    const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
    if (metadata.health_status === 'ok') return 'ok';
    if (metadata.health_status === 'error') return 'error';
    if (metadata.health_status === 'degraded') return 'degraded';
    if (metadata.last_error_message) return 'error';
    return 'unknown';
  }

  function healthTooltip(row: McpServerRow) {
    const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
    if (metadata.last_error_message) return metadata.last_error_message;
    if (metadata.health_status === 'ok' && metadata.last_health_at)
      return `最近成功：${formatDate(metadata.last_health_at)}`;
    if (!row.enabled) return '未启用 / 未检测';
    return '未检测';
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
    loading,
    error,
    testingId,
    editorOpen,
    editingRow,
    credDialogOpen,
    credServer,
    credUserId,
    enabledCount,
    filteredRows,
    loadRows,
    openCreate,
    openEdit,
    openCredentials,
    onSaved,
    testRow,
    confirmDelete,
    healthTone,
    healthTooltip,
  };
}
