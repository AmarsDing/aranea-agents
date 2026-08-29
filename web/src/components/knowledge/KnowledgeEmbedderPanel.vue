<template>
  <section class="app-settings-section knowledge-embedder-panel">
    <!-- 头部：标题 + 副标题说明 + 配置状态徽标（状态上提，一眼可见） -->
    <div class="knowledge-embedder-panel__head">
      <div class="knowledge-embedder-panel__title-wrap">
        <h3 class="app-settings-section__title">{{ t('knowledgeEmbed.title') }}</h3>
        <p class="app-settings-section__hint q-mb-none q-mt-xs">{{ t('knowledgeEmbed.subtitle') }}</p>
      </div>
      <q-badge :color="config?.configured ? 'positive' : 'warning'" class="knowledge-embedder-panel__badge">
        {{ config?.configured ? t('knowledgeEmbed.configured') : t('knowledgeEmbed.notConfigured') }}
      </q-badge>
    </div>

    <knowledge-embedder-fields :form="form" :has-api-key="config?.has_api_key" />

    <!-- 操作条：保存右对齐，与表单网格拉开节奏 -->
    <div class="knowledge-embedder-panel__actions">
      <q-btn color="primary" unelevated no-caps :label="t('knowledgeEmbed.save')" :loading="saving" @click="save" />
    </div>

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
import AppStatusChip from '../common/AppStatusChip.vue';

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

<style lang="scss" scoped>
.knowledge-embedder-panel {
  &__head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 16px;
  }

  &__title-wrap {
    min-width: 0;
  }

  &__badge {
    flex: none;
    margin-top: 2px;
  }

  &__actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--color-border-soft);
  }
}
</style>
