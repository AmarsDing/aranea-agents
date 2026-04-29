<template>
  <q-page class="app-page-cream mcp-page q-pa-sm q-pa-md-md">
    <section class="mcp-hero">
      <div>
        <div class="mcp-kicker">Model Context Protocol</div>
        <h1 class="mcp-title">MCP 服务器</h1>
        <p class="mcp-subtitle">管理 Model Context Protocol 服务器连接、传输配置与健康状态。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat bordered class="mcp-toolbar q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="search" class="col-12 col-md-5" dense outlined clearable debounce="200" placeholder="搜索服务器...">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <div class="col-12 col-md text-caption text-grey-7">
          共 {{ filteredRows.length }} 个服务器，{{ enabledCount }} 个已启用
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card flat bordered class="mcp-list-card">
      <q-card-section v-if="loading" class="q-gutter-md">
        <q-skeleton v-for="item in 4" :key="item" type="rect" height="116px" />
      </q-card-section>

      <q-list v-else-if="filteredRows.length" padding separator>
        <q-item v-for="server in filteredRows" :key="server.id" class="mcp-list-item">
          <q-item-section side top>
            <span class="health-dot" :class="`health-dot--${healthTone(server)}`">
              <q-tooltip>{{ healthTooltip(server) }}</q-tooltip>
            </span>
          </q-item-section>
          <q-item-section>
            <McpServerItem
              :server="server"
              :testing="testingId === server.id"
              @edit="openEdit"
              @delete="confirmDelete"
              @test="testRow"
            />
          </q-item-section>
        </q-item>
      </q-list>

      <q-card-section v-else class="mcp-empty">
        <q-icon name="power" size="48px" color="grey-5" />
        <div class="text-subtitle1 text-weight-bold">暂无 MCP 服务器</div>
        <div class="text-caption text-grey-7">添加您的第一个 MCP 服务器以开始使用。</div>
        <q-btn color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
      </q-card-section>
    </q-card>

    <McpServerFormDialog v-model="editorOpen" :row="editingRow" @saved="onSaved" @tested="loadRows" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import McpServerFormDialog from "../features/mcp/McpServerFormDialog.vue";
import McpServerItem from "../features/mcp/McpServerItem.vue";
import { deleteMcpServer, listMcpServers, testMcpServer } from "../features/mcp/api";
import type { McpServerConfig, McpServerMetadata, McpServerRow } from "../features/mcp/types";

const $q = useQuasar();
const rows = ref<McpServerRow[]>([]);
const search = ref("");
const loading = ref(false);
const error = ref("");
const testingId = ref("");
const editorOpen = ref(false);
const editingRow = ref<McpServerRow | null>(null);

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
      metadata.last_error_message
    ].some((value) => String(value || "").toLowerCase().includes(keyword));
  });
});

onMounted(loadRows);

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    rows.value = await listMcpServers();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 MCP 服务器失败";
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

function onSaved(row: McpServerRow) {
  const index = rows.value.findIndex((item) => item.id === row.id);
  if (index >= 0) rows.value[index] = row;
  else rows.value.unshift(row);
}

async function testRow(row: McpServerRow) {
  testingId.value = row.id;
  try {
    const result = await testMcpServer(row.id);
    $q.notify({ type: result.ok ? "positive" : "warning", message: result.message || result.status });
    await loadRows();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "测试连接失败" });
  } finally {
    testingId.value = "";
  }
}

function confirmDelete(row: McpServerRow) {
  $q.dialog({
    title: "确认删除该 MCP 服务器？",
    message: "删除后依赖该服务器的工具将不可用。",
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await deleteMcpServer(row.id);
    rows.value = rows.value.filter((item) => item.id !== row.id);
    $q.notify({ type: "positive", message: "MCP 服务器已删除" });
  });
}

function healthTone(row: McpServerRow) {
  const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
  if (metadata.health_status === "ok") return "ok";
  if (metadata.health_status === "error" || metadata.last_error_message) return "error";
  if (metadata.health_status === "degraded") return "degraded";
  return "unknown";
}

function healthTooltip(row: McpServerRow) {
  const metadata = parseJSON<McpServerMetadata>(row.metadata_json, {});
  if (metadata.last_error_message) return metadata.last_error_message;
  if (metadata.health_status === "ok" && metadata.last_health_at) return `最近成功：${formatDate(metadata.last_health_at)}`;
  if (!row.enabled) return "未启用 / 未检测";
  return "未检测";
}

function formatDate(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
</script>

<style scoped>
.mcp-page {
  min-height: 100%;
}

.mcp-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.mcp-kicker {
  color: #9a6a4f;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.mcp-title {
  color: #4e342e;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.mcp-subtitle {
  color: #795548;
  margin: 0;
  max-width: 720px;
}

.mcp-toolbar,
.mcp-list-card {
  border-radius: 18px;
}

.mcp-list-item {
  align-items: flex-start;
  padding-left: 10px;
  padding-right: 10px;
}

.health-dot {
  border-radius: 999px;
  display: inline-block;
  height: 10px;
  margin-top: 22px;
  width: 10px;
}

.health-dot--ok {
  background: #21ba45;
}

.health-dot--error {
  background: #c10015;
}

.health-dot--degraded {
  background: #f2c037;
}

.health-dot--unknown {
  background: #9e9e9e;
}

.mcp-empty {
  align-items: center;
  color: #8d6e63;
  display: grid;
  gap: 8px;
  justify-items: center;
  min-height: 240px;
}

body.body--dark .mcp-title,
body.body--dark .mcp-kicker,
body.body--dark .mcp-subtitle {
  color: inherit;
}

@media (max-width: 720px) {
  .mcp-hero {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
