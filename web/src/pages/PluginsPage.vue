<template>
  <q-page class="app-standard-page app-registry-page plugins-page">
    <AppPageHero
      kicker="ADK Runner plugins"
      title="Plugin 管理"
      subtitle="配置 ADK Runner 运行时插件，替代手工维护 ADK_RUNNER_PLUGINS 环境变量。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="history" label="运行记录" to="/plugins/runs" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="250" label="搜索 Plugin">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select v-model="category" class="app-page-toolbar__field" dense outlined clearable emit-value map-options label="类型" :options="categoryOptions" />
      <q-select v-model="enabled" class="app-page-toolbar__field" dense outlined clearable emit-value map-options label="启用状态" :options="enabledOptions" />
      <q-input v-model="callbackPoint" class="app-page-toolbar__field" dense outlined clearable label="Callback" placeholder="before_model" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="() => loadRows()" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="() => loadRows()" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="extension" />
        <div class="text-h6 q-mt-md">{{ search ? "没有匹配的 Plugin" : "暂无 Plugin" }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">调整筛选条件或刷新列表。</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <AppRegistryTable
        table-class="plugins-table"
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.description" empty-label="暂无说明">
              <div class="plugins-table__name-hit min-width-0">
                <div class="app-registry-cell-primary">{{ props.row.name }}</div>
                <div class="app-registry-cell-sub">{{ props.row.key }}</div>
              </div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-category="props">
          <q-td :props="props">
            <div class="app-registry-chip-wrap">
              <q-chip dense square color="primary" text-color="white">{{ props.row.category }}</q-chip>
              <q-chip dense square :color="riskColor(props.row.risk_level)" text-color="white">{{ props.row.risk_level }}</q-chip>
            </div>
          </q-td>
        </template>
        <template #body-cell-callbacks="props">
          <q-td :props="props">
            <span class="plugins-table__callbacks-text" :title="formatCallbacksSummary(props.row.callback_points)">
              {{ formatCallbacksSummary(props.row.callback_points) }}
            </span>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-toggle
              :model-value="props.row.enabled"
              color="primary"
              :disable="!props.row.permissions?.can_toggle || togglingId === props.row.id"
              @update:model-value="toggleEnabled(props.row, Boolean($event))"
            />
          </q-td>
        </template>
        <template #body-cell-scope="props">
          <q-td :props="props">
            <q-chip dense outline>{{ props.row.scope || "global" }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round color="primary" icon="visibility" :disable="!props.row.permissions?.can_view" @click="openDetail(props.row)">
                <q-tooltip>查看详情</q-tooltip>
              </q-btn>
              <q-btn flat dense round color="primary" icon="settings" :disable="!props.row.permissions?.can_edit_config" @click="openConfig(props.row)">
                <q-tooltip>编辑配置</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        label="个 Plugin"
      />
    </template>

    <q-dialog v-model="detailOpen">
      <q-card class="plugin-detail-card app-dialog-card app-dialog-card--md">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ detailTarget?.name }}</div>
            <div class="text-caption text-grey-7">{{ detailTarget?.key }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailTarget" class="q-gutter-md">
          <q-banner rounded dense class="app-banner-warning">{{ detailTarget.description || "暂无说明" }}</q-banner>
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
          <div v-if="detailTarget.permissions?.can_edit_config" class="row q-gutter-sm">
            <q-btn outline dense no-caps icon="arrow_upward" label="上移顺序" @click="bumpSort(detailTarget, -10)" />
            <q-btn outline dense no-caps icon="arrow_downward" label="下移顺序" @click="bumpSort(detailTarget, 10)" />
          </div>
          <q-expansion-item dense-toggle label="Agent 绑定">
            <div class="q-gutter-sm">
              <q-radio v-model="scopeMode" val="global" label="全局生效" />
              <q-radio v-model="scopeMode" val="agent" label="指定 Agent" />
              <q-input v-if="scopeMode === 'agent'" v-model="scopeAgentId" dense outlined label="Agent ID" />
              <q-btn color="primary" rounded unelevated no-caps label="保存作用域" :loading="savingScope" @click="saveScope" />
            </div>
          </q-expansion-item>
          <q-expansion-item dense-toggle default-opened label="Callback">
            <div class="app-registry-chip-wrap">
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
      <q-card class="app-dialog-card app-dialog-card--md">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">配置 {{ configTarget?.name }}</div>
            <div class="text-caption text-grey-7">Schema 驱动表单或 JSON 编辑；保存后 Runner 热重载生效。</div>
          </div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body q-gutter-md">
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
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="savingConfig" :disable="Boolean(configError)" @click="saveConfig" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar, type QTableColumn } from "quasar";
import { storeToRefs } from "pinia";
import { usePluginsStore } from "../stores/plugins";
import type { Plugin } from "../features/plugins/types";
import PluginSchemaForm from "../components/plugins/PluginSchemaForm.vue";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../components/layout/AppRegistryHoverTip.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import { registryColWidth } from "../features/ui/registryTableColumns";

