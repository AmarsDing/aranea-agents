<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-glass-dialog knowledge-doc-preview">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title ellipsis col">
          {{ source || t('knowledgePage.previewTitleFallback') }}
          <q-chip v-if="organized" dense color="positive" text-color="white" size="sm" class="q-ml-sm">
            {{ t('knowledgePage.previewOrganizedBadge') }}
          </q-chip>
          <q-chip v-else-if="!loading" dense color="grey-6" text-color="white" size="sm" class="q-ml-sm">
            {{ t('knowledgePage.previewRawBadge') }}
          </q-chip>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-glass-dialog__body">
          <div v-if="loading" class="row justify-center q-pa-xl">
            <q-spinner-dots color="primary" size="32px" />
          </div>
          <div v-else-if="content" class="chat-message-prose knowledge-doc-preview__md" v-html="rendered"></div>
          <div v-else class="text-grey-6 text-center q-pa-xl">{{ t('knowledgePage.previewEmpty') }}</div>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import MarkdownIt from 'markdown-it';

const props = defineProps<{
  open: boolean;
  source: string;
  organized: boolean;
  content: string;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();

const { t } = useI18n();

// 静态全文渲染：html:false 转义原始 HTML，避免注入；预览场景无需流式分段。
const md = new MarkdownIt({ html: false, linkify: true, breaks: false });
const rendered = computed(() => md.render(props.content));
</script>

<style scoped>
.knowledge-doc-preview {
  width: min(860px, 92vw);
  max-width: 92vw;
  height: min(72vh, 900px);
  display: flex;
  flex-direction: column;
}
.knowledge-doc-preview__md {
  font-size: 13px;
  line-height: 1.7;
  word-break: break-word;
  white-space: normal;
}
</style>
