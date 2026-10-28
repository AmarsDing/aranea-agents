<!-- Container: owns artifact actions and failure feedback for the props/emits-only reference card. -->
<template>
  <ArtifactRefCard
    :artifact-id="artifactId"
    :name="name"
    :mime-type="mimeType"
    :downloading="downloading"
    @preview="onPreview"
    @download="onDownload"
  />
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useArtifactStore } from '../../stores/artifact';
import ArtifactRefCard from '../../components/artifact/ArtifactRefCard.vue';

defineProps<{ artifactId: string; name?: string; mimeType?: string }>();

const store = useArtifactStore();
const { t } = useI18n();
const $q = useQuasar();
const downloading = ref(false);

function onPreview(id: string) {
  store.openArtifactPreview(id);
}

async function onDownload(id: string) {
  if (downloading.value) return;
  downloading.value = true;
  try {
    await store.download({ id });
  } catch {
    $q.notify({ type: 'negative', message: t('uiSystem.operations.downloadFailed') });
  } finally {
    downloading.value = false;
  }
}
</script>
