import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';
import {
  editorTabForJsonKey,
  firstInvalidToolJsonKey,
  riskLevelOptions,
  toolEditorJsonKeys,
  validateToolJsonFields,
} from '../../components/tools/toolUi';
import { TOOL_CREATE_TEMPLATES } from '../../features/tools/toolEditorCopy';
import type { Tool, ToolUpsertInput } from '../../features/tools/types';
import { i18n } from '../../i18n';
import { useToolsStore } from './index';

export function blankToolForm(): ToolUpsertInput {
  return {
    key: '',
    display_name: '',
    description: '',
    category: 'custom',
    source: 'external',
    risk_level: 'low',
    enabled: true,
    readonly: false,
    requires_confirmation: false,
    supports_streaming: false,
    supports_concurrency: false,
    parameters_schema_json: '{}',
    result_schema_json: '{}',
    config_schema_json: '{}',
    config_json: '{}',
    default_config_json: '{}',
    metadata_json: '{}',
  };
}

export const useToolEditorStore = defineStore('toolEditor', () => {
  const toolsStore = useToolsStore();

  const open = ref(false);
  const editingId = ref('');
  const saving = ref(false);
  const activeTab = ref('basic');
  const jsonErrors = reactive<Record<string, string>>({});
  const form = reactive<ToolUpsertInput>(blankToolForm());
  const originalForm = ref<ToolUpsertInput>(blankToolForm());
  const riskOptions = riskLevelOptions;
  const selectedTemplate = ref('blank');

  const dirty = computed(() => {
    const keys = Object.keys(originalForm.value) as (keyof ToolUpsertInput)[];
    return keys.some((k) => form[k] !== originalForm.value[k]);
  });

  function assignForm(input: ToolUpsertInput) {
    Object.assign(form, input);
    Object.assign(originalForm.value, input);
    Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
  }

  function applyTemplate(templateId: string) {
    selectedTemplate.value = templateId;
    const tpl = TOOL_CREATE_TEMPLATES.find((t) => t.id === templateId);
    if (!tpl?.apply) return;
    assignForm({ ...blankToolForm(), ...tpl.apply });
  }

  function openCreate() {
    editingId.value = '';
    selectedTemplate.value = 'blank';
    activeTab.value = 'basic';
    assignForm(blankToolForm());
    open.value = true;
  }

  function openEdit(tool: Tool) {
    editingId.value = tool.id;
    selectedTemplate.value = 'blank';
    activeTab.value = 'basic';
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
      parameters_schema_json: tool.parameters_schema_json || '{}',
      result_schema_json: tool.result_schema_json || '{}',
      config_schema_json: tool.config_schema_json || '{}',
      config_json: tool.config_json || '{}',
      default_config_json: tool.default_config_json || '{}',
      metadata_json: tool.metadata_json || '{}',
    });
    open.value = true;
  }

  function closeEditor() {
    open.value = false;
  }

  function validateJSONFields() {
    Object.keys(jsonErrors).forEach((key) => delete jsonErrors[key]);
    const keys = [...toolEditorJsonKeys];
    const fieldObj = keys.reduce(
      (acc, k) => {
        acc[k] = form[k];
        return acc;
      },
      {} as Record<string, string>,
    );
    Object.assign(jsonErrors, validateToolJsonFields(fieldObj, keys));
    return Object.keys(jsonErrors).length === 0;
  }

  /**
   * 保存（纯数据流，不含 UI 反馈——notify/dialog 由 Page 层编排，红线 #4）。
   * 校验失败或 API 失败时抛出带用户可读信息的 Error；成功返回 { created }（编辑时 created 为 null）。
   */
  async function save(): Promise<{ created: Tool | null }> {
    const t = i18n.global.t;
    // 客户端必填校验：与后端 validateToolUpsert 口径一致，避免整表提交后才收到原始英文错误。
    const missingMsg = !form.key.trim()
      ? t('toolsPage.editor.requiredKey')
      : !form.display_name.trim()
        ? t('toolsPage.editor.requiredName')
        : '';
    if (missingMsg) {
      activeTab.value = 'basic';
      throw new Error(missingMsg);
    }
    if (!validateJSONFields()) {
      const badKey = firstInvalidToolJsonKey(jsonErrors);
      if (badKey) {
        activeTab.value = editorTabForJsonKey(badKey);
        throw new Error(t('toolsPage.editor.invalidJsonField', { field: badKey }));
      }
      throw new Error(t('toolsPage.editor.invalidJson'));
    }
    saving.value = true;
    try {
      if (editingId.value) {
        await toolsStore.editTool(editingId.value, { ...form });
        open.value = false;
        return { created: null };
      }
      const created = await toolsStore.addTool({ ...form });
      open.value = false;
      return { created };
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
    originalForm,
    dirty,
    riskOptions,
    selectedTemplate,
    applyTemplate,
    openCreate,
    openEdit,
    closeEditor,
    save,
  };
});
