<template>
  <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
    <q-card class="session-trace session-timeline-dialog">
      <q-card-section class="session-trace__header row items-start justify-between no-wrap">
        <div>
          <div class="session-trace__eyebrow">Session Trace</div>
          <div class="session-trace__title">{{ sessionTitle || "Untitled session" }}</div>
          <div class="session-trace__subtitle">
            {{ timeline?.summary.total ?? 0 }} events · conversation + tools + skills + MCP
          </div>
        </div>
        <q-btn flat round icon="close" aria-label="Close" class="sessions-icon-btn" @click="dialogOpen = false" />
      </q-card-section>

      <q-separator class="sessions-sep" />

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
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { SessionTimeline } from "../../features/session/api";
import { useSessionStore } from "../../stores/session/index";
import SessionTimelineEntry from "../sessions/SessionTimelineEntry.vue";
import SessionTimelineStats from "../sessions/SessionTimelineStats.vue";
import { buildTimelineStats } from "../sessions/sessionTimelineUi";

const props = defineProps<{
  modelValue: boolean;
  sessionId?: string | null;
  sessionTitle?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const sessionStore = useSessionStore();
const timeline = ref<SessionTimeline | null>(null);
const loading = ref(false);
const error = ref("");

const dialogOpen = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value)
});

const stats = computed(() => buildTimelineStats(timeline.value?.summary));

watch(
  () => [props.modelValue, props.sessionId] as const,
  async ([open, sessionId]) => {
    if (!open || !sessionId) return;
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
  { immediate: true }
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
  padding: 22px 24px 16px;
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

.session-trace__subtitle {
  margin-top: 4px;
  color: var(--color-text-secondary);
  font-size: 13px;
}

.session-trace__stats {
  flex: 0 0 auto;
  padding: 14px 20px;
}

.session-trace__body {
  flex: 1 1 auto;
  min-height: 360px;
  overflow: auto;
  padding: 8px 20px 24px;
}
</style>
