<template>
  <div class="node-media-preview">
    <div
      v-for="art in displayArtifacts"
      :key="art.artifact_id"
      class="node-media-preview__item"
      @click="$emit('preview', art)"
    >
      <video
        v-if="art.mime_type.startsWith('video/')"
        :src="art.url"
        :poster="art.thumbnail"
        muted
        preload="metadata"
        class="node-media-preview__media"
      />
      <img v-else :src="art.url" loading="lazy" class="node-media-preview__media" />
    </div>
    <span v-if="artifacts.length > 3" class="node-media-preview__more"> +{{ artifacts.length - 3 }} </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

const props = defineProps<{ artifacts: MediaArtifact[] }>();
defineEmits<{ preview: [art: MediaArtifact] }>();

const displayArtifacts = computed(() => props.artifacts.slice(0, 3));
</script>

<style scoped lang="sass">
.node-media-preview
  display: flex
  gap: 4px
  align-items: center
  margin-top: 6px

.node-media-preview__item
  width: 64px
  height: 64px
  border-radius: 4px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border-soft)
  flex-shrink: 0

  &:hover
    border-color: var(--q-primary)

.node-media-preview__media
  width: 100%
  height: 100%
  object-fit: cover

.node-media-preview__more
  font-size: 11px
  color: var(--color-text-tertiary)
  margin-left: 4px
</style>
