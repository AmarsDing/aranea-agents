<template>
  <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
    <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog session-timeline-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__subtitle">Session Inspector</div>
          <div class="app-glass-dialog__title">{{ sessionTitle || 'Untitled session' }}</div>
          <q-tabs v-model="activeTab" dense align="left" class="q-mt-sm" active-color="accent">
            <q-tab name="trace" label="历史 Trace" />
            <q-tab name="events" label="实时 Envelope" />
          </q-tabs>
        </div>
        <q-btn flat round icon="close" aria-label="Close" class="sessions-icon-btn" @click="dialogOpen = false" />
      </q-card-section>

      <q-separator class="sessions-sep" />

      <q-tab-panels v-model="activeTab" animated class="session-timeline-dialog__panels app-glass-dialog__scroll">
        <q-tab-panel name="trace" class="q-pa-none">
          <q-card-section class="session-timeline-dialog__stats">
            <SessionTimelineStats :stats="stats" />
          </q-card-section>
          <q-separator class="sessions-sep" />
          <q-card-section class="app-dialog-body app-glass-dialog__body session-timeline">
            <div v-if="timelineLoading" class="session-timeline__state column items-center justify-center">
              <q-spinner color="accent" size="32px" />
              <div class="q-mt-sm sessions-muted">Loading session trace…</div>
            </div>
            <div v-else-if="timelineError" class="session-timeline__state session-timeline__state--error">
              {{ timelineError }}
            </div>
            <div v-else-if="!timeline?.items.length" class="session-timeline__state">
              <q-icon name="timeline" size="36px" class="sessions-muted" />
              <div class="q-mt-sm sessions-muted">No trace events yet.</div>
            </div>
            <div v-else class="session-timeline__rail">
              <SessionTimelineEntry v-for="item in timeline.items" :key="`${item.kind}:${item.id}`" :item="item" />
            </div>
          </q-card-section>
        </q-tab-panel>

        <q-tab-panel name="events" class="q-pa-md app-glass-dialog__body">
          <SessionEventInspectorPanel
            v-if="sessionId"
            :session-id="sessionId"
            :active="dialogOpen && activeTab === 'events'"
            :stream-deps="props.streamDeps"
          />
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { ChatEventInspectorStreamDeps } from '../../features/chat/composables/useChatEventInspector';
import type { SessionTimeline } from '../../features/session/types';
import { buildTimelineStats } from '../../features/session/timelineHelpers';
import SessionEventInspectorPanel from './SessionEventInspectorPanel.vue';
import SessionTimelineEntry from '../sessions/SessionTimelineEntry.vue';
import SessionTimelineStats from '../sessions/SessionTimelineStats.vue';

export type SessionInspectorTab = 'trace' | 'events';

const props = defineProps<{
  modelValue: boolean;
  sessionId?: string | null;
  sessionTitle?: string;
  initialTab?: SessionInspectorTab;
  streamDeps?: ChatEventInspectorStreamDeps;
  timeline?: SessionTimeline | null;
  timelineLoading?: boolean;
  timelineError?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'refresh-trace': [];
}>();

const activeTab = ref<SessionInspectorTab>(props.initialTab ?? 'trace');

const dialogOpen = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
});

const stats = computed(() => buildTimelineStats(props.timeline?.summary));

watch(
  () => props.initialTab,
  (tab) => {
    if (tab) activeTab.value = tab;
  },
);

watch(
  () => props.modelValue,
  (open) => {
    if (open && props.initialTab) activeTab.value = props.initialTab;
  },
);
</script>

<style scoped>
.session-timeline-dialog {
  display: flex;
  flex-direction: column;
  max-height: min(88vh, 920px);
}

.session-timeline-dialog__panels {
  flex: 1 1 auto;
  min-height: 0;
}

.session-timeline-dialog__stats {
  flex: 0 0 auto;
  padding: var(--space-3) var(--space-5);
}
</style>
