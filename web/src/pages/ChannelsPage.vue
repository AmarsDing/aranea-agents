<template>
  <q-page class="app-page-cream channels-page">
    <section class="channels-hero">
      <div>
        <div class="channels-kicker">Channel management</div>
        <h1 class="channels-title">Channel 管理</h1>
        <p class="channels-subtitle">统一管理外部消息渠道、凭据引用、Webhook 与运行时启停。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="add" label="新增 Channel" @click="openCreate" />
        <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </div>
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="search" class="col-12 col-md-4" dense outlined clearable debounce="200" label="搜索 Channel">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select v-model="typeFilter" class="col-12 col-md-3" dense outlined clearable emit-value map-options label="平台类型" :options="typeOptions" />
        <q-select v-model="statusFilter" class="col-12 col-md-3" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <q-card flat bordered>
      <q-table flat :rows="filteredRows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 12 }">
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="row items-center no-wrap q-gutter-xs">
              <q-icon v-if="isConnected(props.row)" name="circle" color="positive" size="10px">
                <q-tooltip>连接正常</q-tooltip>
              </q-icon>
              <div class="text-weight-bold">{{ props.row.name }}</div>
            </div>
            <div class="text-caption text-grey-7">{{ props.row.key }}</div>
          </q-td>
        </template>
        <template #body-cell-type="props">
          <q-td :props="props">
            <q-chip dense square color="primary" text-color="white">{{ catalogLabel(channelType(props.row)) }}</q-chip>
            <q-chip dense outline>{{ receiveMode(props.row) }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <div class="row items-center no-wrap q-gutter-xs">
              <q-icon v-if="isConnected(props.row)" name="circle" color="positive" size="10px" />
              <q-badge :color="props.row.enabled ? statusColor(props.row.status) : 'grey'">
                {{ props.row.enabled ? statusText(props.row) : "disabled" }}
              </q-badge>
            </div>
            <div v-if="metadata(props.row).last_error_message" class="text-caption text-negative ellipsis">
              {{ metadata(props.row).last_error_message }}
            </div>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-toggle :model-value="props.row.enabled" color="primary" :disable="togglingId === props.row.id" @update:model-value="toggleRow(props.row, Boolean($event))" />
          </q-td>
        </template>
        <template #body-cell-updated="props">
          <q-td :props="props">{{ formatDate(props.row.updated_at) }}</q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props" class="q-gutter-xs">
            <q-btn flat dense round icon="science" color="primary" :loading="testingId === props.row.id" @click="testRow(props.row)">
              <q-tooltip>测试连接</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)">
              <q-tooltip>编辑</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="delete" color="negative" @click="confirmDelete(props.row)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <ChannelEditorDialog
      v-model="editorOpen"
      :catalog="catalog"
      :row="editingRow"
      :credentials="editingCredentials"
      @saved="onSaved"
      @tested="loadAll"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useQuasar, type QTableColumn } from "quasar";
import ChannelEditorDialog from "../features/channels/ChannelEditorDialog.vue";
import { deleteChannel, listChannelCatalog, listChannelCredentials, listChannels, testChannel, toggleChannel } from "../features/channels/api";
import type { ChannelCatalogItem, ChannelConfig, ChannelCredential, ChannelMetadata, ChannelRow } from "../features/channels/types";

const $q = useQuasar();
const catalog = ref<ChannelCatalogItem[]>([]);
const rows = ref<ChannelRow[]>([]);
const loading = ref(false);
const error = ref("");
const search = ref("");
const typeFilter = ref("");
const statusFilter = ref("");
const togglingId = ref("");
const testingId = ref("");
const editorOpen = ref(false);
const editingRow = ref<ChannelRow | null>(null);
const editingCredentials = ref<ChannelCredential[]>([]);

