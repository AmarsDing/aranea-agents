<template>
  <div class="column q-gutter-md">
    <section class="tool-editor-section">
      <h3 class="tool-editor-section__title">模型参数（LLM 可见）</h3>
      <tool-schema-builder
        :model-value="form.parameters_schema_json"
        title="模型参数定义"
        :hint="hints.parameters_schema_json"
        :readonly="schemaReadonly"
        @update:model-value="patch({ parameters_schema_json: $event })"
      />
    </section>

    <q-expansion-item dense-toggle label="返回结构说明（可选）" header-class="text-subtitle2">
      <div class="q-pt-sm">
        <tool-schema-builder
          :model-value="form.result_schema_json"
          title="返回 Schema"
          :hint="hints.result_schema_json"
          :readonly="schemaReadonly"
          @update:model-value="patch({ result_schema_json: $event })"
        />
      </div>
    </q-expansion-item>

    <section class="tool-editor-section">
      <h3 class="tool-editor-section__title">管理员配置（LLM 不可见）</h3>
      <tool-schema-builder
        :model-value="form.config_schema_json"
        title="配置项定义"
        :hint="hints.config_schema_json"
        :readonly="schemaReadonly"
        @update:model-value="onConfigSchemaChange"
      />

      <q-banner v-if="form.key === 'web_research'" rounded dense class="settings-info-banner q-mt-sm">
        API Key 留空时使用
        <router-link :to="{ name: 'settings' }" class="text-primary">系统设置 → Web 研究</router-link>
        或环境变量 TAVILY_API_KEY。
      </q-banner>

      <div class="q-mt-md">
        <div class="tool-editor-section__title">当前配置值</div>
        <template v-if="hasConfigSchema">
          <tool-schema-form
            :schema-json="form.config_schema_json"
            :model-value="form.config_json"
            @update:model-value="patch({ config_json: $event })"
          />
        </template>
        <q-input
          v-else
          :model-value="form.config_json"
          type="textarea"
          outlined
          autogrow
          dense
          class="app-field-long"
          label="配置 JSON"
          :hint="hints.config_json"
          :readonly="schemaReadonly"
          :error="Boolean(errors.config_json)"
          :error-message="errors.config_json"
          @update:model-value="patch({ config_json: $event })"
        />
        <q-banner v-if="extraConfigKeys.length" rounded class="settings-warning-banner q-mt-sm">
          以下字段不在配置 Schema 中，保存时可能被后端拒绝：{{ extraConfigKeys.join(", ") }}
        </q-banner>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { configExtraKeys } from "../../../features/tools/jsonSchemaBuilder";
import { TOOL_FIELD_HINTS } from "../../../features/tools/toolEditorCopy";
import { patchToolForm } from "../../../features/tools/toolFormPatch";
import type { ToolUpsertInput } from "../../../features/tools/types";
import ToolSchemaForm from "../ToolSchemaForm.vue";
import ToolSchemaBuilder from "./ToolSchemaBuilder.vue";

const props = defineProps<{
  form: ToolUpsertInput;
  errors: Record<string, string>;
  schemaReadonly: boolean;
}>();

const hints = TOOL_FIELD_HINTS;

const hasConfigSchema = computed(() => {
  try {
    const s = JSON.parse(props.form.config_schema_json || "{}");
    return s.properties && Object.keys(s.properties).length > 0;
  } catch {
    return false;
  }
});

const extraConfigKeys = computed(() =>
  configExtraKeys(props.form.config_json, props.form.config_schema_json)
);

function patch(p: Partial<ToolUpsertInput>) {
  patchToolForm(props.form, p);
}

function onConfigSchemaChange(v: string) {
  patch({ config_schema_json: v });
}
</script>
