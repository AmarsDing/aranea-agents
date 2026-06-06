// TECH-DEBT: bypass store — memory admin panels fetch ad-hoc data; migrate to Store when shared state needed
import {
  compositeSearchMemories,
  debugMemoryRecall,
  getMemoryNeighborhood,
  getMemoryPlatformSettings,
  listMemoryDeadLetters,
  updateMemoryPlatformSettings,
} from '../api';

export function useMemoryApi() {
  return {
    compositeSearchMemories,
    debugMemoryRecall,
    getMemoryNeighborhood,
    getMemoryPlatformSettings,
    listMemoryDeadLetters,
    updateMemoryPlatformSettings,
  };
}
