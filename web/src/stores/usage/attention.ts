import { defineStore } from 'pinia';
import { ref, shallowRef } from 'vue';
import { listMonitorAlertRules, listAlertMetrics, getRunnerMetrics } from '../../features/monitor/api';
import { listTaskDeadLetters } from '../../features/teams/api';
import { listDecisionRecords } from '../../features/decision/api';
import { listCronTaskRuns } from '../../features/cron/api';

type Snapshot = {
  alerts: Awaited<ReturnType<typeof listMonitorAlertRules>>;
  metrics: Awaited<ReturnType<typeof listAlertMetrics>>;
  runner: Awaited<ReturnType<typeof getRunnerMetrics>>;
  dead: Awaited<ReturnType<typeof listTaskDeadLetters>>;
  decisions: Awaited<ReturnType<typeof listDecisionRecords>>;
  cron: Awaited<ReturnType<typeof listCronTaskRuns>>;
};
export type AttentionSource = keyof Snapshot;

/** Existing read APIs, one snapshot owner. Failed reads retain the last successful source. */
export const useAttentionStore = defineStore('overview-attention', () => {
  const snapshot = shallowRef<Partial<Snapshot>>({});
  const failedSources = ref<AttentionSource[]>([]);
  const loading = ref(false);
  const initialized = ref(false);
  const updatedAt = ref('');
  let inFlight: Promise<void> | null = null;

  function refresh(): Promise<void> {
    if (inFlight) return inFlight;
    loading.value = true;
    const next = { ...snapshot.value };
    const failures: AttentionSource[] = [];
    async function read<K extends AttentionSource>(key: K, fetcher: () => Promise<Snapshot[K]>) {
      try {
        next[key] = await fetcher();
      } catch {
        failures.push(key);
      }
    }
    inFlight = Promise.all([
      read('alerts', listMonitorAlertRules),
      read('metrics', listAlertMetrics),
      read('runner', () => getRunnerMetrics(24 * 60)),
      read('dead', () => listTaskDeadLetters({ status: 'pending', limit: 20 })),
      read('decisions', () => listDecisionRecords({ category: 'hitl_approval', pageSize: 50 })),
      read('cron', () => listCronTaskRuns({ status: 'failure', limit: 20 })),
    ])
      .then(() => {
        snapshot.value = next;
        failedSources.value = failures;
        initialized.value = true;
        if (!failures.length) updatedAt.value = new Date().toISOString();
      })
      .finally(() => {
        loading.value = false;
        inFlight = null;
      });
    return inFlight;
  }

  return { snapshot, failedSources, loading, initialized, updatedAt, refresh };
});