const columns: QTableColumn<ChannelRow>[] = [
  { name: "name", label: "名称", field: "name", align: "left" },
  { name: "type", label: "平台", field: "config_json", align: "left" },
  { name: "status", label: "连接状态", field: "status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "center" },
  { name: "updated", label: "最近更新", field: "updated_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];

const typeOptions = computed(() => catalog.value.map((item) => ({ label: item.label, value: item.type })));
const statusOptions = [
  { label: "启用", value: "enabled" },
  { label: "停用", value: "disabled" },
  { label: "正常", value: "active" },
  { label: "待授权", value: "pending_auth" },
  { label: "异常", value: "error" }
];

const filteredRows = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    const cfg = config(row);
    const meta = metadata(row);
    if (typeFilter.value && cfg.type !== typeFilter.value) return false;
    if (statusFilter.value === "enabled" && !row.enabled) return false;
    if (statusFilter.value === "disabled" && row.enabled) return false;
    if (!["", "enabled", "disabled"].includes(statusFilter.value) && row.status !== statusFilter.value) return false;
    if (!keyword) return true;
    return [row.name, row.key, row.description, cfg.type, meta.external_id].some((value) => String(value || "").toLowerCase().includes(keyword));
  });
});

onMounted(loadAll);

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [catalogRows, channelRows] = await Promise.all([listChannelCatalog(), listChannels()]);
    catalog.value = catalogRows;
    rows.value = channelRows;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Channel 失败";
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingRow.value = null;
  editingCredentials.value = [];
  editorOpen.value = true;
}

async function openEdit(row: ChannelRow) {
  editingRow.value = row;
  editingCredentials.value = await listChannelCredentials(row.id);
  editorOpen.value = true;
}

function onSaved(row: ChannelRow) {
  const index = rows.value.findIndex((item) => item.id === row.id);
  if (index >= 0) rows.value[index] = row;
  else rows.value.unshift(row);
}

async function toggleRow(row: ChannelRow, enabled: boolean) {
  togglingId.value = row.id;
  try {
    const updated = await toggleChannel(row.id, enabled);
    onSaved(updated);
    $q.notify({ type: "positive", message: enabled ? "Channel 已启用" : "Channel 已停用" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "启停失败" });
  } finally {
    togglingId.value = "";
  }
}

async function testRow(row: ChannelRow) {
  testingId.value = row.id;
  try {
    const result = await testChannel(row.id);
    $q.notify({ type: result.ok ? "positive" : "warning", message: result.message || result.status });
    await loadAll();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "测试失败" });
  } finally {
    testingId.value = "";
  }
}

function confirmDelete(row: ChannelRow) {
  $q.dialog({
    title: "确认删除该 Channel？",
    message: "删除后将停止运行时加载，第三方 Webhook 需要自行解绑。",
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await deleteChannel(row.id);
    rows.value = rows.value.filter((item) => item.id !== row.id);
    $q.notify({ type: "positive", message: "Channel 已删除" });
  });
}

function config(row: ChannelRow): ChannelConfig {
  return parseJSON<ChannelConfig>(row.config_json, {});
}

function metadata(row: ChannelRow): ChannelMetadata {
  return parseJSON<ChannelMetadata>(row.metadata_json, {});
}

function channelType(row: ChannelRow) {
  return config(row).type || "unknown";
}

function receiveMode(row: ChannelRow) {
  return config(row).receive_mode || "-";
}

function catalogLabel(type: string) {
  return catalog.value.find((item) => item.type === type)?.label || type;
}

function statusColor(status: string) {
  if (status === "active") return "positive";
  if (status === "error") return "negative";
  if (status === "pending_auth") return "warning";
  return "grey";
}

function statusText(row: ChannelRow) {
  if (isConnected(row)) return "connected";
  return row.status || "unknown";
}

function isConnected(row: ChannelRow) {
  const meta = metadata(row);
  return row.enabled && row.status === "active" && !meta.last_error_message;
}

function formatDate(value: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
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
.channels-hero {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;
}

.channels-kicker {
  color: var(--q-primary);
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.channels-title {
  margin: 4px 0;
  font-size: 30px;
  font-weight: 800;
}

.channels-subtitle {
  margin: 0;
  color: #667085;
}
</style>
