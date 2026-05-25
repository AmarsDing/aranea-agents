<template>
  <div class="callback-editor q-gutter-md">
    <div class="app-form-field-grid">
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
        @update:model-value="emitMeta"
      />
    </div>

    <q-expansion-item dense-toggle default-open label="触发条件">
      <div class="app-form-field-grid q-pa-sm">
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
          :disable="!toolPoint"
          @update:model-value="emitChange"
        />
        <q-input
          v-model="localRule.condition.event_type"
          dense
          outlined
          clearable
          label="事件类型"
          hint="on_event: runner_completion / model_response"
          :disable="localRule.callback_point !== 'on_event'"
          @update:model-value="emitChange"
        />
      </div>
    </q-expansion-item>

    <q-expansion-item dense-toggle default-open label="执行动作">
      <div class="q-pa-sm q-gutter-md">
        <template v-if="localRule.action.type === 'notify'">
          <q-input
            v-model="localRule.action.webhook_url"
            class="app-field-long"
            dense
            outlined
            label="Webhook URL"
            @update:model-value="emitChange"
          />
          <div class="app-form-field-grid app-form-field-grid--2col">
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
              label="超时(秒)"
              hint="默认 8"
              @update:model-value="emitChange"
            />
          </div>
        </template>
        <q-select
          v-if="localRule.action.type === 'log'"
          v-model="localRule.action.log_level"
          dense
          outlined
          emit-value
          map-options
          label="日志级别"
          :options="logLevelOptions"
          @update:model-value="emitChange"
        />
        <q-input
          v-if="localRule.action.type === 'block' || localRule.action.type === 'log'"
          v-model="localRule.action.message"
          dense
          outlined
          type="textarea"
          autogrow
          label="消息"
          @update:model-value="emitChange"
        />
        <template v-if="localRule.action.type === 'modify'">
          <div class="text-caption text-grey-7">
            before_model: generation_config / append_system / append_user。
            before_tool: <strong>arguments</strong> 整包替换；
            <strong>merge_arguments</strong> 深度合并（嵌套对象递归，标量/数组以 patch 为准）。
          </div>
          <q-input
            v-model="modifyPatchText"
            class="app-field-long"
            dense
            outlined
            type="textarea"
            rows="8"
            label="modify_patch (JSON)"
            :error="!!modifyPatchError"
            :error-message="modifyPatchError"
            @update:model-value="onModifyPatchInput"
          />
        </template>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  ACTION_TYPE_OPTIONS,
  CALLBACK_POINT_OPTIONS,
  cloneHookRuleConfig,
  defaultHookRuleConfig,
  type HookRuleConfig
} from "../../features/hooks/types";

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

const localRule = ref<HookRuleConfig>(defaultHookRuleConfig(props.agentId, props.agentKey));
const sortOrder = ref(props.sortOrder ?? 0);
const modifyPatchText = ref("{}");
const modifyPatchError = ref("");

const pointOptions = CALLBACK_POINT_OPTIONS;
const actionOptions = ACTION_TYPE_OPTIONS;
const logLevelOptions = [
  { label: "debug", value: "debug" },
  { label: "info", value: "info" },
  { label: "warn", value: "warn" },
  { label: "error", value: "error" }
];

const toolPoint = computed(
  () => localRule.value.callback_point === "before_tool" || localRule.value.callback_point === "after_tool"
);

watch(
  () => props.modelValue,
  (v) => {
    localRule.value = v
      ? cloneHookRuleConfig(v)
      : defaultHookRuleConfig(props.agentId, props.agentKey);
    syncModifyText();
  },
  { immediate: true, deep: true }
);

watch(
  () => props.sortOrder,
  (v) => {
    sortOrder.value = v ?? 0;
  },
  { immediate: true }
);

function syncModifyText() {
  modifyPatchError.value = "";
  modifyPatchText.value = JSON.stringify(localRule.value.action.modify_patch ?? {}, null, 2);
}

function emitChange() {
  emit("update:modelValue", cloneHookRuleConfig(localRule.value));
}

function emitMeta() {
  emit("update:sortOrder", Number(sortOrder.value) || 0);
}

function onModifyPatchInput(raw: string | number | null) {
  const text = String(raw ?? "").trim();
  if (!text) {
    localRule.value.action.modify_patch = {};
    modifyPatchError.value = "";
    emitChange();
    return;
  }
  try {
    localRule.value.action.modify_patch = JSON.parse(text) as Record<string, unknown>;
    modifyPatchError.value = "";
    emitChange();
  } catch {
    modifyPatchError.value = "JSON 格式错误";
  }
}
</script>
