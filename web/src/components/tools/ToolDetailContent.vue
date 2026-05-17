<template>
  <div v-if="tool" class="tool-detail-content column q-gutter-md">
    <q-banner rounded class="tool-detail-banner">{{ tool.description || "暂无描述" }}</q-banner>

    <q-tabs v-model="activeTab" dense class="tool-detail-tabs" active-color="primary" indicator-color="primary" align="left" no-caps>
      <q-tab name="overview" label="概览" />
      <q-tab name="agents" :label="'Agent (' + overrides.length + ')'" />
      <q-tab name="runs" label="调用记录" />
    </q-tabs>

    <q-tab-panels v-model="activeTab" class="tool-detail-panels">
      <q-tab-panel name="overview" class="q-pa-none">
        <div class="row q-col-gutter-sm text-body2 q-mb-md">
          <div class="col-6">
            <span class="text-weight-medium">分类：</span>{{ tool.category }}
          </div>
          <div class="col-6">
            <span class="text-weight-medium">来源：</span>{{ tool.source }}
          </div>
          <div class="col-6">
            <span class="text-weight-medium">风险：</span>{{ riskLabel(tool.risk_level) }}
          </div>
          <div class="col-6">
            <span class="text-weight-medium">运行时：</span>{{ runtimeStatusLabel(tool.runtime_status) }}
          </div>
          <div class="col-6">
            <span class="text-weight-medium">调用次数：</span>{{ tool.invoke_count }}
          </div>
          <div class="col-6">
            <span class="text-weight-medium">成功 / 失败：</span>{{ tool.success_count }} / {{ tool.failure_count }}
          </div>
        </div>
        <q-expansion-item dense-toggle default-open label="参数 Schema">
          <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.parameters_schema_json)" />
        </q-expansion-item>
        <q-expansion-item dense-toggle label="返回 Schema">
          <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.result_schema_json)" />
        </q-expansion-item>
        <q-expansion-item dense-toggle label="配置 JSON">
          <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.config_json)" />
        </q-expansion-item>
        <q-expansion-item dense-toggle label="默认配置">
          <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.default_config_json)" />
        </q-expansion-item>
        <q-expansion-item dense-toggle label="元数据">
          <tool-json-block class="q-mt-sm" :text="prettyJSON(tool.metadata_json)" />
        </q-expansion-item>
      </q-tab-panel>

      <q-tab-panel name="agents" class="q-pa-none">
        <div v-if="overridesLoading" class="text-center q-pa-md">
          <q-spinner-dots size="28px" />
        </div>
        <template v-else>
          <q-list v-if="overrides.length" separator dense class="rounded-borders">
            <q-item v-for="o in overrides" :key="o.id" class="override-item">
              <q-item-section avatar>
                <q-icon :name="o.enabled ? 'check_circle' : 'cancel'" :color="o.enabled ? 'positive' : 'negative'" size="sm" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ o.agent_id }}</q-item-label>
                <q-item-label caption>模式：{{ modeLabel(o.mode) }}<span v-if="o.requires_confirmation"> · 需确认</span></q-item-label>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn flat dense round icon="edit" size="sm" class="tool-icon-btn" @click="openOverrideEditor(o)">
                    <q-tooltip>编辑覆盖</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="delete" size="sm" class="tool-icon-btn" @click="removeOverride(o)">
                    <q-tooltip>删除覆盖</q-tooltip>
                  </q-btn>
                </div>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-caption q-pa-sm">暂无 Agent 覆盖配置</div>
          <q-btn flat no-caps icon="add" label="添加覆盖" class="tool-accent-btn q-mt-sm" @click="openOverrideEditor(null)" />
        </template>
      </q-tab-panel>

      <q-tab-panel name="runs" class="q-pa-none">
        <div v-if="runsLoading" class="text-center q-pa-md">
          <q-spinner-dots size="28px" />
        </div>
        <template v-else>
          <q-list v-if="recentRuns.length" separator dense class="rounded-borders">
            <q-item v-for="r in recentRuns" :key="r.id" class="run-item">
              <q-item-section avatar>
                <q-icon :name="runStatusIcon(r.status)" :color="runStatusColor(r.status)" size="sm" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ r.agent_display_name || r.agent_id }}</q-item-label>
                <q-item-label caption>{{ r.started_at }} · {{ r.duration_ms }}ms</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge :color="runStatusColor(r.status)" :label="r.status" />
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-caption q-pa-sm">暂无调用记录</div>
          <q-btn
            flat
            no-caps
            class="tool-accent-btn q-mt-sm"
            icon="history"
            label="查看全部调用记录"
            :to="{ name: 'tool-runs', query: { tool_key: tool.key } }"
            v-close-popup
          />
        </template>
      </q-tab-panel>
    </q-tab-panels>

    <q-dialog v-model="overrideEditorOpen" persistent>
      <q-card class="tool-dialog-card" style="max-width: 480px; width: 90vw">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingOverride ? '编辑 Agent 覆盖' : '添加 Agent 覆盖' }}</div>
          <q-btn flat dense round icon="close" class="tool-icon-btn" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-sm">
          <q-input v-model="overrideForm.agent_id" label="Agent ID" dense outlined :disable="!!editingOverride" />
          <q-select v-model="overrideForm.mode" label="模式" dense outlined :options="modeOptions" emit-value map-options />
          <q-toggle v-model="overrideForm.enabled" label="启用" />
          <q-toggle v-model="overrideForm.requires_confirmation" label="需要确认" />
          <q-input v-model="overrideForm.config_override_json" label="配置覆盖 JSON" type="textarea" dense outlined autogrow />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn no-caps unelevated class="tool-primary-btn" label="保存" :loading="overrideSaving" @click="saveOverride" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { Tool, ToolAgentOverride, ToolInvocation } from "../../features/tools/types";
