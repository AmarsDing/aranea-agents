<template>
  <div class="app-form-field-grid app-form-field-grid--2col">
    <q-select
      v-model="form.provider"
      class="app-field-sm"
      dense
      outlined
      :label="t('settingsPage.webResearch.provider')"
      :options="providerOptions"
      emit-value
      map-options
    />
    <q-select
      v-model="form.search_depth"
      class="app-field-sm"
      dense
      outlined
      :label="t('settingsPage.webResearch.searchDepth')"
      :options="depthOptions"
      emit-value
      map-options
      :disable="form.provider !== 'tavily'"
    />
    <q-input
      v-model="form.api_key"
      class="app-grid-span-full app-field-long"
      dense
      outlined
      :label="apiKeyLabel"
      type="password"
      :placeholder="hasApiKey ? t('settingsPage.webResearch.apiKeySet') : t('settingsPage.webResearch.apiKeyEmpty')"
    />
    <q-input
      v-model.number="form.max_results"
      class="app-field-sm"
      dense
      outlined
      type="number"
      min="1"
      max="20"
      :label="t('settingsPage.webResearch.maxResults')"
    />
    <q-input
      v-model.number="form.fetch_top"
      class="app-field-sm"
      dense
      outlined
      type="number"
      min="0"
      max="20"
      :label="t('settingsPage.webResearch.fetchTop')"
    />
    <q-input
      v-model.number="form.timeout_sec"
      class="app-field-sm"
      dense
      outlined
      type="number"
      min="5"
      max="120"
      :label="t('settingsPage.webResearch.timeoutSec')"
    />
    <q-input
      v-model="form.http_proxy"
      class="app-grid-span-full app-field-long"
      dense
      outlined
      :label="t('settingsPage.webResearch.httpProxy')"
      :hint="t('settingsPage.webResearch.httpProxyHint')"
      placeholder="http://127.0.0.1:7890"
    />
    <div v-if="showStatus" class="app-grid-span-full app-actions-bar app-actions-bar--start">
      <q-badge :color="configured ? 'positive' : 'warning'">
        {{ configured ? t("settingsPage.webResearch.configured") : t("settingsPage.webResearch.notConfigured") }}
      </q-badge>
      <q-btn
        outline
        rounded
        no-caps
        color="primary"
        icon="science"
        :label="t('settingsPage.webResearch.testConnection')"
        :loading="testing"
        :disable="testing"
        @click="$emit('test')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import {
  WEB_RESEARCH_DEPTH_OPTIONS,
  WEB_RESEARCH_PROVIDER_OPTIONS,
  type WebResearchFormState
} from "../../features/system-settings/web-research";

const props = defineProps<{
  form: WebResearchFormState;
  configured?: boolean;
  hasApiKey?: boolean;
  showStatus?: boolean;
  testing?: boolean;
}>();

defineEmits<{
  test: [];
}>();

const { t } = useI18n();
const providerOptions = WEB_RESEARCH_PROVIDER_OPTIONS;
const depthOptions = WEB_RESEARCH_DEPTH_OPTIONS;

const apiKeyLabel = computed(() =>
  props.form.provider === "serpapi"
    ? t("settingsPage.webResearch.serpApiKey")
    : t("settingsPage.webResearch.tavilyApiKey")
);
</script>
