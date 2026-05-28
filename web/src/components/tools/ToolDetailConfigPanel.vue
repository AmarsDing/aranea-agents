<template>
  <div class="tool-detail-config-panel">
    <p class="app-registry-muted-caption q-ma-none">
      修改全局配置（API Key、超时等）；须符合 config_schema。Agent 级覆盖在「Agent」Tab。
    </p>

    <q-banner v-if="tool?.key === 'web_research'" rounded dense class="settings-info-banner">
      API Key 留空时使用
      <router-link :to="{ name: 'settings' }" class="text-primary">系统设置 → Web 研究</router-link>。
    </q-banner>

    <q-expansion-item
      dense-toggle
      :default-opened="false"
      label="配置结构定义 (config_schema)"
      caption="定义该工具接受哪些配置项，通常由管理员维护"
      header-class="text-subtitle2"
      class="q-mb-md"
    >
      <div class="q-pt-sm">
        <tool-json-block :text="prettySchemaJson" />
        <q-input
          :model-value="schemaEditJson"
          type="textarea"
          outlined
          autogrow
          dense
          class="app-field-long q-mt-sm"
          label="编辑配置结构 (JSON)"
          @update:model-value="onSchemaEdit(String($event ?? '{}'))"
        />
        <q-banner v-if="schemaParseError" rounded class="settings-warning-banner q-mt-sm">
          {{ schemaParseError }}
        </q-banner>
      </div>
    </q-expansion-item>

    <section class="tool-editor-section">
      <div class="text-subtitle2 q-mb-sm">配置值</div>
      <template v-if="hasConfigSchema">
        <tool-schema-form
          :schema-json="tool!.config_schema_json"
          :model-value="configJson"
          @update:model-value="$emit('update:configJson', $event)"
        />
      </template>
      <q-input
        v-else
        :model-value="configJson"
        type="textarea"
        outlined
        autogrow
        dense
        class="app-field-long"
        label="配置 JSON"
        @update:model-value="$emit('update:configJson', String($event ?? '{}'))"
      />

      <q-banner v-if="extraKeys.length" rounded class="settings-warning-banner q-mt-sm">
        Schema 未声明字段：{{ extraKeys.join(", ") }}
      </q-banner>
    </section>

    <q-expansion-item
      v-if="tool?.default_config_json && tool.default_config_json !== '{}'"
      dense-toggle
      label="出厂默认配置"
      class="q-mt-md"
    >
      <div class="q-pt-sm">
        <tool-json-block :text="prettyDefaultConfig" />
        <ul v-if="diffLines.length" class="app-registry-muted-caption q-pl-md q-ma-none q-mt-xs">
          <li v-for="(line, i) in diffLines" :key="i">{{ line }}</li>
        </ul>
      </div>
    </q-expansion-item>

    <div class="row q-gutter-sm q-mt-md">
      <q-btn
        no-caps
        unelevated
        class="app-registry-primary-btn"
        label="保存配置"
        icon="save"
        :loading="saving"
        :disable="!tool?.id"
        @click="$emit('save')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { configExtraKeys, configDiffSummary } from "../../features/tools/jsonSchemaBuilder";
import { prettyJSON } from "./toolUi";
import type { Tool } from "../../features/tools/types";
import ToolSchemaForm from "./ToolSchemaForm.vue";
import ToolJsonBlock from "./ToolJsonBlock.vue";

const props = defineProps<{
  tool: Tool | null;
  configJson: string;
  saving: boolean;
}>();

defineEmits<{ save: []; "update:configJson": [value: string] }>();

const hasConfigSchema = computed(() => {
  try {
    const s = JSON.parse(props.tool?.config_schema_json || "{}");
    return s.properties && Object.keys(s.properties).length > 0;
  } catch {
    return false;
  }
});

const extraKeys = computed(() => {
  if (!props.tool) return [];
  return configExtraKeys(props.configJson, props.tool.config_schema_json);
});

const prettySchemaJson = computed(() => prettyJSON(props.tool?.config_schema_json || "{}"));

const prettyDefaultConfig = computed(() => prettyJSON(props.tool?.default_config_json || "{}"));

const diffLines = computed(() => {
  if (!props.tool) return [];
  return configDiffSummary(props.configJson, props.tool.default_config_json || "{}");
});

const schemaEditJson = ref("");
const schemaParseError = ref("");

function onSchemaEdit(val: string) {
  schemaEditJson.value = val;
  try {
    JSON.parse(val || "{}");
    schemaParseError.value = "";
  } catch (err) {
    schemaParseError.value = err instanceof Error ? err.message : "JSON 格式错误";
  }
}
</script>
