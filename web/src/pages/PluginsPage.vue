<template>
  <q-page class="app-page-cream plugins-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">ADK Runner plugins</div>
        <h1 class="app-page-title">Plugin 管理</h1>
        <p class="app-page-subtitle">配置 ADK Runner 运行时插件，替代手工维护 ADK_RUNNER_PLUGINS 环境变量。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded color="primary" icon="history" label="运行记录" to="/plugins/runs" />
        <q-btn color="primary" rounded unelevated icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat bordered class="plugin-filter-card q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="search" class="col-12 col-md-4" dense outlined clearable debounce="250" label="搜索 Plugin">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select v-model="category" class="col-12 col-md-3" dense outlined clearable emit-value map-options label="类型" :options="categoryOptions" />
        <q-select v-model="enabled" class="col-12 col-md-2" dense outlined clearable emit-value map-options label="启用状态" :options="enabledOptions" />
        <q-input v-model="callbackPoint" class="col-12 col-md-3" dense outlined clearable label="Callback" placeholder="before_model" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card flat bordered class="plugin-table-card">
      <q-table
        flat
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        v-model:pagination="tablePagination"
        :rows-per-page-options="[10, 20, 50]"
        @request="onTableRequest"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="text-weight-bold">{{ props.row.name }}</div>
            <div class="text-caption text-grey-7">{{ props.row.key }}</div>
          </q-td>
        </template>
        <template #body-cell-description="props">
          <q-td :props="props">
            <div class="plugin-description">{{ props.row.description || "暂无说明" }}</div>
          </q-td>
        </template>
        <template #body-cell-category="props">
          <q-td :props="props">
            <q-chip dense square color="primary" text-color="white">{{ props.row.category }}</q-chip>
            <q-chip dense square :color="riskColor(props.row.risk_level)" text-color="white">{{ props.row.risk_level }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-callbacks="props">
          <q-td :props="props">
            <q-chip v-for="point in props.row.callback_points" :key="point" dense outline color="primary">{{ point }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-toggle :model-value="props.row.enabled" color="primary" :disable="!props.row.permissions?.can_toggle || togglingId === props.row.id" @update:model-value="toggleEnabled(props.row, Boolean($event))" />
          </q-td>
        </template>
        <template #body-cell-scope="props">
          <q-td :props="props">
            <q-chip dense outline>{{ props.row.scope || "global" }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-sort_order="props">
          <q-td :props="props">
            <div class="row items-center no-wrap q-gutter-xs">
              <span>{{ props.row.sort_order }}</span>
              <q-btn v-if="props.row.permissions?.can_edit_config" flat dense round icon="arrow_upward" @click="bumpSort(props.row, -10)" />
              <q-btn v-if="props.row.permissions?.can_edit_config" flat dense round icon="arrow_downward" @click="bumpSort(props.row, 10)" />
            </div>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn flat dense round color="primary" icon="visibility" :disable="!props.row.permissions?.can_view" @click="openDetail(props.row)">
              <q-tooltip>查看详情</q-tooltip>
            </q-btn>
            <q-btn flat dense round color="primary" icon="settings" :disable="!props.row.permissions?.can_edit_config" @click="openConfig(props.row)">
              <q-tooltip>编辑配置</q-tooltip>
            </q-btn>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card class="plugin-detail-card">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ detailTarget?.name }}</div>
            <div class="text-caption text-grey-7">{{ detailTarget?.key }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailTarget" class="q-gutter-md">
          <q-banner rounded class="bg-grey-2 text-grey-9">{{ detailTarget.description || "暂无说明" }}</q-banner>
          <div class="row q-col-gutter-sm">
            <div class="col-6"><b>类型：</b>{{ detailTarget.category }}</div>
            <div class="col-6"><b>风险：</b>{{ detailTarget.risk_level }}</div>
            <div class="col-6"><b>作用域：</b>{{ detailTarget.scope }}</div>
            <div class="col-6"><b>排序：</b>{{ detailTarget.sort_order }}</div>
            <div class="col-6"><b>调用次数：</b>{{ detailTarget.invoke_count }}</div>
            <div class="col-6"><b>阻断 / 错误：</b>{{ detailTarget.block_count }} / {{ detailTarget.error_count }}</div>
            <div class="col-6"><b>最近状态：</b>{{ detailTarget.last_status || "未调用" }}</div>
            <div class="col-6"><b>最近调用：</b>{{ formatDate(detailTarget.last_invoked_at) }}</div>
          </div>
          <q-expansion-item dense-toggle label="Agent 绑定">
            <div class="q-gutter-sm">
              <q-radio v-model="scopeMode" val="global" label="全局生效" />
              <q-radio v-model="scopeMode" val="agent" label="指定 Agent" />
              <q-input v-if="scopeMode === 'agent'" v-model="scopeAgentId" dense outlined label="Agent ID" />
              <q-btn color="primary" rounded unelevated label="保存作用域" :loading="savingScope" @click="saveScope" />
            </div>
          </q-expansion-item>
          <q-expansion-item dense-toggle default-opened label="Callback">
            <div class="q-gutter-xs">
              <q-chip v-for="point in detailTarget.callback_points" :key="point" dense outline color="primary">{{ point }}</q-chip>
              <span v-if="!detailTarget.callback_points.length" class="text-grey-7">暂无 Callback</span>
            </div>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="配置 JSON">
            <pre class="app-code-block app-code-block--compact">{{ prettyJSON(detailTarget.config_json, "暂无配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="默认配置">
            <pre class="app-code-block app-code-block--compact">{{ prettyJSON(detailTarget.default_config_json, "暂无默认配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="配置 Schema">
            <pre class="app-code-block app-code-block--compact">{{ prettyJSON(detailTarget.config_schema_json, "暂无 Schema") }}</pre>
          </q-expansion-item>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="configOpen" persistent>
      <q-card style="width: 760px; max-width: 94vw">
        <q-card-section>
          <div class="text-h6">配置 {{ configTarget?.name }}</div>
          <div class="text-caption text-grey-7">Schema 驱动表单或 JSON 编辑；保存后 Runner 热重载生效。</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <q-tabs v-model="configMode" dense align="left" class="text-primary">
            <q-tab name="form" label="表单" />
            <q-tab name="json" label="JSON" />
          </q-tabs>
          <q-tab-panels v-model="configMode" animated>
            <q-tab-panel name="form" class="q-pa-none">
              <PluginSchemaForm v-model="configText" :schema-json="configTarget?.config_schema_json || '{}'" />
            </q-tab-panel>
            <q-tab-panel name="json" class="q-pa-none">
              <q-input v-model="configText" type="textarea" autogrow outlined label="config_json" :error="Boolean(configError)" :error-message="configError" />
            </q-tab-panel>
          </q-tab-panels>
          <q-expansion-item icon="schema" label="默认配置 / Schema 参考">
            <pre class="app-code-block app-code-block--compact">{{ configTarget?.default_config_json || "{}" }}</pre>
            <pre class="app-code-block app-code-block--compact">{{ configTarget?.config_schema_json || "{}" }}</pre>
          </q-expansion-item>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated label="保存" :loading="savingConfig" :disable="Boolean(configError)" @click="saveConfig" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar, type QTableColumn, type QTableProps } from "quasar";
import { listPlugins, togglePluginEnabled, updatePluginConfig, updatePluginScope, updatePluginSortOrder } from "../features/plugins/usePluginsPage";
import type { Plugin } from "../features/plugins/usePluginsPage";
import PluginSchemaForm from "../components/plugins/PluginSchemaForm.vue";

const $q = useQuasar();
const rows = ref<Plugin[]>([]);
const total = ref(0);
const tablePagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });
const scopeMode = ref<"global" | "agent">("global");
const scopeAgentId = ref("");
const savingScope = ref(false);
const search = ref("");
const category = ref("");
const enabled = ref<boolean | null>(null);
const callbackPoint = ref("");
const loading = ref(false);
const error = ref("");
const togglingId = ref("");
const detailOpen = ref(false);
const detailTarget = ref<Plugin | null>(null);
const configOpen = ref(false);
const configTarget = ref<Plugin | null>(null);
const configText = ref("{}");
const configMode = ref<"form" | "json">("form");
const savingConfig = ref(false);

const categoryOptions = ["observability", "guard", "tracking", "debug", "routing", "policy"].map((value) => ({ label: value, value }));
const enabledOptions = [
  { label: "已启用", value: true },
  { label: "已停用", value: false }
];
const columns: QTableColumn<Plugin>[] = [
  { name: "name", label: "Plugin", field: "name", align: "left" as const },
  { name: "description", label: "说明", field: "description", align: "left" as const, style: "max-width: 280px; white-space: normal;" },
  { name: "category", label: "类型 / 风险", field: "category", align: "left" as const },
  { name: "callbacks", label: "Callback", field: "callback_points", align: "left" as const },
  { name: "enabled", label: "启用", field: "enabled", align: "center" as const },
  { name: "scope", label: "作用域", field: "scope", align: "left" as const },
  { name: "sort_order", label: "顺序", field: "sort_order", align: "left" as const },
  { name: "actions", label: "操作", field: "id", align: "right" as const }
];

const configError = computed(() => {
  try {
    JSON.parse(configText.value || "{}");
    return "";
  } catch (err) {
    return err instanceof Error ? err.message : "JSON 格式错误";
  }
});

async function loadRows(
  nextPage = tablePagination.value.page,
  nextPageSize = tablePagination.value.rowsPerPage
) {
  loading.value = true;
  error.value = "";
  try {
    const data = await listPlugins({
      search: search.value,
      category: category.value,
      enabled: enabled.value,
      callback_point: callbackPoint.value,
      page: nextPage,
      page_size: nextPageSize
    });
    rows.value = data.items;
    total.value = data.total;
    tablePagination.value = { page: data.page, rowsPerPage: data.page_size, rowsNumber: data.total };
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Plugin 失败";
  } finally {
    loading.value = false;
  }
}

const onTableRequest: QTableProps["onRequest"] = (props) => {
  void loadRows(props.pagination.page, props.pagination.rowsPerPage);
};

async function toggleEnabled(plugin: Plugin, next: boolean) {
  togglingId.value = plugin.id;
  try {
    const updated = await togglePluginEnabled(plugin.id, next);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: next ? "Plugin 已启用" : "Plugin 已停用" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新失败" });
  } finally {
    togglingId.value = "";
  }
}

function openDetail(plugin: Plugin) {
  detailTarget.value = plugin;
  scopeMode.value = plugin.scope && plugin.scope !== "global" ? "agent" : "global";
  scopeAgentId.value = scopeMode.value === "agent" ? plugin.scope : "";
  detailOpen.value = true;
}

function openConfig(plugin: Plugin) {
  configTarget.value = plugin;
  configText.value = prettyJSON(plugin.config_json || plugin.default_config_json || "{}", "{}");
  configMode.value = "form";
  configOpen.value = true;
}

async function saveConfig() {
  if (!configTarget.value || configError.value) return;
  savingConfig.value = true;
  try {
    const updated = await updatePluginConfig(configTarget.value.id, JSON.stringify(JSON.parse(configText.value)));
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    configOpen.value = false;
    $q.notify({ type: "positive", message: "Plugin 配置已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    savingConfig.value = false;
  }
}

function prettyJSON(value: string, emptyLabel = "{}") {
  try {
    const parsed = JSON.parse(value || "{}");
    if (parsed && typeof parsed === "object" && Object.keys(parsed).length === 0) {
      return emptyLabel;
    }
    return JSON.stringify(parsed, null, 2);
  } catch {
    return value || emptyLabel;
  }
}

async function bumpSort(plugin: Plugin, delta: number) {
  try {
    const updated = await updatePluginSortOrder(plugin.id, Math.max(0, plugin.sort_order + delta));
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: "执行顺序已更新" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新失败" });
  }
}

async function saveScope() {
  if (!detailTarget.value) return;
  savingScope.value = true;
  try {
    const scope = scopeMode.value === "global" ? "global" : scopeAgentId.value.trim();
    const updated = await updatePluginScope(detailTarget.value.id, scope);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    detailTarget.value = updated;
    $q.notify({ type: "positive", message: "作用域已保存，下次对话生效" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    savingScope.value = false;
  }
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function riskColor(risk: string) {
  if (risk === "high") return "negative";
  if (risk === "medium") return "warning";
  return "positive";
}

watch([search, category, enabled, callbackPoint], () => {
  tablePagination.value.page = 1;
  void loadRows(1, tablePagination.value.rowsPerPage);
});

onMounted(() => loadRows());
</script>

<style scoped lang="sass">
.plugins-page
  padding: var(--space-6)

.plugin-filter-card,
.plugin-table-card
  border-radius: 22px

.plugin-description
  color: var(--color-text-secondary)
  line-height: 1.45

.plugin-detail-card
  width: 760px
  max-width: 94vw

@media (max-width: 720px)
  .app-page-hero
    flex-direction: column
    align-items: stretch
</style>
