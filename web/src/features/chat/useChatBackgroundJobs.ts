import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue';
import { listChatBackgroundJobs } from './api';
import type { ChatBackgroundJobRow } from './types';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from './globalWsHub';
import type { Envelope } from './envelope';

// Backoff steps: 5s → 10s → 15s → 30s (max)
const BACKOFF_STEPS = [5_000, 10_000, 15_000, 30_000];

function isBackgroundJobRefreshEnvelope(env: Envelope, sessionId?: string): boolean {
  const md = env.metadata as Record<string, unknown> | undefined;
  if (!md?.background_job_refresh) return false;
  const sid = (env.session_id ?? '').trim();
  if (sessionId && sid && sid !== sessionId.trim()) return false;
  return true;
}

export function useChatBackgroundJobs(
  sessionId: Ref<string | undefined>,
  agentId: Ref<string | undefined>,
  refreshNonce?: Ref<number | undefined>,
) {
  const loading = ref(false);
  const error = ref('');
  const rows = ref<ChatBackgroundJobRow[]>([]);
  let hubId: string | null = null;
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let backoffIndex = 0;
  let loadInFlight = false;

  async function load() {
    if (loadInFlight) return; // dedup: skip if previous load still in-flight
    const sid = sessionId.value?.trim();
    const aid = agentId.value?.trim();
    if (!sid && !aid) {
      rows.value = [];
      return;
    }
    loadInFlight = true;
    loading.value = true;
    error.value = '';
    try {
      rows.value = await listChatBackgroundJobs({
        sessionId: sid,
        agentId: !sid ? aid : undefined,
        limit: 50,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'load failed';
    } finally {
      loading.value = false;
      loadInFlight = false;
    }
  }

  function currentInterval() {
    return BACKOFF_STEPS[Math.min(backoffIndex, BACKOFF_STEPS.length - 1)];
  }

  function scheduleNextPoll() {
    cancelPoll();
    pollTimer = setTimeout(() => {
      pollTimer = null;
      void load().then(() => {
        // After each poll, re-evaluate: still running → schedule next with backoff
        if (runningCount() > 0) {
          backoffIndex = Math.min(backoffIndex + 1, BACKOFF_STEPS.length - 1);
          scheduleNextPoll();
        }
        // All done → poll stops naturally (no reschedule)
      });
    }, currentInterval());
  }

  function cancelPoll() {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function resetBackoffAndStartPoll() {
    backoffIndex = 0;
    if (runningCount() > 0) {
      scheduleNextPoll();
    }
  }

  // When rows change, decide whether to start/stop polling
  watch(rows, () => {
    if (runningCount() > 0) {
      // Only start if not already polling
      if (!pollTimer) {
        backoffIndex = 0;
        scheduleNextPoll();
      }
    } else {
      cancelPoll();
    }
  });

  watch(
    [sessionId, agentId],
    () => {
      backoffIndex = 0;
      void load();
    },
    { immediate: false },
  );

  if (refreshNonce) {
    watch(refreshNonce, () => void load());
  }

  onMounted(() => {
    void load();
    hubId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onEnvelope: (env) => {
        if (env.channel !== 'chat') return;
        if (isBackgroundJobRefreshEnvelope(env, sessionId.value)) {
          // WS event arrived → immediate load + reset backoff
          cancelPoll();
          void load().then(() => resetBackoffAndStartPoll());
        }
      },
    });
  });

  onUnmounted(() => {
    cancelPoll();
    if (hubId) {
      releaseGlobalWsConsumer(hubId);
      hubId = null;
    }
  });

  const ACTIVE_STATUSES = ['running', 'accepted', 'async_queued', 'queued', 'interactive', 'escalating', 'durable'];

  const runningCount = () => rows.value.filter((r) => ACTIVE_STATUSES.includes(r.status)).length;

  return { loading, error, rows, load, runningCount };
}
