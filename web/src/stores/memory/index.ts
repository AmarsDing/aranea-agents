import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listL0Snapshots,
  listMemoryFacts,
  listMemoryEntities,
  type L0AssemblySnapshot,
  type MemoryFact,
  type MemoryEntity
} from "../../features/memory/api";

export const useMemoryStore = defineStore("memory", () => {
  const snapshots = ref<L0AssemblySnapshot[]>([]);
  const facts = ref<MemoryFact[]>([]);
  const entities = ref<MemoryEntity[]>([]);
  const loading = ref(false);

  async function loadSnapshots(sessionID: string, limit = 20) {
    loading.value = true;
    try {
      snapshots.value = await listL0Snapshots(sessionID, limit);
    } finally {
      loading.value = false;
    }
  }

  async function loadFacts(query?: Parameters<typeof listMemoryFacts>[0]) {
    const result = await listMemoryFacts(query);
    facts.value = result.items ?? [];
    return result;
  }

  async function loadEntities(query?: Parameters<typeof listMemoryEntities>[0]) {
    const result = await listMemoryEntities(query);
    entities.value = result.items ?? [];
    return result;
  }

  return { snapshots, facts, entities, loading, loadSnapshots, loadFacts, loadEntities };
});
