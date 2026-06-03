import { ref, reactive } from 'vue';
import { getIndustry, listDepartments, listPositions } from './api';
import type { Industry, Department, Position } from './types';

export function useIndustryDetail(industryKey: string) {
  const industry = ref<Industry | null>(null);
  const departments = ref<Department[]>([]);
  const positions = ref<Position[]>([]);
  const departmentPositions = reactive<Record<string, Position[]>>({});
  const loading = ref(false);

  async function fetchDetail() {
    loading.value = true;
    try {
      const [indResult, depResult] = await Promise.all([getIndustry(industryKey), listDepartments(industryKey)]);
      industry.value = indResult;
      departments.value = depResult.items;
    } finally {
      loading.value = false;
    }
  }

  async function fetchPositions(departmentKey: string) {
    const result = await listPositions(industryKey, departmentKey);
    positions.value = result.items;
    departmentPositions[departmentKey] = result.items;
  }

  // TODO: persist sort order via API when backend supports it
  function reorderPositions(departmentKey: string, reordered: Position[]) {
    departmentPositions[departmentKey] = reordered;
  }

  return {
    industry,
    departments,
    positions,
    departmentPositions,
    loading,
    fetchDetail,
    fetchPositions,
    reorderPositions,
  };
}
