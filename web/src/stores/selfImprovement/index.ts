import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  approveRun,
  closeRun,
  getOutcomeStats,
  getRun,
  listRuns,
  rejectRun,
  rollbackRun,
} from '../../features/self-improvement/api';
import type { SIOutcomeStats, SIRun, SIRunDetail, SIRunFilter } from '../../features/self-improvement/types';

// Self-improvement console store (73-self-iteration-v3, design §八):
// runs list + active detail + Learn-stage outcome stats; mutations reload the
// affected run through the returned promise's caller (page composable).
export const useSelfImprovementStore = defineStore('selfImprovement', () => {
  const runs = ref<SIRun[]>([]);
  const total = ref(0);
  const detail = ref<SIRunDetail | null>(null);
  const outcomeStats = ref<SIOutcomeStats | null>(null);
  const loading = ref(false);
  const detailLoading = ref(false);
  const statsLoading = ref(false);
  const error = ref<string | null>(null);

  function captureError(e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  }

  async function loadRuns(filter: SIRunFilter) {
    loading.value = true;
    error.value = null;
    try {
      const res = await listRuns(filter);
      runs.value = res.items;
      total.value = res.total;
    } catch (e: unknown) {
      captureError(e);
    } finally {
      loading.value = false;
    }
  }

  async function loadRun(id: string): Promise<SIRunDetail | null> {
    detailLoading.value = true;
    error.value = null;
    try {
      const res = await getRun(id);
      detail.value = res;
      return res;
    } catch (e: unknown) {
      captureError(e);
      return null;
    } finally {
      detailLoading.value = false;
    }
  }

  async function loadOutcomeStats() {
    statsLoading.value = true;
    error.value = null;
    try {
      outcomeStats.value = await getOutcomeStats();
    } catch (e: unknown) {
      captureError(e);
    } finally {
      statsLoading.value = false;
    }
  }

  function patchRow(id: string, status: SIRun['status']) {
    const idx = runs.value.findIndex((r) => r.id === id);
    if (idx >= 0) {
      runs.value[idx] = { ...runs.value[idx], status };
    }
    if (detail.value?.id === id) {
      detail.value = { ...detail.value, status };
    }
  }

  async function approve(id: string, reason?: string) {
    await approveRun(id, reason);
    patchRow(id, 'applying');
  }

  async function reject(id: string, reason: string) {
    await rejectRun(id, reason);
    patchRow(id, 'rejected');
  }

  async function rollback(id: string, reason?: string) {
    await rollbackRun(id, reason);
    patchRow(id, 'rolled_back');
  }

  async function close(id: string, reason?: string) {
    await closeRun(id, reason);
    patchRow(id, 'closed');
  }

  return {
    runs,
    total,
    detail,
    outcomeStats,
    loading,
    detailLoading,
    statsLoading,
    error,
    loadRuns,
    loadRun,
    loadOutcomeStats,
    approve,
    reject,
    rollback,
    close,
  };
});
