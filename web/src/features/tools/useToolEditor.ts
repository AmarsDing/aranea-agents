import { reactive, ref } from "vue";
import { useQuasar } from "quasar";
import {
  riskLevelOptions,
  toolEditorJsonKeys,
  validateToolJsonFields
} from "../../components/tools/toolUi";
import type { Tool, ToolUpsertInput } from "./types";
import { useToolsStore } from "../../stores/tools";

export function blankToolForm(): ToolUpsertInput {
  return {
    key: "",
    display_name: "",
    description: "",
    category: "custom",
    source: "external",
    risk_level: "low",
    enabled: true,
    readonly: false,
    requires_confirmation: false,
    supports_streaming: false,
    supports_concurrency: false,
    parameters_schema_json: "{}",
    result_schema_json: "{}",
    config_schema_json: "{}",
    config_json: "{}",
    default_config_json: "{}",
    metadata_json: "{}"
  };
}

/** Page-level create/edit dialog orchestration for Tools catalog. */
export function useToolEditor(onSaved: () => void | Promise<void>) {
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const open = ref(false);
  const editingId = ref("");
  const saving = ref(false);
  const jsonErrors = reactive<Record<string, string>>({});
  const form = reactive<ToolUpsertInput>(blankToolForm());
  const riskOptions = riskLevelOptions;

  function assignForm(input: ToolUpsertInput) {
    Object.assign(form, input);
    Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
  }

  function openCreate() {
    editingId.value = "";
    assignForm(blankToolForm());
    open.value = true;
  }

  function openEdit(tool: Tool) {
    editingId.value = tool.id;
    assignForm({
      key: tool.key,
      display_name: tool.display_name,
      description: tool.description,
      category: tool.category,
      source: tool.source,
      risk_level: tool.risk_level,
      enabled: tool.enabled,
      readonly: tool.readonly,
      requires_confirmation: tool.requires_confirmation,
      supports_streaming: tool.supports_streaming,
      supports_concurrency: tool.supports_concurrency,
      parameters_schema_json: tool.parameters_schema_json || "{}",
      result_schema_json: tool.result_schema_json || "{}",
      config_schema_json: tool.config_schema_json || "{}",
      config_json: tool.config_json || "{}",
      default_config_json: tool.default_config_json || "{}",
      metadata_json: tool.metadata_json || "{}"
    });
    open.value = true;
  }

  function validateJSONFields() {
    Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
    const keys = [...toolEditorJsonKeys];
    const fieldObj = keys.reduce(
      (acc, k) => {
        acc[k] = form[k];
        return acc;
      },
      {} as Record<string, string>
    );
    Object.assign(jsonErrors, validateToolJsonFields(fieldObj, keys));
    return Object.keys(jsonErrors).length === 0;
  }

  async function save() {
    if (!validateJSONFields()) return;
    saving.value = true;
    try {
      if (editingId.value) {
        await toolsStore.editTool(editingId.value, { ...form });
      } else {
        await toolsStore.addTool({ ...form });
      }
      open.value = false;
      await onSaved();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存 Tool 失败" });
    } finally {
      saving.value = false;
    }
  }

  return {
    open,
    editingId,
    saving,
    jsonErrors,
    form,
    riskOptions,
    openCreate,
    openEdit,
    save
  };
}

/** High-risk enable confirmation flow for Tools table toggle. */
export function useToolToggle(onChanged: () => void | Promise<void>) {
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const busyId = ref("");

  async function toggleEnabled(tool: Tool, value: boolean) {
    if (value && (tool.risk_level === "high" || tool.risk_level === "critical")) {
      $q.dialog({
        title: "高风险工具确认",
        message: `即将启用高风险工具「${tool.display_name}」（风险等级：${tool.risk_level}）。请输入工具 Key 以确认：${tool.key}`,
        prompt: { model: "", type: "text", label: "请输入 Tool Key" },
        cancel: true,
        persistent: true
      }).onOk(async (inputKey: string) => {
        if (inputKey !== tool.key) {
          $q.notify({ type: "negative", message: "输入的 Key 不匹配，操作已取消" });
          return;
        }
        busyId.value = tool.id;
        try {
          await toolsStore.toggle(tool.id || tool.key, value, tool.key);
          await onChanged();
        } catch (err) {
          $q.notify({ type: "negative", message: err instanceof Error ? err.message : "操作失败" });
        } finally {
          busyId.value = "";
        }
      });
      return;
    }
    busyId.value = tool.id;
    try {
      await toolsStore.toggle(tool.id || tool.key, value);
      await onChanged();
    } finally {
      busyId.value = "";
    }
  }

  function removeTool(tool: Tool) {
    $q.dialog({
      title: "删除 Tool",
      message: `确认删除 ${tool.display_name}（${tool.key}）？`,
      cancel: true,
      persistent: true
    }).onOk(async () => {
      busyId.value = tool.id;
      try {
        await toolsStore.remove(tool.id || tool.key);
        await onChanged();
      } finally {
        busyId.value = "";
      }
    });
  }

  return { busyId, toggleEnabled, removeTool };
}
