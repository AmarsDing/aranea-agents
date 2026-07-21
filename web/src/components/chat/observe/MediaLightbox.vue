<template>
  <q-dialog :model-value="true" maximized @hide="$emit('close')">
    <q-card class="media-lightbox">
      <q-card-section class="row items-center q-pb-none">
        <q-space />
        <q-btn flat round dense icon="close" @click="$emit('close')" />
      </q-card-section>
      <q-card-section class="media-lightbox__content">
        <video
          v-if="artifact.mime_type.startsWith('video/')"
          :src="artifact.url"
          controls
          autoplay
          class="media-lightbox__media"
        />
        <img v-else :src="artifact.url" class="media-lightbox__media" />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

defineProps<{ artifact: MediaArtifact }>();
defineEmits<{ close: [] }>();
</script>

<style scoped lang="sass">
.media-lightbox
  background: rgba(0, 0, 0, 0.9)

.media-lightbox__content
  display: flex
  align-items: center
  justify-content: center
  height: 100%

.media-lightbox__media
  max-width: 90vw
  max-height: 80vh
  object-fit: contain
</style>