import ToolJsonBlock from "./ToolJsonBlock.vue";
import { prettyJSON, riskLabel, runtimeStatusLabel } from "./toolUi";
import { useToolsStore } from "../../stores/tools/index";

const toolsStore = useToolsStore();

const props = defineProps<{
  tool: Tool | null;
}>();

const $q = useQuasar();
const activeTab = ref("overview");

const overrides = ref<ToolAgentOverride[]>([]);
const overridesLoading = ref(false);

const recentRuns = ref<ToolInvocation[]>([]);
const runsLoading = ref(false);

const overrideEditorOpen = ref(false);
const editingOverride = ref<ToolAgentOverride | null>(null);
const overrideSaving = ref(false);
const overrideForm = ref({
  agent_id: "",
  mode: "inherit",
  enabled: true,
  requires_confirmation: false,
  config_override_json: "{}"
});

const modeOptions = [
  { label: "继承 (inherit)", value: "inherit" },
  { label: "允许 (allow)", value: "allow" },
  { label: "拒绝 (deny)", value: "deny" }
];

function modeLabel(mode: string): string {
  const m = modeOptions.find((o) => o.value === mode);
  return m ? m.label : mode;
}

function runStatusIcon(status: string): string {
  if (status === "success") return "check_circle";
  if (status === "error") return "error";
  if (status === "blocked") return "block";
  return "help";
}

function runStatusColor(status: string): string {
  if (status === "success") return "positive";
  if (status === "error") return "negative";
  if (status === "blocked") return "warning";
  return "grey";
}

async function loadOverrides() {
  if (!props.tool) return;
  overridesLoading.value = true;
  try {
    overrides.value = await toolsStore.fetchOverrides(props.tool.id || props.tool.key);
  } catch {
    overrides.value = [];
  } finally {
    overridesLoading.value = false;
  }
}

async function loadRecentRuns() {
  if (!props.tool) return;
  runsLoading.value = true;
  try {
    const res = await toolsStore.fetchToolRuns(props.tool.id || props.tool.key, { page: 1, page_size: 20 });
    recentRuns.value = res.items;
  } catch {
    recentRuns.value = [];
  } finally {
    runsLoading.value = false;
  }
}

function openOverrideEditor(o: ToolAgentOverride | null) {
  editingOverride.value = o;
  if (o) {
    overrideForm.value = {
      agent_id: o.agent_id,
      mode: o.mode,
      enabled: o.enabled,
      requires_confirmation: o.requires_confirmation,
      config_override_json: o.config_override_json
    };
  } else {
    overrideForm.value = { agent_id: "", mode: "inherit", enabled: true, requires_confirmation: false, config_override_json: "{}" };
  }
  overrideEditorOpen.value = true;
}

async function saveOverride() {
  if (!props.tool) return;
  overrideSaving.value = true;
  try {
    await toolsStore.saveOverride({
      tool_id: props.tool.id || props.tool.key,
      agent_id: overrideForm.value.agent_id,
      enabled: overrideForm.value.enabled,
      mode: overrideForm.value.mode,
      config_override_json: overrideForm.value.config_override_json,
      requires_confirmation: overrideForm.value.requires_confirmation
    });
    overrideEditorOpen.value = false;
    await loadOverrides();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存覆盖失败" });
  } finally {
    overrideSaving.value = false;
  }
}

function removeOverride(o: ToolAgentOverride) {
  if (!props.tool) return;
  $q.dialog({ title: "删除覆盖", message: `确认删除 Agent ${o.agent_id} 的覆盖？`, cancel: true, persistent: true }).onOk(async () => {
    try {
      await toolsStore.removeOverride(props.tool!.id || props.tool!.key, o.agent_id);
      await loadOverrides();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除覆盖失败" });
    }
  });
}

watch(
  () => props.tool,
  (t) => {
    if (t) {
      loadOverrides();
      loadRecentRuns();
    }
  }
);

onMounted(() => {
  if (props.tool) {
    loadOverrides();
    loadRecentRuns();
  }
});
</script>

<style scoped lang="sass">
.tool-detail-banner
  color: var(--color-text-primary)
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

body.body--dark .tool-detail-banner
  background: rgba(18, 24, 34, 0.55)

.tool-detail-tabs
  border-bottom: 1px solid var(--glass-border)

.tool-detail-panels
  background: transparent

.tool-accent-btn
  color: var(--color-accent)
  align-self: flex-start

body:not(.body--dark) .tool-accent-btn:hover
  background: var(--interaction-surface-hover)

.tool-icon-btn
  color: var(--color-icon-muted)

body:not(.body--dark) .tool-icon-btn:hover
  color: var(--color-accent)

.tool-primary-btn
  background: var(--color-accent)
  color: #fff

.override-item, .run-item
  border-radius: 8px

.tool-dialog-card
  border-radius: 22px
  border: 1px solid var(--glass-border)
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
</style>
