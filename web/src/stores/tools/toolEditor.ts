import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';
import { useQuasar } from 'quasar';
import {
  editorTabForJsonKey,
  firstInvalidToolJsonKey,
  riskLevelOptions,
  toolEditorJsonKeys,
  validateToolJsonFields,
} from '../../components/tools/toolUi';
import { TOOL_CREATE_TEMPLATES } from '../../features/tools/toolEditorCopy';
import type { Tool, ToolUpsertInput } from '../../features/tools/types';
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
  const $q = useQuasar();
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

  const onSavedCallback = ref<(() => void | Promise<void>) | null>(null);
  const onCreatedCallback = ref<((tool: Tool) => void | Promise<void>) | null>(null);

  const dirty = computed(() => {
    const keys = Object.keys(originalForm.value) as (keyof ToolUpsertInput)[];
    return keys.some((k) => form[k] !== originalForm.value[k]);
  });

  function setCallbacks(opts: {
    onSaved?: () => void | Promise<void>;
    onCreated?: (tool: Tool) => void | Promise<void>;
  }) {
    if (opts.onSaved) onSavedCallback.value = opts.onSaved;
    if (opts.onCreated) onCreatedCallback.value = opts.onCreated;
  }

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

  async function save() {
    if (!validateJSONFields()) {
      const badKey = firstInvalidToolJsonKey(jsonErrors);
      if (badKey) {
        activeTab.value = editorTabForJsonKey(badKey);
        $q.notify({ type: 'negative', message: `JSON 格式错误（${badKey}）` });
      }
      return;
    }
    saving.value = true;
    try {
      if (editingId.value) {
        await toolsStore.editTool(editingId.value, { ...form });
        open.value = false;
        $q.notify({ type: 'positive', message: 'Tool 已保存' });
        if (onSavedCallback.value) await onSavedCallback.value();
      } else {
        const created = await toolsStore.addTool({ ...form });
        open.value = false;
        if (onSavedCallback.value) await onSavedCallback.value();
        $q.dialog({
          title: 'Tool 已创建',
          message: `「${created.display_name || created.key}」已注册。建议打开详情 → 在线测试，确认 Schema 与配置可用。`,
          cancel: { label: '稍后', flat: true, noCaps: true },
          ok: { label: '打开详情', noCaps: true, unelevated: true, class: 'app-registry-primary-btn' },
          persistent: false,
        }).onOk(async () => {
          if (onCreatedCallback.value) await onCreatedCallback.value(created);
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存 Tool 失败' });
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
    dirty,
    riskOptions,
    selectedTemplate,
    setCallbacks,
    applyTemplate,
    openCreate,
    openEdit,
    closeEditor,
    save,
  };
});
