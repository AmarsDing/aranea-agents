<!-- web/src/components/artifact/SessionArtifactsDrawer.vue
  会话产物抽屉（P0 会话产物点击查看，2026-09-01）。
  在会话页内直接列出当前会话产物，点击进入预览弹窗；
  底部「管理全部产物」跳转产物管理页（带会话筛选）。
  2026-11-11：迁入 AppDialog 抽屉形态（position=right），滚动交由弹框 body。 -->
<template>
  <AppDialog
    :model-value="open"
    position="right"
    size="sm"
    icon="inventory_2"
    :title="t('artifact.drawer.title')"
    @update:model-value="onToggle"
  >
    <div v-if="artifacts.length > 0" class="session-artifacts-drawer__count">
      <q-chip dense size="sm" color="accent" text-color="white">{{ artifacts.length }}</q-chip>
    </div>

    <div v-if="loading" class="session-artifacts-drawer__state">
      <q-spinner-dots size="32px" color="accent" />
    </div>
    <div v-else-if="artifacts.length === 0" class="session-artifacts-drawer__state session-artifacts-drawer__state--empty">
      <q-icon name="inventory_2" size="2.5em" />
      <div>{{ t('artifact.drawer.empty') }}</div>
    </div>
    <q-list v-else separator>
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

    <template #actions>
      <q-btn flat dense no-caps icon="open_in_new" :label="t('artifact.drawer.manage')" @click="emit('manage')" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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
.session-artifacts-drawer__count
  margin-bottom: var(--nt-space-2)

.session-artifacts-drawer__state
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  gap: var(--nt-space-2)
  min-height: 240px
  color: var(--color-text-tertiary)

.session-artifacts-drawer__item
  min-height: 56px
</style>
