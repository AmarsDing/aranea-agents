<template>
  <div class="tool-detail-config-panel">
    <p class="app-registry-muted-caption q-ma-none">
      {{ $t('toolsPage.configPanel.hint') }}
    </p>

    <q-banner v-if="tool?.key === 'web_research'" rounded dense class="settings-info-banner">
      {{ $t('toolsPage.configPanel.webResearchPre') }}
      <router-link :to="{ name: 'settings' }" class="text-primary">{{ $t('toolsPage.configPanel.webResearchLink') }}</router-link>{{ $t('toolsPage.configPanel.webResearchPost') }}
    </q-banner>

    <q-expansion-item
      dense-toggle
      :default-opened="false"
      :label="$t('toolsPage.configPanel.schemaLabel')"
      :caption="$t('toolsPage.configPanel.schemaCaption')"
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
          :label="$t('toolsPage.configPanel.schemaEditLabel')"
          @update:model-value="onSchemaEdit(String($event ?? '{}'))"
        />
        <q-banner v-if="schemaParseError" rounded class="settings-warning-banner q-mt-sm">
          {{ schemaParseError }}
        </q-banner>
        <q-btn
          v-if="schemaEditDirty && !schemaParseError"
          flat
          dense
          no-caps
          icon="save"
          :label="$t('toolsPage.configPanel.schemaApply')"
          class="app-registry-accent-btn q-mt-sm"
          @click="$emit('update:configSchemaJson', schemaEditJson)"
        />
      </div>
    </q-expansion-item>

    <section class="tool-editor-section">
      <div class="text-subtitle2 q-mb-sm">{{ $t('toolsPage.configPanel.valuesTitle') }}</div>
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
        :label="$t('toolsPage.configPanel.configJsonLabel')"
        @update:model-value="$emit('update:configJson', String($event ?? '{}'))"
      />

      <q-banner v-if="extraKeys.length" rounded class="settings-warning-banner q-mt-sm">
        {{ $t('toolsPage.configPanel.undeclaredKeys', { keys: extraKeys.join(', ') }) }}
      </q-banner>
    </section>

    <q-expansion-item
      v-if="tool?.default_config_json && tool.default_config_json !== '{}'"
      dense-toggle
      :label="$t('toolsPage.configPanel.defaultConfig')"
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
        :label="$t('toolsPage.configPanel.save')"
        icon="save"
        :loading="saving"
        :disable="!tool?.id"
        @click="$emit('save')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { configExtraKeys, configDiffSummary } from '../../features/tools/jsonSchemaBuilder';
import { prettyJSON } from './toolUi';
import type { Tool } from '../../features/tools/types';
import ToolSchemaForm from './ToolSchemaForm.vue';
import ToolJsonBlock from './ToolJsonBlock.vue';

const props = defineProps<{
  tool: Tool | null;
  configJson: string;
  saving: boolean;
}>();

defineEmits<{ save: []; 'update:configJson': [value: string]; 'update:configSchemaJson': [value: string] }>();

const { t } = useI18n();

const hasConfigSchema = computed(() => {
  try {
    const s = JSON.parse(props.tool?.config_schema_json || '{}');
    return s.properties && Object.keys(s.properties).length > 0;
  } catch {
    return false;
  }
});

const extraKeys = computed(() => {
  if (!props.tool) return [];
  return configExtraKeys(props.configJson, props.tool.config_schema_json);
});

const prettySchemaJson = computed(() => prettyJSON(props.tool?.config_schema_json || '{}'));

const prettyDefaultConfig = computed(() => prettyJSON(props.tool?.default_config_json || '{}'));

const diffLines = computed(() => {
  if (!props.tool) return [];
  return configDiffSummary(props.configJson, props.tool.default_config_json || '{}');
});

const schemaEditJson = ref('');
const schemaParseError = ref('');

// 预填当前 schema：编辑框若初始为空，用户需手动粘贴整段 JSON 才能改一个字段。
// 切换工具时重置；应用变更后工具对象更新，文本与新 schema 等价，dirty 自然归零。
watch(
  () => props.tool?.id,
  () => {
    schemaEditJson.value = prettySchemaJson.value;
    schemaParseError.value = '';
  },
  { immediate: true },
);

function onSchemaEdit(val: string) {
  schemaEditJson.value = val;
  try {
    JSON.parse(val || '{}');
    schemaParseError.value = '';
  } catch (err) {
    schemaParseError.value = err instanceof Error ? err.message : t('toolsPage.invalidJsonFallback');
  }
}

const schemaEditDirty = computed(() => {
  try {
    return schemaEditJson.value.trim() !== '' && schemaEditJson.value.trim() !== prettySchemaJson.value.trim();
  } catch {
    return false;
  }
});
</script>
