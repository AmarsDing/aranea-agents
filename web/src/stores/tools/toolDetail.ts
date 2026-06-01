import { defineStore } from "pinia";
import { ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useToolsStore } from "./index";
import {
  bindingSummaryLine,
  fetchToolAgentBindingSummary,
  type ToolAgentBindingSummary
} from "../../features/tools/toolAgentBindingSummary";
import { toolToUpsertInput } from "../../features/tools/toolFormPatch";
import type { Tool, ToolAgentOverride, ToolInvocation, ToolTestResult } from "../../features/tools/types";
import { listAgents } from "../../features/agents/api";
import type { Agent } from "../../features/agents/types";

export type ToolOverrideForm = {
  agent_id: string;
  mode: string;
  enabled: boolean;
  requires_confirmation: boolean;
  config_override_json: string;
};

export const useToolDetailStore = defineStore("toolDetail", () => {
  const $q = useQuasar();
  const toolsStore = useToolsStore();

  const open = ref(false);
  const tool = ref<Tool | null>(null);
  const activeTab = ref("overview");
  const loading = ref(false);

  const overrides = ref<ToolAgentOverride[]>([]);
  const overridesLoading = ref(false);
  const recentRuns = ref<ToolInvocation[]>([]);
  const runsLoading = ref(false);
  const testArgsJson = ref("{}");
  const testTimeoutSec = ref(30);
  const testRunning = ref(false);
  const testResult = ref<ToolTestResult | null>(null);
  const overrideEditorOpen = ref(false);
  const editingOverride = ref<ToolAgentOverride | null>(null);
  const overrideSaving = ref(false);
  const overrideForm = ref<ToolOverrideForm>({
    agent_id: "",
    mode: "inherit",
    enabled: true,
    requires_confirmation: false,
    config_override_json: "{}"
  });
  const configJson = ref("{}");
  const configSaving = ref(false);
  const agentBindingSummary = ref<ToolAgentBindingSummary | null>(null);
  const agentBindingLoading = ref(false);
  const agentOptions = ref<{ label: string; value: string }[]>([]);
  const agentsLoading = ref(false);

  function toolKey(): string {
    return tool.value?.key?.trim() || tool.value?.id?.trim() || "";
  }

  function syncConfigFromTool() {
    configJson.value = tool.value?.config_json || "{}";
  }

  async function loadOverrides() {
    const key = toolKey();
    if (!key) return;
    overridesLoading.value = true;
    try {
      overrides.value = await toolsStore.fetchOverrides(key);
    } catch {
      overrides.value = [];
    } finally {
      overridesLoading.value = false;
    }
  }

  async function loadRecentRuns() {
    const key = toolKey();
    if (!key) return;
    runsLoading.value = true;
    try {
      const res = await toolsStore.fetchToolRuns(key, { page: 1, page_size: 20 });
      recentRuns.value = res.items;
    } catch {
      recentRuns.value = [];
    } finally {
      runsLoading.value = false;
    }
  }

  async function loadAgentBindingSummary() {
    const key = toolKey();
    if (!key) {
      agentBindingSummary.value = null;
      return;
    }
    agentBindingLoading.value = true;
    try {
      agentBindingSummary.value = await fetchToolAgentBindingSummary(key, overrides.value.length);
    } catch {
      agentBindingSummary.value = null;
    } finally {
      agentBindingLoading.value = false;
    }
  }

  async function refreshDetail() {
    loading.value = true;
    testResult.value = null;
    testArgsJson.value = "{}";
    syncConfigFromTool();
    try {
      await loadOverrides();
      await Promise.all([loadRecentRuns(), loadAgentBindingSummary()]);
    } finally {
      loading.value = false;
    }
  }

  async function openDetail(t: Tool) {
    tool.value = t;
    open.value = true;
    activeTab.value = "overview";
  }

  function closeDetail() {
    open.value = false;
  }

  async function runToolTest() {
    const t = tool.value;
    if (!t?.id) {
      $q.notify({ type: "warning", message: "工具缺少 ID，无法执行测试" });
      return;
    }
    testRunning.value = true;
    testResult.value = null;
    try {
      testResult.value = await toolsStore.runToolTest(t.id, testArgsJson.value, testTimeoutSec.value);
      if (testResult.value.status === "success") {
        $q.notify({ type: "positive", message: "工具测试成功" });
      }
      await loadRecentRuns();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "工具测试失败" });
    } finally {
      testRunning.value = false;
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
      overrideForm.value = {
        agent_id: "",
        mode: "inherit",
        enabled: true,
        requires_confirmation: false,
        config_override_json: "{}"
      };
    }
    overrideEditorOpen.value = true;
    loadAgentOptions();
  }

  async function loadAgentOptions() {
    if (agentOptions.value.length > 0) return;
    agentsLoading.value = true;
    try {
      const agents: Agent[] = await listAgents({ limit: 200 });
      agentOptions.value = agents.map((a) => ({
        label: a.display_name || a.agent_key || a.id,
        value: a.id
      }));
    } catch {
      agentOptions.value = [];
    } finally {
      agentsLoading.value = false;
    }
  }

  async function saveOverride() {
    const key = toolKey();
    if (!key) return;
    overrideSaving.value = true;
    try {
      await toolsStore.saveOverride({
        tool_id: key,
        agent_id: overrideForm.value.agent_id,
        enabled: overrideForm.value.enabled,
        mode: overrideForm.value.mode,
        config_override_json: overrideForm.value.config_override_json,
        requires_confirmation: overrideForm.value.requires_confirmation
      });
      overrideEditorOpen.value = false;
      await loadOverrides();
      await loadAgentBindingSummary();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存覆盖失败" });
    } finally {
      overrideSaving.value = false;
    }
  }

  function confirmRemoveOverride(o: ToolAgentOverride) {
    const key = toolKey();
    if (!key) return;
    $q.dialog({
      title: "删除覆盖",
      message: `确认删除 Agent ${o.agent_id} 的覆盖？`,
      cancel: true,
      persistent: true
    }).onOk(async () => {
      try {
        await toolsStore.removeOverride(key, o.agent_id);
        await loadOverrides();
        await loadAgentBindingSummary();
      } catch (err) {
        $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除覆盖失败" });
      }
    });
  }

  async function saveConfigSchema(schemaJson: string) {
    const t = tool.value;
    if (!t?.id) return;
    try {
      JSON.parse(schemaJson || "{}");
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? `Schema JSON 无效：${err.message}` : "Schema JSON 无效"
      });
      return;
    }
    configSaving.value = true;
    try {
      const updated = await toolsStore.editTool(t.id, toolToUpsertInput(t, { config_schema_json: schemaJson }));
      tool.value = updated;
      $q.notify({ type: "positive", message: "配置 Schema 已保存" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存 Schema 失败" });
    } finally {
      configSaving.value = false;
    }
  }

  async function saveConfig() {
    const t = tool.value;
    if (!t?.id) return;
    try {
      JSON.parse(configJson.value || "{}");
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? `配置 JSON 无效：${err.message}` : "配置 JSON 无效"
      });
      return;
    }
    configSaving.value = true;
    try {
      const updated = await toolsStore.editToolConfig(t.id, configJson.value);
      tool.value = updated;
      configJson.value = updated.config_json || configJson.value;
      $q.notify({ type: "positive", message: "配置已保存" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存配置失败" });
    } finally {
      configSaving.value = false;
    }
  }

  watch(tool, (t) => {
    if (t) void refreshDetail();
    else syncConfigFromTool();
  });

  return {
    open,
    tool,
    activeTab,
    loading,
    overrides,
    overridesLoading,
    recentRuns,
    runsLoading,
    testArgsJson,
    testTimeoutSec,
    testRunning,
    testResult,
    overrideEditorOpen,
    editingOverride,
    overrideSaving,
    overrideForm,
    configJson,
    configSaving,
    agentBindingSummary,
    agentBindingLoading,
    agentOptions,
    agentsLoading,
    openDetail,
    closeDetail,
    refreshDetail,
    runToolTest,
    saveConfig,
    saveConfigSchema,
    openOverrideEditor,
    saveOverride,
    confirmRemoveOverride,
    bindingSummaryLine
  };
});
