// Container: approved — artifact detail + preview dialog.
<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="text-h6">{{ meta?.name }}</q-card-section>
      <q-card-section v-if="meta" class="app-dialog-body q-gutter-sm q-pt-none text-body2">
        <div><b>{{ t('artifact.detail.id') }}</b>{{ meta.id }}</div>
        <div><b>{{ t('artifact.detail.session') }}</b>{{ meta.session_id }}</div>
        <div>
          <b>{{ t('artifact.detail.sha256') }}</b><span class="text-caption">{{ meta.sha256 }}</span>
        </div>
        <div><b>{{ t('artifact.detail.storage') }}</b>{{ meta.storage_kind }}</div>
        <div class="artifact-detail-path">
          <b>{{ t('artifact.detail.path') }}</b>
          <span class="artifact-detail-path__value text-caption">{{ meta.storage_uri }}</span>
          <q-btn flat dense round size="sm" icon="content_copy" @click="copyPath">
            <q-tooltip>{{ t('artifact.detail.copyPath') }}</q-tooltip>
          </q-btn>
          <q-btn v-if="revealEnabled" flat dense round size="sm" icon="folder_open" @click="reveal">
            <q-tooltip>{{ t('artifact.detail.reveal') }}</q-tooltip>
          </q-btn>
        </div>
        <div><b>{{ t('artifact.detail.size') }}</b>{{ formatBytes(meta.size) }} · v{{ meta.version }}</div>
        <div v-if="versions.length > 1" class="q-mt-sm">
          <div class="text-caption text-grey-7 q-mb-xs">{{ t('artifact.detail.versionHistory') }}</div>
          <div class="row q-gutter-xs">
            <q-chip
              v-for="v in versions"
              :key="`${v.id}-v${v.version}`"
              dense
              clickable
              :color="v.version === selectedVersion ? 'primary' : undefined"
              :outline="v.version !== selectedVersion"
              @click="$emit('select-version', v)"
            >
              v{{ v.version }}
            </q-chip>
          </div>
        </div>
      </q-card-section>
      <q-card-section v-if="artifactId">
        <ArtifactPreview
          :artifact-id="artifactId"
          :version="selectedVersion"
          :show-download="true"
          @download="$emit('download', $event)"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn v-close-popup flat no-caps :label="t('common.close')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { copyToClipboard, useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import ArtifactPreview from '../../features/artifact/ArtifactPreview.vue';
import { fetchLocalRevealEnabled, revealArtifact } from '../../features/artifact/api';
import type { ArtifactMeta } from '../../features/artifact/types';
const props = defineProps<{
  open: boolean;
  meta: ArtifactMeta | null;
  artifactId: string;
  selectedVersion?: number;
  versions: ArtifactMeta[];
  formatBytes: (n: number) => string;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'select-version': [meta: ArtifactMeta];
  download: [meta: ArtifactMeta];
}>();
const $q = useQuasar();
const { t } = useI18n();
async function copyPath() {
  const uri = props.meta?.storage_uri;
  if (!uri) return;
  try {
    await copyToClipboard(uri);
    $q.notify({ type: 'positive', message: t('artifact.detail.pathCopied') });
  } catch {
    $q.notify({ type: 'negative', message: t('artifact.detail.copyFailed') });
  }
}

// M27 Phase 5：本地打开文件夹。对话框打开时探测功能开关（默认关闭 → 隐藏按钮）。
const revealEnabled = ref(false);
watch(
  () => props.open,
  async (open) => {
    if (open) revealEnabled.value = await fetchLocalRevealEnabled();
  },
);
async function reveal() {
  const id = props.meta?.id;
  if (!id) return;
  try {
    await revealArtifact(id);
    $q.notify({ type: 'positive', message: t('artifact.detail.revealed') });
  } catch {
    $q.notify({ type: 'negative', message: t('artifact.detail.revealFailed') });
  }
}
</script>

<style scoped>
.artifact-detail-path {
  display: flex;
  align-items: center;
  gap: 4px;
}

.artifact-detail-path__value {
  word-break: break-all;
  min-width: 0;
}
</style>
