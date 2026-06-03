import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { cloneHookRuleConfig, defaultHookRuleConfig, type HookRuleConfig } from '../../features/hooks/types';
import { isOnEventPoint, isToolCallbackPoint, parseModifyPatchText, stringifyModifyPatch } from './callbackEditorUi';
import { useCallbackPointOptions } from '../../features/callback/constants';

type CallbackEditorProps = {
  modelValue: HookRuleConfig;
  sortOrder?: number;
  agentId?: string;
  agentKey?: string;
};

export function useCallbackEditor(
  props: Readonly<CallbackEditorProps>,
  emit: {
    (event: 'update:modelValue', value: HookRuleConfig): void;
    (event: 'update:sortOrder', value: number): void;
  },
) {
  const { t } = useI18n();
  const localRule = ref<HookRuleConfig>(defaultHookRuleConfig(props.agentId, props.agentKey));
  const sortOrder = ref(props.sortOrder ?? 0);
  const modifyPatchText = ref('{}');
  const modifyPatchError = ref('');

  const pointOptions = useCallbackPointOptions();
  const actionOptions = computed(() => [
    { label: t('hooksPage.actionTypes.log'), value: 'log' as const },
    { label: t('hooksPage.actionTypes.notify'), value: 'notify' as const },
    { label: t('hooksPage.actionTypes.block'), value: 'block' as const },
    { label: t('hooksPage.actionTypes.modify'), value: 'modify' as const },
  ]);

  const toolPoint = computed(() => isToolCallbackPoint(localRule.value.callback_point));
  const onEventPoint = computed(() => isOnEventPoint(localRule.value.callback_point));
  const showNotifyFields = computed(() => localRule.value.action.type === 'notify');
  const showLogFields = computed(() => localRule.value.action.type === 'log');
  const showModifyFields = computed(() => localRule.value.action.type === 'modify');
  const showMessageField = computed(() => localRule.value.action.type === 'block');

  watch(
    () => props.modelValue,
    (value) => {
      localRule.value = value ? cloneHookRuleConfig(value) : defaultHookRuleConfig(props.agentId, props.agentKey);
      syncModifyText();
    },
    { immediate: true, deep: true },
  );

  watch(
    () => props.sortOrder,
    (value) => {
      sortOrder.value = value ?? 0;
    },
    { immediate: true },
  );

  watch(
    () => localRule.value.callback_point,
    (point, prev) => {
      if (point === prev) return;
      if (!isToolCallbackPoint(point)) {
        localRule.value.condition.tool_name = '';
      }
      if (!isOnEventPoint(point)) {
        localRule.value.condition.event_type = '';
      }
      emitChange();
    },
  );

  watch(
    () => localRule.value.action.type,
    (type, prev) => {
      if (type === prev) return;
      if (type === 'modify') {
        syncModifyText();
      } else {
        modifyPatchError.value = '';
      }
    },
  );

  function syncModifyText() {
    modifyPatchError.value = '';
    modifyPatchText.value = stringifyModifyPatch(localRule.value.action.modify_patch);
  }

  function emitChange() {
    emit('update:modelValue', cloneHookRuleConfig(localRule.value));
  }

  function emitMeta() {
    emit('update:sortOrder', Number(sortOrder.value) || 0);
  }

  function onModifyPatchInput(raw: string | number | null) {
    const { patch, error } = parseModifyPatchText(raw, t);
    modifyPatchError.value = error;
    if (error) return;
    localRule.value.action.modify_patch = patch;
    emitChange();
  }

  return {
    localRule,
    sortOrder,
    modifyPatchText,
    modifyPatchError,
    pointOptions,
    actionOptions,
    toolPoint,
    onEventPoint,
    showNotifyFields,
    showLogFields,
    showModifyFields,
    showMessageField,
    emitChange,
    emitMeta,
    onModifyPatchInput,
  };
}
