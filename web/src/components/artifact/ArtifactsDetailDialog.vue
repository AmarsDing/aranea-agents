<template>
  <AppDialog :model-value="open" :title="meta?.name ?? ''" size="md" @update:model-value="$emit('update:open', $event)">
    <div v-if="meta" class="q-gutter-sm text-body2">
      <div>
        <b>{{ t('artifact.detail.id') }}</b
        >{{ meta.id }}
      </div>
      <div>
        <b>{{ t('artifact.detail.session') }}</b
        >{{ meta.session_id }}
      </div>
      <div>
        <b>{{ t('artifact.detail.sha256') }}</b
        ><span class="text-caption">{{ meta.sha256 }}</span>
      </div>
      <div>
        <b>{{ t('artifact.detail.storage') }}</b
        >{{ meta.storage_kind }}
      </div>
      <div class="artifact-detail-path">
        <b>{{ t('artifact.detail.path') }}</b>
        <span class="artifact-detail-path__value text-caption">{{ meta.storage_uri }}</span>
        <q-btn flat dense round size="sm" icon="content_copy" @click="copyPath">
          <q-tooltip>{{ t('artifact.detail.copyPath') }}</q-tooltip>
        </q-btn>
        <q-btn v-if="revealEnabled" flat dense round size="sm" icon="folder_open" @click="$emit('reveal', meta)">
          <q-tooltip>{{ t('artifact.detail.reveal') }}</q-tooltip>
        </q-btn>
      </div>
      <div>
        <b>{{ t('artifact.detail.size') }}</b
        >{{ formatBytes(meta.size) }} · v{{ meta.version }}
      </div>
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
        <q-btn
          flat
          dense
          no-caps
          color="negative"
          icon="delete_outline"
          :label="t('artifact.detail.deleteVersion')"
          class="q-mt-xs"
          @click="$emit('delete-version', meta)"
        />
      </div>
    </div>
    <div v-if="artifactId" class="q-mt-md">
      <ArtifactPreview
        :artifact-id="artifactId"
        :version="selectedVersion"
        :show-download="true"
        @download="$emit('download', $event)"
      />
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('common.close')" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { copyToClipboard, useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import ArtifactPreview from '../../features/artifact/ArtifactPreview.vue';
import type { ArtifactMeta } from '../../features/artifact/types';
const props = defineProps<{
  open: boolean;
  meta: ArtifactMeta | null;
  artifactId: string;
  selectedVersion?: number;
  versions: ArtifactMeta[];
  formatBytes: (n: number) => string;
  /** 本地 reveal 开关（由 Page 经 composable 探测注入）。 */
  revealEnabled: boolean;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'select-version': [meta: ArtifactMeta];
  'delete-version': [meta: ArtifactMeta];
  download: [meta: ArtifactMeta];
  reveal: [meta: ArtifactMeta];
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
</script>

<style scoped>
.artifact-detail-path {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* 长路径换行时保持「路径：」标签完整（否则被挤压成逐字换行） */
.artifact-detail-path > b {
  flex: none;
  white-space: nowrap;
  align-self: flex-start;
}

.artifact-detail-path__value {
  word-break: break-all;
  min-width: 0;
}
</style>
