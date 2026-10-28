<!-- web/src/components/artifact/ArtifactRefCard.vue
  工具结果产物卡片（P1 消息流产物点击查看，2026-09-01）。
  工具结果（如 officecli_render）携带 artifact_id 时由 ActionBlock 挂载；
  操作经 props/emits 交给 ArtifactRef 容器。 -->
<template>
  <div class="artifact-ref-card row items-center no-wrap">
    <button type="button" class="artifact-ref-card__preview" @click="$emit('preview', artifactId)">
      <q-icon :name="iconOf(mimeType)" size="sm" aria-hidden="true" />
      <span class="artifact-ref-card__name ellipsis">{{ displayName }}</span>
    </button>
    <q-btn flat round dense icon="download" size="sm" :loading="downloading" :disable="downloading" :aria-label="t('artifact.preview.download')" @click="$emit('download', artifactId)">
      <q-tooltip>{{ t('artifact.preview.download') }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  artifactId: string;
  name?: string;
  mimeType?: string;
  downloading?: boolean;
}>();

defineEmits<{
  preview: [artifactId: string];
  download: [artifactId: string];
}>();

const { t } = useI18n();

const displayName = computed(() => props.name || t('artifact.refCard.defaultName'));

/** MIME → 图标（与 SessionArtifactsDrawer 同一套映射）。 */
function iconOf(mime?: string): string {
  const m = (mime || '').toLowerCase();
  if (m.startsWith('image/')) return 'image';
  if (m.startsWith('audio/')) return 'audiotrack';
  if (m.startsWith('video/')) return 'movie';
  if (m === 'application/pdf') return 'picture_as_pdf';
  if (m.startsWith('text/') || m === 'application/json' || m === 'application/xml') return 'article';
  return 'description';
}
</script>

<style scoped lang="sass">
.artifact-ref-card
  gap: 4px
  margin: 4px 0 4px 20px
  padding: 4px 8px
  max-width: 360px
  border: 1px solid var(--glass-border)
  border-radius: var(--radius-control)
  &:hover
    background: var(--surface-muted)

.artifact-ref-card__preview
  display: flex
  align-items: center
  flex: 1
  min-width: 0
  gap: 8px
  padding: 8px 4px
  border: 0
  background: transparent
  color: var(--color-accent)
  text-align: left
  cursor: pointer

.artifact-ref-card__name
  font-size: 12px
  color: var(--color-text-primary)
</style>
