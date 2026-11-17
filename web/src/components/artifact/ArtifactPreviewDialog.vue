<!-- web/src/components/artifact/ArtifactPreviewDialog.vue
  全局产物预览弹窗（P0/P1 会话产物点击查看，2026-09-01）。
  挂载于 MainLayout，目标由 artifactStore.previewTarget 驱动；
  内嵌 ArtifactPreview（text/image/pdf/audio/video 内联预览，binary 降级下载）。 -->
<template>
  <AppDialog
    :model-value="modelValue != null"
    :title="t('artifact.preview.title')"
    size="lg"
    @update:model-value="onDialogToggle"
  >
    <ArtifactPreview
      v-if="modelValue"
      :artifact-id="modelValue.id"
      :version="modelValue.version"
      show-download
      @download="(meta) => emit('download', meta)"
    />
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
