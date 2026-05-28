import { reactive, ref } from "vue";
import { useQuasar } from "quasar";
import {
  editorTabForJsonKey,
  firstInvalidToolJsonKey,
  riskLevelOptions,
  toolEditorJsonKeys,
  validateToolJsonFields
} from "../../components/tools/toolUi";
import { TOOL_CREATE_TEMPLATES } from "./toolEditorCopy";
import type { Tool, ToolUpsertInput } from "./types";
import { useToolsStore } from "../../stores/tools";

export type UseToolEditorOptions = {
  onSaved: () => void | Promise<void>;
  /** After creating external tool — open detail for online test. */
  onCreated?: (tool: Tool) => void | Promise<void>;
};

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
export function useToolEditor(options: UseToolEditorOptions | (() => void | Promise<void>)) {
  const opts: UseToolEditorOptions = typeof options === "function" ? { onSaved: options } : options;
  const onSaved = opts.onSaved;
  const onCreated = opts.onCreated;
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const open = ref(false);
  const editingId = ref("");
  const saving = ref(false);
  const activeTab = ref("basic");
  const jsonErrors = reactive<Record<string, string>>({});
  const form = reactive<ToolUpsertInput>(blankToolForm());
  const riskOptions = riskLevelOptions;
  const selectedTemplate = ref("blank");

  function assignForm(input: ToolUpsertInput) {
    Object.assign(form, input);
    Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
  }

  function applyTemplate(templateId: string) {
    selectedTemplate.value = templateId;
    const tpl = TOOL_CREATE_TEMPLATES.find((t) => t.id === templateId);
    if (!tpl?.apply) return;
    assignForm({ ...blankToolForm(), ...tpl.apply });
  }

  function openCreate() {
    editingId.value = "";
    selectedTemplate.value = "blank";
    activeTab.value = "basic";
    assignForm(blankToolForm());
    open.value = true;
  }

  function openEdit(tool: Tool) {
    editingId.value = tool.id;
    selectedTemplate.value = "blank";
    activeTab.value = "basic";
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
    if (!validateJSONFields()) {
      const badKey = firstInvalidToolJsonKey(jsonErrors);
      if (badKey) {
        activeTab.value = editorTabForJsonKey(badKey);
        $q.notify({ type: "negative", message: `JSON 格式错误（${badKey}）` });
      }
      return;
    }
    saving.value = true;
    try {
      if (editingId.value) {
        await toolsStore.editTool(editingId.value, { ...form });
        open.value = false;
        $q.notify({ type: "positive", message: "Tool 已保存" });
        await onSaved();
      } else {
        const created = await toolsStore.addTool({ ...form });
        open.value = false;
        await onSaved();
        $q.dialog({
          title: "Tool 已创建",
          message: `「${created.display_name || created.key}」已注册。建议打开详情 → 在线测试，确认 Schema 与配置可用。`,
          cancel: { label: "稍后", flat: true, noCaps: true },
          ok: { label: "打开详情", noCaps: true, unelevated: true, class: "app-registry-primary-btn" },
          persistent: false
        }).onOk(async () => {
          if (onCreated) await onCreated(created);
        });
      }
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
    activeTab,
    jsonErrors,
    form,
    riskOptions,
    selectedTemplate,
    applyTemplate,
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
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "操作失败" });
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
      } catch (err) {
        $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
      } finally {
        busyId.value = "";
      }
    });
  }

  return { busyId, toggleEnabled, removeTool };
}
