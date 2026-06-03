<template>
  <div class="artifact-list">
    <div v-if="loading" class="row justify-center q-py-sm">
      <q-spinner color="primary" size="1.5em" />
    </div>
    <template v-else-if="displayItems.length">
      <div class="artifact-list__items">
        <div
          v-for="item in displayItems"
          :key="item.id"
          class="artifact-list__item row items-center q-gutter-xs"
          clickable
        >
          <q-icon :name="mimeIcon(item.mime_type)" size="18px" color="grey-7" />
          <div class="col" style="min-width: 0">
            <div class="artifact-list__name app-ellipsis">{{ item.name }}</div>
            <div class="artifact-list__meta text-caption text-grey-7">
              {{ formatBytes(item.size) }}
              <span v-if="item.version > 0"> · v{{ item.version }}</span>
            </div>
          </div>
          <q-btn flat dense round icon="visibility" size="xs" @click.stop="onView(item)">
            <q-tooltip>{{ t('chat.sessionArtifacts.view', '查看') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="download" size="xs" @click.stop="onDownload(item)">
            <q-tooltip>{{ t('chat.sessionArtifacts.download', '下载') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="delete" size="xs" color="negative" @click.stop="onDelete(item)">
            <q-tooltip>{{ t('chat.sessionArtifacts.delete', '删除') }}</q-tooltip>
          </q-btn>
        </div>
      </div>
    </template>
    <div v-else class="text-caption text-grey-7 q-pa-sm">
      {{ t('chat.sessionArtifacts.empty', '暂无制品') }}
    </div>

    <q-dialog v-model="previewOpen" transition-show="slide-up" transition-hide="slide-down">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="row items-center q-pb-none">
          <div class="text-subtitle1 app-ellipsis col">{{ previewMeta?.name }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-card-section v-if="previewMeta" class="q-pt-sm text-caption text-grey-7">
          {{ previewMeta.mime_type }} · {{ formatBytes(previewMeta.size) }}
          <span v-if="previewMeta.version > 0"> · v{{ previewMeta.version }}</span>
        </q-card-section>
        <q-card-section v-if="previewArtifactId">
          <ArtifactPreview :artifact-id="previewArtifactId" :show-download="true" @download="onPreviewDownload" />
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

// Container: approved because these are container components that coordinate artifact state for their parent page
<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useArtifactStore } from '../../stores/artifact';
import { formatBytes } from '../../shared/format';
import ArtifactPreview from './ArtifactPreview.vue';
import type { ArtifactMeta } from './types';

const props = defineProps<{
  items: ArtifactMeta[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  open: [id: string];
  deleted: [id: string];
}>();

const { t } = useI18n();
const { notify } = useQuasar();
const artifactStore = useArtifactStore();

const deletedIds = ref(new Set<string>());
const displayItems = computed(() => props.items.filter((item) => !deletedIds.value.has(item.id)));

const previewOpen = ref(false);
const previewMeta = ref<ArtifactMeta | null>(null);
const previewArtifactId = ref('');

function mimeIcon(mime: string): string {
  if (!mime) return 'insert_drive_file';
  const m = mime.toLowerCase();
  if (m.startsWith('image/')) return 'image';
  if (m === 'application/pdf') return 'picture_as_pdf';
  if (
    m.startsWith('text/') ||
    m.includes('json') ||
    m.includes('xml') ||
    m.includes('javascript') ||
    m.includes('yaml')
  )
    return 'code';
  if (m.startsWith('video/')) return 'videocam';
  if (m.startsWith('audio/')) return 'audiotrack';
  if (m.includes('zip') || m.includes('tar') || m.includes('gzip') || m.includes('compressed')) return 'folder_zip';
  return 'insert_drive_file';
}

function onView(item: ArtifactMeta) {
  previewMeta.value = item;
  previewArtifactId.value = item.id;
  previewOpen.value = true;
}

async function onDownload(item: ArtifactMeta) {
  try {
    const signed = await artifactStore.signDownload(item.id, item.version);
    window.open(artifactStore.artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
  } catch (e) {
    notify({
      type: 'negative',
      message: e instanceof Error ? e.message : t('chat.sessionArtifacts.download', '下载失败'),
    });
  }
}

async function onPreviewDownload(meta: ArtifactMeta) {
  await onDownload(meta);
}

async function onDelete(item: ArtifactMeta) {
  try {
    await artifactStore.remove(item.id);
    deletedIds.value.add(item.id);
    emit('deleted', item.id);
    notify({ type: 'positive', message: t('chat.attachmentDeleted') });
  } catch {
    notify({ type: 'negative', message: t('chat.attachmentDeleteFailed') });
  }
}
</script>
