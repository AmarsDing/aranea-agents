<!-- web/src/components/artifact/ArtifactRefCard.vue
  工具结果产物卡片（P1 消息流产物点击查看，2026-09-01）。
  工具结果（如 officecli_render）携带 artifact_id 时由 ActionBlock 挂载；
  点击经 artifactStore.openArtifactPreview 打开全局预览弹窗。 -->
<template>
  <div class="artifact-ref-card row items-center no-wrap" role="button" tabindex="0" @click="onPreview" @keyup.enter="onPreview">
    <q-icon :name="iconOf(mimeType)" size="sm" color="accent" class="q-mr-sm" />
    <span class="artifact-ref-card__name ellipsis">{{ displayName }}</span>
    <q-btn flat round dense icon="download" size="sm" :aria-label="t('artifact.preview.download')" @click.stop="onDownload">
      <q-tooltip>{{ t('artifact.preview.download') }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useArtifactStore } from '../../stores/artifact';

const props = defineProps<{
  artifactId: string;
  name?: string;
  mimeType?: string;
}>();

const { t } = useI18n();
const artifactStore = useArtifactStore();

const displayName = computed(() => props.name || t('artifact.refCard.defaultName'));

function onPreview() {
  artifactStore.openArtifactPreview(props.artifactId);
}

async function onDownload() {
  try {
    await artifactStore.download({ id: props.artifactId });
  } catch {
    // 下载失败静默降级：签名失败等场景已由浏览器/控制台可见，
    // 与会话页既有下载入口的失败处理保持一致（不弹额外通知）。
  }
}

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
  border: 1px solid var(--q-primary)
  border-radius: 6px
  cursor: pointer
  opacity: 0.9
  &:hover
    opacity: 1
    background: rgba(127, 127, 127, 0.08)

.artifact-ref-card__name
  font-size: 12px
  color: var(--color-text-primary)
</style>
