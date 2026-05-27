<template>
  <div class="callback-editor">
    <section class="callback-editor__section">
      <div class="callback-editor__section-head">
        <span class="callback-editor__section-title">规则概要</span>
        <span class="hook-tag hook-tag--point">{{ localRule.callback_point }}</span>
        <span :class="actionTagClass(localRule.action.type)">{{ actionTypeLabel(localRule.action.type) }}</span>
      </div>
      <div class="app-form-field-grid app-form-field-grid--wide">
        <q-select
          v-model="localRule.callback_point"
          dense
          outlined
          emit-value
          map-options
          label="回调点"
          :options="pointOptions"
          @update:model-value="emitChange"
        />
        <q-select
          v-model="localRule.action.type"
          dense
          outlined
          emit-value
          map-options
          label="动作"
          :options="actionOptions"
          @update:model-value="emitChange"
        />
        <q-input
          v-model.number="sortOrder"
          dense
          outlined
          type="number"
          label="排序"
          hint="越小越先执行"
          @update:model-value="emitMeta"
        />
      </div>
    </section>

    <q-expansion-item dense-toggle default-opened icon="filter_alt" label="触发条件" header-class="callback-editor__expansion-head">
      <div class="callback-editor__panel app-form-field-grid">
        <q-input
          v-model="localRule.condition.agent_id"
          dense
          outlined
          clearable
          label="Agent ID / Key"
          hint="留空表示匹配全部 Agent"
          @update:model-value="emitChange"
        />
        <q-input
          v-model="localRule.condition.tool_name"
          dense
          outlined
          clearable
          label="Tool 名称"
          hint="仅 before_tool / after_tool 生效"
          :disable="!toolPoint"
          @update:model-value="emitChange"
        />
        <q-input
          v-model="localRule.condition.event_type"
          dense
          outlined
          clearable
          label="事件类型"
          hint="on_event：runner_completion / model_response"
          :disable="!onEventPoint"
          @update:model-value="emitChange"
        />
      </div>
    </q-expansion-item>

    <q-expansion-item dense-toggle default-opened icon="bolt" label="执行动作" header-class="callback-editor__expansion-head">
      <div class="callback-editor__panel q-gutter-md">
        <template v-if="showNotifyFields">
          <q-input
            v-model="localRule.action.webhook_url"
            class="app-grid-span-full"
            dense
            outlined
            label="Webhook URL"
            @update:model-value="emitChange"
          />
          <div class="app-form-field-grid app-form-field-grid--2col app-grid-span-full">
            <q-input
              v-model.number="localRule.action.notify_max_retries"
              dense
              outlined
              type="number"
              min="1"
              max="10"
              label="最大重试"
              hint="默认 3"
              @update:model-value="emitChange"
            />
            <q-input
              v-model.number="localRule.action.notify_timeout_sec"
              dense
              outlined
              type="number"
              min="1"
              max="60"
              label="超时 (秒)"
              hint="默认 8"
              @update:model-value="emitChange"
            />
          </div>
        </template>

        <q-select
          v-if="showLogFields"
          v-model="localRule.action.log_level"
          dense
          outlined
          emit-value
          map-options
          label="日志级别"
          :options="LOG_LEVEL_OPTIONS"
          @update:model-value="emitChange"
        />

        <q-input
          v-if="showMessageField"
          v-model="localRule.action.message"
          class="app-grid-span-full"
          dense
          outlined
          type="textarea"
          autogrow
          label="消息"
          @update:model-value="emitChange"
        />

        <template v-if="showModifyFields">
          <p class="callback-editor__hint app-grid-span-full">{{ MODIFY_PATCH_HINT }}</p>
          <q-input
            v-model="modifyPatchText"
            class="app-grid-span-full"
            dense
            outlined
            type="textarea"
            rows="8"
            label="modify_patch (JSON)"
            :error="Boolean(modifyPatchError)"
            :error-message="modifyPatchError"
            @update:model-value="onModifyPatchInput"
          />
        </template>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import {
  actionTagClass,
  actionTypeLabel,
  LOG_LEVEL_OPTIONS,
  MODIFY_PATCH_HINT
} from "./callbackEditorUi";
import { useCallbackEditor } from "./useCallbackEditor";
import type { HookRuleConfig } from "../../features/hooks/types";

const props = defineProps<{
  modelValue: HookRuleConfig;
  sortOrder?: number;
  agentId?: string;
  agentKey?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [HookRuleConfig];
  "update:sortOrder": [number];
}>();

const {
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
  onModifyPatchInput
} = useCallbackEditor(props, emit);
</script>
