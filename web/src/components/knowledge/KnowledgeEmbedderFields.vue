<template>
  <div class="row q-col-gutter-md">
    <div class="col-12 col-md-3">
      <q-select
        v-model="form.provider"
        dense
        outlined
        :label="t('knowledgeEmbed.provider')"
        :options="providerOptions"
        emit-value
        map-options
      />
    </div>
    <div class="col-12 col-md-5">
      <q-input
        v-model="form.base_url"
        dense
        outlined
        :label="t('knowledgeEmbed.baseUrl')"
        placeholder="https://api.openai.com"
      />
    </div>
    <div class="col-12 col-md-4">
      <q-input v-model="form.model" dense outlined :label="t('knowledgeEmbed.model')" />
    </div>
    <div class="col-12 col-md-3">
      <q-input v-model.number="form.dim" dense outlined type="number" :label="t('knowledgeEmbed.dim')" />
    </div>
    <div class="col-12 col-md-5">
      <q-input
        v-model="form.api_key"
        dense
        outlined
        :label="t('knowledgeEmbed.apiKey')"
        type="password"
        :placeholder="hasApiKey ? t('knowledgeEmbed.apiKeyPlaceholderSet') : t('knowledgeEmbed.apiKeyPlaceholderEmpty')"
      />
    </div>
    <div v-if="showStatus" class="col-12 col-md-4 flex items-center q-gutter-sm">
      <q-badge :color="configured ? 'positive' : 'warning'">
        {{ configured ? t("knowledgeEmbed.configured") : t("knowledgeEmbed.notConfigured") }}
      </q-badge>
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import {
  KNOWLEDGE_EMBED_PROVIDER_OPTIONS,
  type KnowledgeEmbedFormState
} from "../../features/knowledge/embedder-constants";

defineProps<{
  form: KnowledgeEmbedFormState;
  configured?: boolean;
  hasApiKey?: boolean;
  showStatus?: boolean;
}>();

const { t } = useI18n();
const providerOptions = KNOWLEDGE_EMBED_PROVIDER_OPTIONS;
</script>
