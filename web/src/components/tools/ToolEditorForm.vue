<template>
  <div class="column q-gutter-md">
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input :model-value="form.key" dense outlined label="Key" :disable="Boolean(editingId)" @update:model-value="patch({ key: $event })" />
      <q-input :model-value="form.display_name" dense outlined label="显示名称" @update:model-value="patch({ display_name: $event })" />
      <q-input class="app-grid-span-full" :model-value="form.description" dense outlined autogrow label="描述" @update:model-value="patch({ description: $event })" />
      <q-input :model-value="form.category" dense outlined label="分类" @update:model-value="patch({ category: $event })" />
      <q-input :model-value="form.source" dense outlined label="来源" @update:model-value="patch({ source: $event })" />
      <q-select
        :model-value="form.risk_level"
        dense
        outlined
        emit-value
        map-options
        label="风险"
        :options="riskOptions"
        @update:model-value="patch({ risk_level: $event })"
      />
    </div>
    <div class="app-actions-bar app-actions-bar--start q-gutter-sm">
      <q-toggle class="col-auto" :model-value="form.enabled" label="启用" @update:model-value="patch({ enabled: $event })" />
      <q-toggle class="col-auto" :model-value="form.readonly" label="只读" @update:model-value="patch({ readonly: $event })" />
      <q-toggle class="col-auto" :model-value="form.requires_confirmation" label="需确认" @update:model-value="patch({ requires_confirmation: $event })" />
      <q-toggle class="col-auto" :model-value="form.supports_streaming" label="流式" @update:model-value="patch({ supports_streaming: $event })" />
      <q-toggle class="col-auto" :model-value="form.supports_concurrency" label="并发" @update:model-value="patch({ supports_concurrency: $event })" />
    </div>
    <q-input
      class="app-field-long"
      :model-value="form.parameters_schema_json"
      type="textarea"
      outlined
      label="参数 Schema JSON"
      :error="Boolean(errors.parameters_schema_json)"
      :error-message="errors.parameters_schema_json"
      @update:model-value="patch({ parameters_schema_json: $event })"
    />
    <q-input
      class="app-field-long"
      :model-value="form.result_schema_json"
      type="textarea"
      outlined
      label="返回 Schema JSON"
      :error="Boolean(errors.result_schema_json)"
      :error-message="errors.result_schema_json"
      @update:model-value="patch({ result_schema_json: $event })"
    />
    <q-input
      class="app-field-long"
      :model-value="form.config_schema_json"
      type="textarea"
      outlined
      label="配置 Schema JSON"
      @update:model-value="patch({ config_schema_json: $event })"
    />
    <template v-if="hasConfigSchema">
      <div class="text-caption q-mb-xs">配置（按 Schema 渲染）</div>
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
      label="配置 JSON"
      :error="Boolean(errors.config_json)"
      :error-message="errors.config_json"
      @update:model-value="patch({ config_json: $event })"
    />
    <q-input
      :model-value="form.default_config_json"
      type="textarea"
      outlined
      label="默认配置 JSON"
      :error="Boolean(errors.default_config_json)"
      :error-message="errors.default_config_json"
      @update:model-value="patch({ default_config_json: $event })"
    />
    <q-input
      :model-value="form.metadata_json"
      type="textarea"
      outlined
      label="元数据 JSON"
      :error="Boolean(errors.metadata_json)"
      :error-message="errors.metadata_json"
      @update:model-value="patch({ metadata_json: $event })"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ToolUpsertInput } from "../../features/tools/types";
import ToolSchemaForm from "./ToolSchemaForm.vue";

const props = defineProps<{
  form: ToolUpsertInput;
  editingId: string;
  errors: Record<string, string>;
  riskOptions: { label: string; value: string }[];
}>();

function patch(p: Partial<ToolUpsertInput>) {
  Object.assign(props.form, p);
}

const hasConfigSchema = computed(() => {
  try {
    const s = JSON.parse(props.form.config_schema_json || "{}");
    return s.properties && Object.keys(s.properties).length > 0;
  } catch {
    return false;
  }
});
</script>
