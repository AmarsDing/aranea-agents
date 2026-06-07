import { ref, reactive } from 'vue';
import { getCompany, listDepartments, listPositions, invalidateCache } from './api';
import type { Company, Department, Position } from './types';

export function useOrgDetail(companyKey: string) {
  const company = ref<Company | null>(null);
  const departments = ref<Department[]>([]);
  const positions = ref<Position[]>([]);
  const departmentPositions = reactive<Record<string, Position[]>>({});
  const loading = ref(false);

  async function fetchDetail() {
    loading.value = true;
    invalidateCache(); // 清除 organization 缓存以获取最新数据
    try {
      const [compResult, depResult] = await Promise.all([getCompany(companyKey), listDepartments(companyKey)]);
      company.value = compResult;
      departments.value = depResult.items;
    } finally {
      loading.value = false;
    }
  }

  async function fetchPositions(departmentKey: string) {
    const result = await listPositions(companyKey, departmentKey);
    positions.value = result.items;
    departmentPositions[departmentKey] = result.items;
  }

  // TODO: persist sort order via API when backend supports it
  function reorderPositions(departmentKey: string, reordered: Position[]) {
    departmentPositions[departmentKey] = reordered;
  }

  return {
    company,
    departments,
    positions,
    departmentPositions,
    loading,
    fetchDetail,
    fetchPositions,
    reorderPositions,
  };
}
