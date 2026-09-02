<!-- web/src/components/artifact/SessionArtifactsDrawer.vue
  会话产物抽屉（P0 会话产物点击查看，2026-09-01）。
  在会话页内直接列出当前会话产物，点击进入预览弹窗；
  底部「管理全部产物」跳转产物管理页（带会话筛选）。 -->
<template>
  <q-dialog :model-value="open" position="right" full-height @update:model-value="onToggle">
    <q-card class="session-artifacts-drawer column no-wrap">
      <q-card-section class="row items-center q-py-sm">
        <q-icon name="inventory_2" size="sm" class="q-mr-sm" />
        <div class="text-subtitle1">{{ t('artifact.drawer.title') }}</div>
        <q-chip v-if="artifacts.length > 0" dense size="sm" color="accent" text-color="white" class="q-ml-sm">
          {{ artifacts.length }}
        </q-chip>
        <q-space />
        <q-btn v-close-popup flat round dense icon="close" :aria-label="t('artifact.drawer.close')" />
      </q-card-section>
      <q-separator />

      <q-card-section v-if="loading" class="col flex flex-center">
        <q-spinner-dots size="32px" color="accent" />
      </q-card-section>
      <q-card-section v-else-if="artifacts.length === 0" class="col flex flex-center text-grey-7">
        <div class="text-center">
          <q-icon name="inventory_2" size="2.5em" class="q-mb-sm" />
          <div>{{ t('artifact.drawer.empty') }}</div>
        </div>
      </q-card-section>
      <q-scroll-area v-else class="col">
        <q-list separator>
          <q-item
            v-for="art in artifacts"
            :key="`${art.id}:v${art.version}`"
            clickable
            class="session-artifacts-drawer__item"
            @click="emit('preview', art)"
          >
            <q-item-section avatar>
              <q-icon :name="iconOf(art.mime_type)" size="sm" />
            </q-item-section>
            <q-item-section>
              <q-item-label class="ellipsis">{{ art.name }}</q-item-label>
              <q-item-label caption>
                {{ formatBytes(art.size) }} · v{{ art.version }} · {{ formatDate(art.created_at) }}
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-btn
                flat
                round
                dense
                icon="download"
                size="sm"
                :aria-label="t('artifact.preview.download')"
                @click.stop="emit('download', art)"
              >
                <q-tooltip>{{ t('artifact.preview.download') }}</q-tooltip>
              </q-btn>
            </q-item-section>
          </q-item>
        </q-list>
      </q-scroll-area>

      <q-separator />
      <q-card-actions align="right" class="q-pa-sm">
        <q-btn flat dense no-caps icon="open_in_new" :label="t('artifact.drawer.manage')" @click="emit('manage')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { ArtifactMeta } from '../../features/artifact/types';
import { formatBytes, formatDate } from '../../shared/format';

defineProps<{
  open: boolean;
  artifacts: ArtifactMeta[];
  loading: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  preview: [meta: ArtifactMeta];
  download: [meta: ArtifactMeta];
  manage: [];
}>();

const { t } = useI18n();

function onToggle(open: boolean) {
  emit('update:open', open);
}

/** MIME → 图标（纯展示映射，与 artifact 域预览分类一致）。 */
function iconOf(mime: string): string {
  const m = (mime || '').toLowerCase();
  if (m.startsWith('image/')) return 'image';
  if (m.startsWith('audio/')) return 'audiotrack';
  if (m.startsWith('video/')) return 'movie';
  if (m === 'application/pdf') return 'picture_as_pdf';
  if (m.startsWith('text/') || m === 'application/json' || m === 'application/xml') return 'article';
  if (m.includes('zip') || m.includes('compressed') || m.includes('tar')) return 'folder_zip';
  return 'description';
}
</script>

<style scoped lang="sass">
.session-artifacts-drawer
  width: min(380px, 92vw)

.session-artifacts-drawer__item
  min-height: 56px
</style>
