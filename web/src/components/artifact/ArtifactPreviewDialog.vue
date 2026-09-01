<!-- web/src/components/artifact/ArtifactPreviewDialog.vue
  全局产物预览弹窗（P0/P1 会话产物点击查看，2026-09-01）。
  挂载于 MainLayout，目标由 artifactStore.previewTarget 驱动；
  内嵌 ArtifactPreview（text/image/pdf/audio/video 内联预览，binary 降级下载）。 -->
<template>
  <q-dialog :model-value="modelValue != null" @update:model-value="onDialogToggle">
    <q-card v-if="modelValue" class="artifact-preview-dialog">
      <q-card-section class="row items-center q-py-sm">
        <div class="text-subtitle1 ellipsis">{{ t('artifact.preview.title') }}</div>
        <q-space />
        <q-btn v-close-popup flat round dense icon="close" :aria-label="t('artifact.preview.close')" />
      </q-card-section>
      <q-separator />
      <q-card-section class="artifact-preview-dialog__body">
        <ArtifactPreview
          :artifact-id="modelValue.id"
          :version="modelValue.version"
          show-download
          @download="(meta) => emit('download', meta)"
        />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import ArtifactPreview from '../../features/artifact/ArtifactPreview.vue';
import type { ArtifactMeta } from '../../features/artifact/types';

defineProps<{
  modelValue: { id: string; version?: number } | null;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: { id: string; version?: number } | null];
  download: [meta: ArtifactMeta];
}>();

const { t } = useI18n();

function onDialogToggle(open: boolean) {
  if (!open) emit('update:modelValue', null);
}
</script>

<style scoped lang="sass">
.artifact-preview-dialog
  width: min(860px, 92vw)
  max-width: 92vw

.artifact-preview-dialog__body
  max-height: 76vh
  overflow: auto
</style>
