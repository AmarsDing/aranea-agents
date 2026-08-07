/**
 * Polling for eval run progress (ISSUE-003): while any run of the selected
 * dataset is pending/running, reload runs every `intervalMs`; stop when all
 * runs reach a terminal status (completed/failed) or the page unmounts.
 */
import { onUnmounted, watch, type Ref } from 'vue';

const TERMINAL_STATUSES = new Set(['completed', 'failed']);

/** True when any run is still in a non-terminal status. */
export function hasActiveRuns(runs: { status: string }[]): boolean {
  return runs.some((r) => !TERMINAL_STATUSES.has(r.status));
}

/**
 * Watch run statuses and poll `reload` while active. Overlapping polls are
 * skipped (a slow reload never stacks). The timer is cleaned up on unmount.
 */
export function useEvalRunPolling(
  runs: Ref<{ status: string }[]>,
  reload: () => Promise<void> | void,
  intervalMs = 3000,
) {
  let timer: ReturnType<typeof setInterval> | null = null;
  let inFlight = false;

  function stop() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  function start() {
    if (timer !== null) return;
    timer = setInterval(() => {
      if (inFlight) return;
      inFlight = true;
      Promise.resolve(reload())
        .catch(() => {})
        .finally(() => {
          inFlight = false;
        });
    }, intervalMs);
  }

  watch(
    () => hasActiveRuns(runs.value),
    (active) => {
      if (active) start();
      else stop();
    },
    { immediate: true },
  );

  onUnmounted(stop);

  return { stopPolling: stop };
}
