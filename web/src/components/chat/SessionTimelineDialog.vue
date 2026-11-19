<template>
  <AppDialog
    v-model="dialogOpen"
    :title="sessionTitle || 'Untitled session'"
    subtitle="Session Inspector"
    size="lg"
  >
    <q-tabs v-model="activeTab" dense align="left" class="q-mb-md" active-color="accent">
      <q-tab name="trace" label="历史 Trace" />
      <q-tab name="events" label="实时 Envelope" />
    </q-tabs>
    <q-tab-panels v-model="activeTab" animated class="session-timeline-dialog__panels">
      <q-tab-panel name="trace" class="q-pa-none">
        <div class="session-timeline-dialog__stats">
          <SessionTimelineStats :stats="stats" />
        </div>
        <q-separator class="sessions-sep" />
        <div class="session-timeline">
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
        </div>
      </q-tab-panel>

      <q-tab-panel name="events" class="q-pa-none">
        <SessionEventInspectorPanel
          v-if="sessionId"
          :session-id="sessionId"
          :active="dialogOpen && activeTab === 'events'"
          :stream-deps="props.streamDeps"
        />
      </q-tab-panel>
    </q-tab-panels>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { ChatEventInspectorStreamDeps } from '../../features/chat/composables/useChatEventInspector';
import type { SessionTimeline } from '../../features/session/types';
import { buildTimelineStats } from '../../features/session/timelineHelpers';
import AppDialog from '../common/AppDialog.vue';
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

// 用户切换到「历史 Trace」Tab 时触发刷新，确保显示最新数据。
// 排除 dialog 打开时的初始 Tab 设置（由 modelValue watch 处理）和
// 父组件通过 initialTab 同步的情况（activeTab 由 watch 驱动，会触发本 watch）。
// 通过 dialog 已 open 且 activeTab 变为 'trace' 来识别用户手动切换。
watch(activeTab, (tab, prev) => {
  if (dialogOpen.value && tab === 'trace' && prev !== undefined) {
    emit('refresh-trace');
  }
});
</script>

<style scoped>
.session-timeline-dialog__panels {
  flex: 1 1 auto;
  min-height: 0;
}

.session-timeline-dialog__stats {
  flex: 0 0 auto;
  padding: var(--space-3) var(--space-5);
}
</style>
