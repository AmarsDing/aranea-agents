<template>
  <div class="tool-detail">
    <div v-if="prompt" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.prompt') }}</div>
      <code class="tool-detail__inline">{{ prompt }}</code>
    </div>
    <div v-if="artifacts.length" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.mediaOutputs') }}</div>
      <div class="media-tool-detail__grid">
        <div
          v-for="art in artifacts"
          :key="art.artifact_id || art.url"
          class="media-tool-detail__item"
          @click="onPreview(art)"
        >
          <video
            v-if="art.mime_type.startsWith('video/')"
            :src="mediaSrc(art)"
            :poster="art.thumbnail"
            muted
            preload="metadata"
            class="media-tool-detail__media"
          />
          <img v-else :src="mediaSrc(art)" loading="lazy" class="media-tool-detail__media" />
        </div>
      </div>
    </div>
    <div v-if="!artifacts.length && rawResult" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.result') }}</div>
      <pre class="tool-detail__pre">{{ rawResult }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { useMediaUrl } from '../../../features/chat/useMediaUrl';
import { asRecord } from './toolDetailShared';

const { t } = useI18n();
const { mediaSrc } = useMediaUrl();

const props = defineProps<{ step: Step }>();

const emit = defineEmits<{
  preview: [art: MediaArtifact];
}>();

const parsedArgs = computed(() => asRecord(props.step.ToolArgs));
const parsedResult = computed(() => asRecord(props.step.ToolResult));

const prompt = computed(() => String(parsedArgs.value?.prompt ?? ''));

const artifacts = computed<MediaArtifact[]>(() => {
  const raw = parsedResult.value?.artifacts;
  if (!Array.isArray(raw)) return [];
  return raw.filter(
    (a): a is MediaArtifact => typeof a === 'object' && a !== null && 'artifact_id' in a && 'mime_type' in a,
  );
});

const rawResult = computed(() => {
  if (!parsedResult.value) return '';
  return JSON.stringify(parsedResult.value, null, 2).slice(0, 500);
});

function onPreview(art: MediaArtifact) {
  emit('preview', art);
}
</script>

<style scoped lang="sass">
.media-tool-detail__grid
  display: flex
  gap: 8px
  flex-wrap: wrap
  margin-top: 4px

.media-tool-detail__item
  width: 120px
  height: 120px
  border-radius: 6px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border-soft)
  transition: border-color 0.2s

  &:hover
    border-color: var(--color-primary)

.media-tool-detail__media
  width: 100%
  height: 100%
  object-fit: cover
</style>