const $q = useQuasar();
const pluginsStore = usePluginsStore();
const { plugins: storePlugins, total: storeTotal } = storeToRefs(pluginsStore);
const rows = ref<Plugin[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
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
  { name: "name", label: "Plugin", field: "name", align: "left", ...registryColWidth("14%") },
  { name: "category", label: "类型 / 风险", field: "category", align: "left", ...registryColWidth("11%") },
  { name: "callbacks", label: "Callback", field: "callback_points", align: "left", ...registryColWidth("13%") },
  { name: "enabled", label: "启用", field: "enabled", align: "center", ...registryColWidth("64px") },
  { name: "scope", label: "作用域", field: "scope", align: "left", ...registryColWidth("72px") },
  { name: "actions", label: "操作", field: "id", align: "right", ...registryColWidth("108px") }
];

const configError = computed(() => {
  try {
    JSON.parse(configText.value || "{}");
    return "";
  } catch (err) {
    return err instanceof Error ? err.message : "JSON 格式错误";
  }
});

async function loadRows(nextPage = page.value, nextPageSize = pageSize.value) {
  loading.value = true;
  error.value = "";
  try {
    await pluginsStore.loadPlugins({
      search: search.value,
      category: category.value,
      enabled: enabled.value,
      callback_point: callbackPoint.value,
      page: nextPage,
      page_size: nextPageSize
    });
    rows.value = [...storePlugins.value];
    total.value = storeTotal.value;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Plugin 失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  search.value = "";
  category.value = "";
  enabled.value = null;
  callbackPoint.value = "";
  page.value = 1;
  void loadRows(1, pageSize.value);
}

async function toggleEnabled(plugin: Plugin, next: boolean) {
  togglingId.value = plugin.id;
  try {
    const updated = await pluginsStore.toggle(plugin.id, next);
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
    const updated = await pluginsStore.setConfig(configTarget.value.id, JSON.stringify(JSON.parse(configText.value)));
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
    const updated = await pluginsStore.bumpSort(plugin.id, Math.max(0, plugin.sort_order + delta));
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    if (detailTarget.value?.id === updated.id) detailTarget.value = updated;
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
    const updated = await pluginsStore.setScope(detailTarget.value.id, scope);
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

const CALLBACK_CHIP_LIMIT = 2;

function visibleCallbackPoints(points?: string[]) {
  return (points ?? []).slice(0, CALLBACK_CHIP_LIMIT);
}

function hiddenCallbackCount(points?: string[]) {
  return Math.max(0, (points ?? []).length - CALLBACK_CHIP_LIMIT);
}

watch([search, category, enabled, callbackPoint], () => {
  page.value = 1;
  void loadRows(1, pageSize.value);
});

watch([page, pageSize], () => void loadRows());

onMounted(() => loadRows());
</script>