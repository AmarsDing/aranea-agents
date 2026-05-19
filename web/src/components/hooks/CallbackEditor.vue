<template>
  <div class="callback-editor q-gutter-md">
    <div class="row q-col-gutter-md">
      <q-select
        v-model="localRule.callback_point"
        class="col-12 col-md-4"
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
        class="col-12 col-md-4"
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
        class="col-12 col-md-4"
        dense
        outlined
        type="number"
        label="排序"
        @update:model-value="emitMeta"
      />
    </div>

    <q-expansion-item dense-toggle default-open label="触发条件">
      <div class="row q-col-gutter-md q-pa-sm">
        <q-input
          v-model="localRule.condition.agent_id"
          class="col-12 col-md-4"
          dense
          outlined
          clearable
          label="Agent ID / Key"
          hint="留空表示匹配全部 Agent"
          @update:model-value="emitChange"
        />
        <q-input
          v-model="localRule.condition.tool_name"
          class="col-12 col-md-4"
          dense
          outlined
          clearable
          label="Tool 名称"
          :disable="!toolPoint"
          @update:model-value="emitChange"
        />
        <q-input
          v-model="localRule.condition.event_type"
          class="col-12 col-md-4"
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
        <q-input
          v-if="localRule.action.type === 'notify'"
          v-model="localRule.action.webhook_url"
          dense
          outlined
          label="Webhook URL"
          @update:model-value="emitChange"
        />
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
            before_model: generation_config / append_system / append_user; before_tool: arguments / merge_arguments
          </div>
          <q-input
            v-model="modifyPatchText"
            dense
            outlined
            type="textarea"
            rows="8"
            label="modify_patch (JSON)"
            :error="modifyPatchError"
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
    localRule.value = structuredClone(v ?? defaultHookRuleConfig(props.agentId, props.agentKey));
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
  emit("update:modelValue", structuredClone(localRule.value));
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
