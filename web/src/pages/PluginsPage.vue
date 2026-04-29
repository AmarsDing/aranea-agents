<template>
  <q-page class="app-page-cream plugins-page">
    <section class="plugins-hero">
      <div>
        <div class="plugins-kicker">ADK Runner plugins</div>
        <h1 class="plugins-title">Plugin 管理</h1>
        <p class="plugins-subtitle">配置 ADK Runner 运行时插件，替代手工维护 ADK_RUNNER_PLUGINS 环境变量。</p>
      </div>
      <q-btn color="primary" rounded unelevated icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
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
      <q-table flat :rows="rows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: pageSize }">
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
          <q-expansion-item dense-toggle default-open label="Callback">
            <div class="q-gutter-xs">
              <q-chip v-for="point in detailTarget.callback_points" :key="point" dense outline color="primary">{{ point }}</q-chip>
              <span v-if="!detailTarget.callback_points.length" class="text-grey-7">暂无 Callback</span>
            </div>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="配置 JSON">
            <pre class="plugin-json-preview">{{ prettyJSON(detailTarget.config_json, "暂无配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="默认配置">
            <pre class="plugin-json-preview">{{ prettyJSON(detailTarget.default_config_json, "暂无默认配置") }}</pre>
          </q-expansion-item>
          <q-expansion-item dense-toggle label="配置 Schema">
            <pre class="plugin-json-preview">{{ prettyJSON(detailTarget.config_schema_json, "暂无 Schema") }}</pre>
          </q-expansion-item>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="configOpen" persistent>
      <q-card style="width: 760px; max-width: 94vw">
        <q-card-section>
          <div class="text-h6">配置 {{ configTarget?.name }}</div>
          <div class="text-caption text-grey-7">当前为 JSON 配置；保存后 Runner 模式会读取页面启用状态。</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <q-input v-model="configText" type="textarea" autogrow outlined label="config_json" :error="Boolean(configError)" :error-message="configError" />
          <q-expansion-item icon="schema" label="默认配置 / Schema">
            <pre class="plugin-json-preview">{{ configTarget?.default_config_json || "{}" }}</pre>
            <pre class="plugin-json-preview">{{ configTarget?.config_schema_json || "{}" }}</pre>
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
import { useQuasar, type QTableColumn } from "quasar";
import { listPlugins, togglePluginEnabled, updatePluginConfig } from "../features/plugins/api";
import type { Plugin } from "../features/plugins/types";

const $q = useQuasar();
const rows = ref<Plugin[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
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

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listPlugins({
      search: search.value,
      category: category.value,
      enabled: enabled.value,
      callback_point: callbackPoint.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Plugin 失败";
  } finally {
    loading.value = false;
  }
}

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
  detailOpen.value = true;
}

function openConfig(plugin: Plugin) {
  configTarget.value = plugin;
  configText.value = prettyJSON(plugin.config_json || plugin.default_config_json || "{}");
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
  page.value = 1;
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.plugins-page
  padding: 24px

.plugins-hero
  display: flex
  align-items: flex-start
  justify-content: space-between
  gap: 16px
  margin-bottom: 18px

.plugins-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.plugins-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.plugins-subtitle
  margin: 0
  color: var(--q-grey-7)

.plugin-filter-card,
.plugin-table-card
  border-radius: 22px

.plugin-description
  color: var(--q-grey-8)
  line-height: 1.45

.plugin-detail-card
  width: 760px
  max-width: 94vw

.plugin-json-preview
  white-space: pre-wrap
  background: rgba(0, 0, 0, .04)
  border-radius: 12px
  padding: 12px
  max-height: 180px
  overflow: auto

@media (max-width: 720px)
  .plugins-hero
    flex-direction: column
    align-items: stretch
</style>
