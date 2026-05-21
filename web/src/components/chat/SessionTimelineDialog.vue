<template>
  <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
    <q-card class="session-trace session-timeline-dialog">
      <q-card-section class="session-trace__header row items-start justify-between no-wrap">
        <div class="col">
          <div class="session-trace__eyebrow">Session Inspector</div>
          <div class="session-trace__title">{{ sessionTitle || "Untitled session" }}</div>
          <q-tabs v-model="activeTab" dense align="left" class="q-mt-sm" active-color="primary">
            <q-tab name="trace" label="历史 Trace" />
            <q-tab name="events" label="实时 Envelope" />
          </q-tabs>
        </div>
        <q-btn flat round icon="close" aria-label="Close" class="sessions-icon-btn" @click="dialogOpen = false" />
      </q-card-section>

      <q-separator class="sessions-sep" />

      <q-tab-panels v-model="activeTab" animated class="session-trace__panels">
        <q-tab-panel name="trace" class="q-pa-none">
          <q-card-section class="session-trace__stats">
            <SessionTimelineStats :stats="stats" />
          </q-card-section>
          <q-separator class="sessions-sep" />
          <q-card-section class="session-trace__body session-timeline">
            <div v-if="loading" class="session-timeline__state column items-center justify-center">
              <q-spinner color="primary" size="32px" />
              <div class="q-mt-sm sessions-muted">Loading session trace…</div>
            </div>
            <div v-else-if="error" class="session-timeline__state session-timeline__state--error">
              {{ error }}
            </div>
            <div v-else-if="!timeline?.items.length" class="session-timeline__state">
              <q-icon name="timeline" size="36px" class="sessions-muted" />
              <div class="q-mt-sm sessions-muted">No trace events yet.</div>
            </div>
            <div v-else class="session-timeline__rail">
              <SessionTimelineEntry
                v-for="item in timeline.items"
                :key="`${item.kind}:${item.id}`"
                :item="item"
              />
            </div>
          </q-card-section>
        </q-tab-panel>

        <q-tab-panel name="events" class="q-pa-md">
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
import { computed, ref, watch } from "vue";
import type { SessionTimeline } from "../../features/session/api";
import type { ChatEventInspectorStreamDeps } from "../../features/chat/composables/useChatEventInspector";
import { useSessionStore } from "../../stores/session/index";
import SessionEventInspectorPanel from "./SessionEventInspectorPanel.vue";
import SessionTimelineEntry from "../sessions/SessionTimelineEntry.vue";
import SessionTimelineStats from "../sessions/SessionTimelineStats.vue";
import { buildTimelineStats } from "../sessions/sessionTimelineUi";

export type SessionInspectorTab = "trace" | "events";

const props = defineProps<{
  modelValue: boolean;
  sessionId?: string | null;
  sessionTitle?: string;
  initialTab?: SessionInspectorTab;
  streamDeps?: ChatEventInspectorStreamDeps;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const sessionStore = useSessionStore();
const timeline = ref<SessionTimeline | null>(null);
const loading = ref(false);
const error = ref("");
const activeTab = ref<SessionInspectorTab>(props.initialTab ?? "trace");

const dialogOpen = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});

const stats = computed(() => buildTimelineStats(timeline.value?.summary));

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

watch(
  () => [props.modelValue, props.sessionId, activeTab.value] as const,
  async ([open, sessionId, tab]) => {
    if (!open || !sessionId || tab !== "trace") return;
    loading.value = true;
    error.value = "";
    try {
      timeline.value = await sessionStore.fetchTimeline(sessionId);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to load session trace.";
      timeline.value = null;
    } finally {
      loading.value = false;
    }
  },
  { immediate: true },
);
</script>

<style scoped>
.session-trace {
  width: min(960px, 92vw);
  max-height: 88vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 24px;
  background: var(--glass-elevated);
  color: var(--color-text-primary);
  border: 1px solid var(--glass-border);
  backdrop-filter: blur(var(--glass-blur-elevated));
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated));
  box-shadow: var(--glass-inner-highlight);
}

.session-trace__header {
  padding: 22px 24px 12px;
}

.session-trace__eyebrow {
  color: var(--color-text-secondary);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.session-trace__title {
  margin-top: 4px;
  color: var(--color-text-primary);
  font-size: 22px;
  font-weight: 850;
  line-height: 1.35;
}

.session-trace__panels {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}

.session-trace__stats {
  flex: 0 0 auto;
  padding: 14px 20px;
}

.session-trace__body {
  flex: 1 1 auto;
  min-height: 280px;
  overflow: auto;
  padding: 8px 20px 24px;
}
</style>
