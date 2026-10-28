<template>
  <aside class="task-artifacts column no-wrap" :aria-label="t('taskWorkspace.artifacts')" tabindex="-1">
    <header class="task-artifacts__header row items-center no-wrap q-gutter-xs">
      <q-icon name="inventory_2" size="20px" color="accent" />
      <span class="text-weight-medium">{{ t('taskWorkspace.artifacts') }}</span>
      <q-space />
      <q-btn
        flat
        round
        dense
        :icon="readingExpanded ? 'close_fullscreen' : 'open_in_full'"
        :aria-label="t(readingExpanded ? 'taskWorkspace.collapseReading' : 'taskWorkspace.expandReading')"
        @click="emit('toggle-reading')"
      />
      <q-btn
        flat
        round
        dense
        icon="refresh"
        :disable="!sessionId || loading"
        :aria-label="t('taskWorkspace.refresh')"
        @click="emit('refresh')"
      />
      <q-btn flat round dense icon="close" :aria-label="t('taskWorkspace.close')" @click="emit('close')" />
    </header>
    <div v-if="!target" class="task-artifacts__summary text-caption" role="status">
      {{
        total == null
          ? t('taskWorkspace.loaded', { count: artifacts.length })
          : t('taskWorkspace.count', { loaded: artifacts.length, total })
      }}
    </div>
    <template v-if="target">
      <div class="row items-center q-px-sm q-py-xs q-gutter-xs">
        <q-btn flat dense no-caps icon="arrow_back" :label="t('taskWorkspace.backToList')" @click="emit('back')" />
        <q-space />
        <q-btn-dropdown
          flat
          dense
          no-caps
          :loading="versionsLoading"
          :label="`${t('taskWorkspace.versions')} · v${target.version ?? '—'}`"
        >
          <q-list>
            <q-item
              v-for="version in versions"
              :key="`${version.id}:${version.version}`"
              v-close-popup
              clickable
              :active="version.id === target.id && version.version === target.version"
              :aria-current="version.id === target.id && version.version === target.version ? 'true' : undefined"
              @click="emit('preview', version)"
            >
              <q-item-section>{{ version.name }} · v{{ version.version }}</q-item-section>
            </q-item>
            <q-item v-if="versionsError || !versions.length" clickable @click="emit('reload-versions')">
              <q-item-section>{{ t('taskWorkspace.retry') }}</q-item-section>
            </q-item>
          </q-list>
        </q-btn-dropdown>
      </div>
      <div ref="bodyElement" class="task-artifacts__body" @scroll="saveScroll">
        <ArtifactPreview
          v-if="previewActive !== false"
          :key="`${target.id}:${target.version}`"
          :artifact-id="target.id"
          :version="target.version"
          :expected-session-id="sessionId"
          show-download
          allow-quote
          @download="emit('download', $event)"
          @quote="emit('quote', $event)"
          @ready="restoreScroll"
        />
      </div>
    </template>
    <div v-else-if="!sessionId" class="task-artifacts__empty">
      <q-icon name="chat_bubble_outline" size="36px" />
      <p>{{ t('taskWorkspace.noSession') }}</p>
    </div>
    <div v-else ref="bodyElement" class="task-artifacts__body" @scroll="saveScroll">
      <div v-if="error" class="app-banner-warning q-pa-sm q-mb-sm" role="alert">
        {{ t('taskWorkspace.loadFailed') }}
        <q-btn flat dense :label="t('taskWorkspace.retry')" @click="emit('refresh')" />
      </div>
      <div v-if="loading && !artifacts.length" class="task-artifacts__empty">
        <q-spinner-dots color="accent" size="32px" />
      </div>
      <div v-else-if="!error && !artifacts.length" class="task-artifacts__empty">
        <q-icon name="inventory_2" size="36px" />
        <p>{{ t('taskWorkspace.empty') }}</p>
      </div>
      <q-list v-else separator>
        <q-item v-for="item in artifacts" :key="`${item.id}:${item.version}`" clickable @click="emit('preview', item)">
          <q-item-section avatar><q-icon name="description" color="accent" /></q-item-section>
          <q-item-section>
            <q-item-label class="task-artifacts__name">{{ item.name }}</q-item-label>
            <q-item-label caption>v{{ item.version }} · {{ formatBytes(item.size) }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-btn
              flat
              round
              dense
              icon="download"
              :aria-label="t('artifact.preview.download')"
              @click.stop="emit('download', item)"
            />
          </q-item-section>
        </q-item>
      </q-list>
      <q-btn
        v-if="hasMore && !loading"
        class="full-width q-mt-sm"
        flat
        no-caps
        :loading="loadingMore"
        :label="t('taskWorkspace.loadMore')"
        @click="emit('load-more')"
      />
    </div>
    <footer v-if="canManage !== false" class="task-artifacts__footer">
      <q-btn
        flat
        dense
        no-caps
        icon="open_in_new"
        :disable="!sessionId"
        :label="t('taskWorkspace.manage')"
        @click="emit('manage')"
      />
    </footer>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { nextTick, ref, watch } from 'vue';
import ArtifactPreview from '../../features/artifact/ArtifactPreview.vue';
import type { ArtifactMeta, ArtifactPreviewTarget, ArtifactTextQuote } from '../../features/artifact/types';
import { formatBytes } from '../../shared/format';

const props = withDefaults(
  defineProps<{
    sessionId?: string;
    artifacts: ArtifactMeta[];
    total: number | null;
    loading: boolean;
    loadingMore: boolean;
    hasMore: boolean;
    error: string;
    target: ArtifactPreviewTarget | null;
    previewActive?: boolean;
    readingExpanded?: boolean;
    scrollTop?: number;
    canManage?: boolean;
    versions: ArtifactMeta[];
    versionsLoading: boolean;
    versionsError: string;
  }>(),
  { sessionId: undefined, scrollTop: 0, previewActive: true, canManage: true },
);
const emit = defineEmits<{
  'scroll-position': [top: number];
  refresh: [];
  'load-more': [];
  close: [];
  back: [];
  manage: [];
  'reload-versions': [];
  'toggle-reading': [];
  preview: [meta: ArtifactMeta];
  download: [meta: ArtifactMeta];
  quote: [quote: ArtifactTextQuote];
}>();
const { t } = useI18n();
const bodyElement = ref<HTMLElement | null>(null);
function saveScroll(event: Event) {
  emit('scroll-position', (event.target as HTMLElement).scrollTop);
}
function restoreScroll() {
  void nextTick(() => {
    if (bodyElement.value) bodyElement.value.scrollTop = props.scrollTop ?? 0;
  });
}
watch(() => [props.target?.id, props.target?.version, props.artifacts.length], restoreScroll, {
  immediate: true,
  flush: 'post',
});
</script>

<style scoped lang="sass">
.task-artifacts
  min-width: 0
  height: 100%
  border: 1px solid var(--glass-border)
  border-radius: var(--radius-lg, 16px)
  background: var(--glass-surface)
  overflow: hidden
.task-artifacts__header, .task-artifacts__footer
  padding: var(--space-3)
  flex: 0 0 auto
.task-artifacts__header
  border-bottom: 1px solid var(--glass-border)
.task-artifacts__summary
  padding: var(--space-2) var(--space-3)
  color: var(--color-text-secondary)
.task-artifacts__body
  flex: 1
  min-height: 0
  overflow: auto
  padding: var(--space-2)
.task-artifacts__empty
  flex: 1
  padding: var(--space-4)
  color: var(--color-text-secondary)
  text-align: center
.task-artifacts__name
  overflow-wrap: anywhere
.task-artifacts__footer
  margin-top: auto
  border-top: 1px solid var(--glass-border)
</style>
