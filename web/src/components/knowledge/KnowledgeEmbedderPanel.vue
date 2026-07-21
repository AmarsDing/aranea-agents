<template>
  <section class="app-settings-section knowledge-embedder-panel">
    <h3 class="app-settings-section__title">{{ t('knowledgeEmbed.title') }}</h3>
    <knowledge-embedder-fields
      :form="form"
      :configured="config?.configured"
      :has-api-key="config?.has_api_key"
      show-status
    >
      <template #actions>
        <q-btn color="primary" unelevated no-caps :label="t('knowledgeEmbed.save')" :loading="saving" @click="save" />
      </template>
    </knowledge-embedder-fields>
    <p class="app-settings-section__hint q-mb-none q-mt-md">
      {{ t('knowledgePage.embedderSharedHintPre')
      }}<router-link class="text-primary" to="/settings">{{ t('knowledgePage.embedderSharedHintLink') }}</router-link
      >{{ t('knowledgePage.embedderSharedHintPost') }}
    </p>
    <p class="app-settings-section__hint q-mb-none q-mt-sm">{{ t('knowledgeEmbed.hint') }}</p>
  </section>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EmbedderConfig, UpdateEmbedderConfigInput } from '../../features/knowledge/types';
import { DEFAULT_KNOWLEDGE_EMBED_FORM } from '../../features/knowledge/embedder-constants';
import KnowledgeEmbedderFields from './KnowledgeEmbedderFields.vue';

const props = defineProps<{
  config: EmbedderConfig | null;
  saving?: boolean;
}>();

const emit = defineEmits<{
  save: [input: UpdateEmbedderConfigInput];
}>();

const { t } = useI18n();

const form = reactive({ ...DEFAULT_KNOWLEDGE_EMBED_FORM });

watch(
  () => props.config,
  (cfg) => {
    if (!cfg) return;
    form.provider = cfg.provider || DEFAULT_KNOWLEDGE_EMBED_FORM.provider;
    form.base_url = cfg.base_url || '';
    form.model = cfg.model || DEFAULT_KNOWLEDGE_EMBED_FORM.model;
    form.dim = cfg.dim || DEFAULT_KNOWLEDGE_EMBED_FORM.dim;
    form.api_key = '';
  },
  { immediate: true },
);

function save() {
  emit('save', {
    provider: form.provider,
    base_url: form.base_url.trim(),
    model: form.model.trim(),
    dim: form.dim,
    api_key: form.api_key.trim() || undefined,
  });
}
</script>
