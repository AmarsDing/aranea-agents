import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listIndustries,
  getIndustry,
  listDepartments,
  listPositions,
  getPositionPrompt,
  listPositionVariants,
} from '../../features/industries/api';
import type {
  Industry,
  Department,
  Position,
  PositionPromptResult,
  VariantInfo,
} from '../../features/industries/types';

export const useIndustryStore = defineStore('industry', () => {
  const industries = ref<Industry[]>([]);
  const currentIndustry = ref<Industry | null>(null);
  const departments = ref<Department[]>([]);
  const positions = ref<Position[]>([]);
  const departmentPositions = ref<Record<string, Position[]>>({});
  const promptResult = ref<PositionPromptResult | null>(null);
  const variantList = ref<VariantInfo[]>([]);
  const error = ref<string | null>(null);
  const loadingIndustries = ref(false);
  const loadingDetail = ref(false);
  const loadingDepartments = ref(false);
  const loadingPositions = ref(false);
  const loadingPrompt = ref(false);
  const loadingVariants = ref(false);

  async function loadIndustries() {
    loadingIndustries.value = true;
    error.value = null;
    try {
      const result = await listIndustries();
      industries.value = result.items;
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to load industries';
    } finally {
      loadingIndustries.value = false;
    }
  }

  async function loadIndustryDetail(key: string) {
    loadingDetail.value = true;
    try {
      currentIndustry.value = await getIndustry(key);
    } finally {
      loadingDetail.value = false;
    }
  }

  async function loadDepartments(industryKey: string) {
    loadingDepartments.value = true;
    try {
      const result = await listDepartments(industryKey);
      departments.value = result.items;
    } finally {
      loadingDepartments.value = false;
    }
  }

  async function loadPositions(industryKey: string, departmentKey?: string) {
    loadingPositions.value = true;
    try {
      const result = await listPositions(industryKey, departmentKey);
      positions.value = result.items;
    } finally {
      loadingPositions.value = false;
    }
  }

  async function loadPositionPrompt(industryKey: string, positionKey: string, variant?: string) {
    loadingPrompt.value = true;
    try {
      promptResult.value = await getPositionPrompt(industryKey, positionKey, variant);
    } finally {
      loadingPrompt.value = false;
    }
  }

  async function loadPositionVariants(industryKey: string, positionKey: string) {
    loadingVariants.value = true;
    try {
      variantList.value = await listPositionVariants(industryKey, positionKey);
    } catch {
      variantList.value = [{ key: 'general', label: '通用' }];
    } finally {
      loadingVariants.value = false;
    }
  }

  return {
    industries,
    currentIndustry,
    departments,
    positions,
    departmentPositions,
    promptResult,
    variantList,
    error,
    loadingIndustries,
    loadingDetail,
    loadingDepartments,
    loadingPositions,
    loadingPrompt,
    loadingVariants,
    loadIndustries,
    loadIndustryDetail,
    loadDepartments,
    loadPositions,
    loadPositionPrompt,
    loadPositionVariants,
  };
});
