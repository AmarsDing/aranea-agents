<template>
  <div class="artifact-preview">
    <div v-if="loading" class="row justify-center q-py-lg">
      <q-spinner color="primary" size="2em" />
    </div>
    <template v-else-if="preview">
      <div class="artifact-preview__header q-mb-sm row items-center q-gutter-sm">
        <strong class="artifact-preview__name ellipsis" :title="preview.meta.name">{{ preview.meta.name }}</strong>
        <q-icon :name="kindIcon" size="sm" />
        <span class="text-caption text-grey-7">{{ formatBytes(preview.meta.size) }}</span>
        <span class="text-caption text-grey-7">v{{ preview.meta.version }}</span>
        <q-space />
        <q-btn
          v-if="showDownload"
          flat
          dense
          round
          icon="download"
          size="sm"
          :aria-label="t('artifact.preview.download')"
          @click="$emit('download', preview.meta)"
        >
          <q-tooltip>{{ t('artifact.preview.download') }}</q-tooltip>
        </q-btn>
      </div>

      <div v-if="preview.preview_kind === 'text'" class="artifact-preview__text">
        <q-btn
          v-if="isMarkdown"
          flat
          dense
          no-caps
          class="q-mb-xs"
          :label="t(showSource ? 'taskWorkspace.readDocument' : 'taskWorkspace.viewSource')"
          @click="
            showSource = !showSource;
            selectedText = '';
          "
        />
        <div v-if="allowQuote" class="q-mb-sm">
          <q-btn
            flat
            dense
            no-caps
            icon="format_quote"
            :label="t('taskWorkspace.quoteSelection')"
            :disable="!selectedText"
            @click="onQuote"
          />
          <div class="text-caption text-cream-muted">{{ t('taskWorkspace.excerptHint') }}</div>
        </div>
        <div
          ref="textElement"
          tabindex="0"
          :aria-label="preview.meta.name"
          @mouseup="onSelectText"
          @keyup="onSelectText"
        >
          <!-- eslint-disable vue/no-v-html -- renderChatMarkdown disables raw HTML and sanitizes its output. -->
          <div
            v-if="isMarkdown && !showSource"
            class="artifact-preview__reading"
            @click="handleMessagesClick"
            v-html="renderedDocument"
          />
          <!-- eslint-enable vue/no-v-html -->
          <pre v-else class="artifact-preview__code">{{ preview.text_content }}</pre>
        </div>
      </div>

      <div v-else-if="preview.preview_kind === 'image'" class="artifact-preview__image">
        <img :src="imageSrc" :alt="preview.meta.name" class="artifact-preview__img" />
      </div>

      <div v-else-if="preview.preview_kind === 'pdf'" class="artifact-preview__pdf">
        <iframe :src="pdfSrc" class="artifact-preview__iframe" :title="preview.meta.name" />
      </div>

      <div v-else-if="preview.preview_kind === 'audio'" class="artifact-preview__audio q-pa-md">
        <audio v-if="inlineMediaSrc" :src="inlineMediaSrc" controls class="artifact-preview__audio-el" />
        <div v-else-if="!mediaError" class="text-grey-7 text-caption">{{ t('artifact.preview.audioLoading') }}</div>
      </div>

      <div v-else-if="preview.preview_kind === 'video'" class="artifact-preview__video">
        <video v-if="inlineMediaSrc" :src="inlineMediaSrc" controls class="artifact-preview__video-el" />
        <div v-else-if="!mediaError" class="text-grey-7 text-caption q-pa-md">
          {{ t('artifact.preview.videoLoading') }}
        </div>
      </div>

      <div v-else class="artifact-preview__binary q-pa-md text-center text-grey-7">
        <q-icon name="description" size="3em" class="q-mb-sm" />
        <div>{{ preview.meta.name }}</div>
        <div class="text-caption">{{ t('artifact.preview.unsupported') }}</div>
      </div>
    </template>
    <div v-if="mediaError" class="q-pa-sm text-warning">
      {{ t('taskWorkspace.mediaFailed') }}
      <q-btn flat dense :label="t('taskWorkspace.retry')" @click="loadPreview" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">
      {{ error }}
      <q-btn flat dense :label="t('taskWorkspace.retry')" @click="loadPreview" />
    </div>
  </div>
</template>

// Container: approved because these are container components that coordinate artifact state for their parent page
<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { computed, nextTick, ref, watch } from 'vue';
import { renderChatMarkdown } from '../chat/chatMessageMarkdown';
import { useChatCodeCopy } from '../chat/composables/useChatMessageScroll';
import { useArtifactPreview } from './useArtifactPreview';
import type { ArtifactMeta, ArtifactTextQuote } from './types';

const props = defineProps<{
  artifactId: string;
  version?: number;
  showDownload?: boolean;
  expectedSessionId?: string;
  allowQuote?: boolean;
}>();

const emit = defineEmits<{
  ready: [];
  download: [meta: ArtifactMeta];
  quote: [quote: ArtifactTextQuote];
}>();

const { t } = useI18n();

const { preview, loading, error, mediaError, kindIcon, imageSrc, pdfSrc, inlineMediaSrc, formatBytes, loadPreview } =
  useArtifactPreview(
    () => props.artifactId,
    () => props.version,
    () => props.expectedSessionId,
  );
const textElement = ref<HTMLElement | null>(null);
const selectedText = ref('');
const showSource = ref(false);
const isMarkdown = computed(
  () => preview.value?.meta.mime_type === 'text/markdown' || /\.md(?:own)?$/i.test(preview.value?.meta.name ?? ''),
);
const renderedDocument = computed(() => renderChatMarkdown(preview.value?.text_content ?? ''));
const { handleMessagesClick } = useChatCodeCopy();
watch(
  preview,
  () => {
    selectedText.value = '';
    showSource.value = false;
    if (preview.value) void nextTick(() => emit('ready'));
  },
  { flush: 'sync' },
);

function onSelectText() {
  const selection = window.getSelection();
  const root = textElement.value;
  selectedText.value =
    selection &&
    root &&
    selection.rangeCount > 0 &&
    root.contains(selection.anchorNode) &&
    root.contains(selection.focusNode)
      ? selection.toString().trim()
      : '';
}

function onQuote() {
  if (preview.value && selectedText.value) emit('quote', { meta: preview.value.meta, text: selectedText.value });
}
</script>

<style scoped lang="sass">
.artifact-preview__name
  flex: 1 1 100%
  font-size: 15px
.artifact-preview__reading
  font-size: 15px
  line-height: 1.65
  overflow-wrap: anywhere
  color: var(--color-text-primary)
  :deep(h1), :deep(h2), :deep(h3)
    line-height: 1.4
    margin: 1em 0 0.5em
  :deep(h1)
    font-size: 1.45em
  :deep(h2)
    font-size: 1.25em
  :deep(p)
    margin: 0.75em 0
  :deep(ul), :deep(ol)
    padding-left: 1.6em
  :deep(table)
    display: block
    overflow-x: auto
    border-collapse: collapse
  :deep(th), :deep(td)
    padding: 6px 8px
    border: 1px solid var(--glass-border)
  :deep(a)
    color: var(--color-accent-text)
    text-decoration: underline
  :deep(img)
    max-width: 100%
  :deep(pre)
    white-space: pre-wrap
  :deep(button)
    color: var(--color-text-primary)
.artifact-preview__audio-el
  width: 100%

.artifact-preview__video-el
  max-width: 100%
  max-height: 60vh
  display: block
  margin: 0 auto
</style>
