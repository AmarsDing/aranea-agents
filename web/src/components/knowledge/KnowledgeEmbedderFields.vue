<template>
  <div class="app-form-field-grid app-form-field-grid--embedder">
    <q-select
      v-model="form.provider"
      class="app-glass-control app-field-sm"
      dense
      outlined
      :label="t('knowledgeEmbed.provider')"
      :options="providerOptions"
      emit-value
      map-options
    />
    <q-input
      v-model="form.base_url"
      class="app-glass-control"
      dense
      outlined
      :label="t('knowledgeEmbed.baseUrl')"
      placeholder="https://api.openai.com"
    />
    <q-input
      v-model="form.model"
      class="app-glass-control app-field-md"
      dense
      outlined
      :label="t('knowledgeEmbed.model')"
    />
    <q-input
      v-model.number="form.dim"
      class="app-glass-control app-field-sm"
      dense
      outlined
      type="number"
      :label="t('knowledgeEmbed.dim')"
    />
    <q-input
      v-model="form.api_key"
      class="app-glass-control app-grid-span-full app-field-long"
      dense
      outlined
      :label="t('knowledgeEmbed.apiKey')"
      type="password"
      :placeholder="hasApiKey ? t('knowledgeEmbed.apiKeyPlaceholderSet') : t('knowledgeEmbed.apiKeyPlaceholderEmpty')"
    />
    <div v-if="showStatus" class="app-grid-span-full app-actions-bar app-actions-bar--start">
      <q-badge :color="configured ? 'positive' : 'warning'">
        {{ configured ? t('knowledgeEmbed.configured') : t('knowledgeEmbed.notConfigured') }}
      </q-badge>
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppStatusChip from '../common/AppStatusChip.vue';
import {
  KNOWLEDGE_EMBED_PROVIDER_OPTIONS,
  type KnowledgeEmbedFormState,
} from '../../features/knowledge/embedder-constants';

const form = defineModel<KnowledgeEmbedFormState>('form', { required: true });

defineProps<{
  configured?: boolean;
  hasApiKey?: boolean;
  showStatus?: boolean;
}>();

const { t } = useI18n();
const providerOptions = KNOWLEDGE_EMBED_PROVIDER_OPTIONS;
</script>
