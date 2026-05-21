<template>
  <q-card flat bordered class="q-mb-md">
    <q-card-section>
      <div class="text-subtitle2 q-mb-sm">{{ t("knowledgeEmbed.title") }}</div>
      <knowledge-embedder-fields
        :form="form"
        :configured="config?.configured"
        :has-api-key="config?.has_api_key"
        show-status
      >
        <template #actions>
          <q-btn color="primary" unelevated :label="t('knowledgeEmbed.save')" :loading="saving" @click="save" />
        </template>
      </knowledge-embedder-fields>
      <div class="text-caption text-grey-7 q-mt-sm">{{ t("knowledgeEmbed.hint") }}</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { reactive, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { EmbedderConfig, UpdateEmbedderConfigInput } from "../../features/knowledge/types";
import { DEFAULT_KNOWLEDGE_EMBED_FORM } from "../../features/knowledge/embedder-constants";
import KnowledgeEmbedderFields from "./KnowledgeEmbedderFields.vue";

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
    form.base_url = cfg.base_url || "";
    form.model = cfg.model || DEFAULT_KNOWLEDGE_EMBED_FORM.model;
    form.dim = cfg.dim || DEFAULT_KNOWLEDGE_EMBED_FORM.dim;
    form.api_key = "";
  },
  { immediate: true }
);

function save() {
  emit("save", {
    provider: form.provider,
    base_url: form.base_url.trim(),
    model: form.model.trim(),
    dim: form.dim,
    api_key: form.api_key.trim() || undefined
  });
}
</script>
