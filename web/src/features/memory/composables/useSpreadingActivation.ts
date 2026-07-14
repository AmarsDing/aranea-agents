// Phase E Task E-9: 扩散激活前端 composable
// 负责调用 store.fetchSpreadingActivation 并管理 loading / error / 结果状态，
// 供 MemoryGraphExplorer.vue 在"扩散激活"按钮触发后消费。
import { ref } from 'vue';
import type { SpreadingActivationResponse } from '../types';
import { useMemoryApi } from './useMemoryApi';

export interface SpreadingActivationParams {
  hops?: number;
  top_k?: number;
}

export function useSpreadingActivation() {
  const { getSpreadingActivation } = useMemoryApi();
  const result = ref<SpreadingActivationResponse | null>(null);
  const loadingActivation = ref(false);
  const activationError = ref('');

  async function runSpreadingActivation(centerID: string, params: SpreadingActivationParams = {}) {
    if (!centerID) return;
    loadingActivation.value = true;
    activationError.value = '';
    try {
      result.value = await getSpreadingActivation(centerID, {
        hops: params.hops ?? 3,
        top_k: params.top_k ?? 12,
      });
    } catch (err) {
      result.value = null;
      activationError.value = err instanceof Error ? err.message : 'Spreading activation failed';
    } finally {
      loadingActivation.value = false;
    }
  }

  function resetActivation() {
    result.value = null;
    activationError.value = '';
  }

  return { result, loadingActivation, activationError, runSpreadingActivation, resetActivation };
}
