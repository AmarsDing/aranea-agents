<template>
  <div class="column q-gutter-md">
    <q-banner rounded class="settings-warning-banner">
      以下字段影响工具契约；错误配置可能导致 Agent 无法调用。不确定时保持 {} 或使用模板。
    </q-banner>

    <q-banner v-if="registryLocked" rounded dense class="settings-info-banner">
      只读内置工具：高级 JSON 可查看；修改 Schema / metadata 可能在重启后被 registry 覆盖。
    </q-banner>

    <q-expansion-item dense-toggle default-opened label="出厂默认配置">
      <div class="q-pt-sm column q-gutter-sm">
        <p class="app-registry-muted-caption q-ma-none">{{ hints.default_config_json }}</p>
        <ul v-if="diffLines.length" class="app-registry-muted-caption q-pl-md q-ma-none">
          <li v-for="(line, i) in diffLines" :key="i">{{ line }}</li>
        </ul>
        <q-input
          :model-value="form.default_config_json"
          type="textarea"
          outlined
          autogrow
          dense
          class="app-field-long"
          label="默认配置 JSON"
          :readonly="registryLocked"
          :error="Boolean(errors.default_config_json)"
          :error-message="errors.default_config_json"
          @update:model-value="patch({ default_config_json: $event })"
        />
        <q-btn v-if="!registryLocked" flat dense no-caps size="sm" label="从当前配置复制" @click="copyConfigToDefault" />
      </div>
    </q-expansion-item>

    <q-expansion-item dense-toggle label="扩展元数据">
      <div class="q-pt-sm">
        <p class="app-registry-muted-caption">{{ hints.metadata_json }}</p>
        <q-input
          :model-value="form.metadata_json"
          type="textarea"
          outlined
          autogrow
          dense
          class="app-field-long"
          label="元数据 JSON"
          :readonly="registryLocked"
          :error="Boolean(errors.metadata_json)"
          :error-message="errors.metadata_json"
          @update:model-value="patch({ metadata_json: $event })"
        />
      </div>
    </q-expansion-item>

    <q-expansion-item dense-toggle label="Raw JSON（全部 Schema）">
      <div class="q-pt-sm column q-gutter-sm">
        <q-input
          v-for="field in rawFields"
          :key="field.key"
          :model-value="form[field.key]"
          type="textarea"
          outlined
          autogrow
          dense
          class="app-field-long"
          :label="field.label"
          :readonly="registryLocked"
          :error="Boolean(errors[field.key])"
          :error-message="errors[field.key]"
          @update:model-value="patch({ [field.key]: $event })"
        />
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { configDiffSummary } from "../../../features/tools/jsonSchemaBuilder";
import { TOOL_FIELD_HINTS } from "../../../features/tools/toolEditorCopy";
import { patchToolForm } from "../../../features/tools/toolFormPatch";
import type { ToolUpsertInput } from "../../../features/tools/types";

const props = defineProps<{
  form: ToolUpsertInput;
  errors: Record<string, string>;
  registryLocked: boolean;
}>();

const hints = TOOL_FIELD_HINTS;

const rawFields = [
  { key: "parameters_schema_json" as const, label: "参数 Schema JSON" },
  { key: "result_schema_json" as const, label: "返回 Schema JSON" },
  { key: "config_schema_json" as const, label: "配置 Schema JSON" }
];

const diffLines = computed(() =>
  configDiffSummary(props.form.config_json, props.form.default_config_json)
);

function patch(p: Partial<ToolUpsertInput>) {
  patchToolForm(props.form, p);
}

function copyConfigToDefault() {
  patch({ default_config_json: props.form.config_json });
}
</script>
