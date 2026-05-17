import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listGraphs,
  getGraph,
  createGraph,
  updateGraph,
  deleteGraph,
  type GraphDefinition
} from "../../features/graph/api";

export const useGraphStore = defineStore("graph", () => {
  const graphs = ref<GraphDefinition[]>([]);
  const activeGraph = ref<GraphDefinition | null>(null);
  const loading = ref(false);
  const total = ref(0);

  async function loadGraphs(pageSize = 50, pageToken = "") {
    loading.value = true;
    try {
      const result = await listGraphs(pageSize, pageToken);
      graphs.value = result.items ?? [];
      total.value = result.total ?? graphs.value.length;
    } finally {
      loading.value = false;
    }
  }

  async function fetchGraph(id: string) {
    const g = await getGraph(id);
    activeGraph.value = g;
    return g;
  }

  async function addGraph(payload: Partial<GraphDefinition>) {
    const created = await createGraph(payload);
    graphs.value.unshift(created);
    activeGraph.value = created;
    return created;
  }

  async function editGraph(id: string, payload: Partial<GraphDefinition>) {
    const updated = await updateGraph(id, payload);
    graphs.value = graphs.value.map((g) => (g.id === id ? updated : g));
    if (activeGraph.value?.id === id) activeGraph.value = updated;
    return updated;
  }

  async function removeGraph(id: string) {
    await deleteGraph(id);
    graphs.value = graphs.value.filter((g) => g.id !== id);
    if (activeGraph.value?.id === id) activeGraph.value = null;
  }

  return { graphs, activeGraph, loading, total, loadGraphs, fetchGraph, addGraph, editGraph, removeGraph };
});
