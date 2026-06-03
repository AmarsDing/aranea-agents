import { ref, computed, watch } from 'vue';
// TECH-DEBT: direct API calls; acceptable for wizard composable scoped to create dialog — issue #industry-wizard
import { listIndustries, listDepartments, listPositions, getPositionPrompt, listPositionVariants } from './api';
import type { Industry, Department, Position, PositionPromptResult, VariantInfo } from './types';

export function useIndustryWizard() {
  const industries = ref<Industry[]>([]);
  const departments = ref<Department[]>([]);
  const positions = ref<Position[]>([]);
  const selectedIndustryKey = ref('');
  const selectedDepartmentKey = ref('');
  const selectedPositionKey = ref('');
  const selectedVariant = ref('general');
  const promptResult = ref<PositionPromptResult | null>(null);
  const loadingIndustries = ref(false);
  const loadingDepartments = ref(false);
  const loadingPositions = ref(false);
  const loadingPrompt = ref(false);
  const loadingVariants = ref(false);
  const variantList = ref<VariantInfo[]>([]);

  async function loadIndustries() {
    loadingIndustries.value = true;
    try {
      const result = await listIndustries();
      industries.value = result.items;
    } finally {
      loadingIndustries.value = false;
    }
  }

  async function loadDepartments() {
    if (!selectedIndustryKey.value) {
      departments.value = [];
      return;
    }
    loadingDepartments.value = true;
    try {
      const result = await listDepartments(selectedIndustryKey.value);
      departments.value = result.items;
    } finally {
      loadingDepartments.value = false;
    }
  }

  async function loadPositions() {
    if (!selectedDepartmentKey.value) {
      positions.value = [];
      return;
    }
    loadingPositions.value = true;
    try {
      const result = await listPositions(selectedIndustryKey.value, selectedDepartmentKey.value);
      positions.value = result.items;
    } finally {
      loadingPositions.value = false;
    }
  }

  async function loadPrompt() {
    if (!selectedIndustryKey.value || !selectedPositionKey.value) return;
    loadingPrompt.value = true;
    try {
      promptResult.value = await getPositionPrompt(
        selectedIndustryKey.value,
        selectedPositionKey.value,
        selectedVariant.value,
      );
    } finally {
      loadingPrompt.value = false;
    }
  }

  async function loadVariants() {
    if (!selectedIndustryKey.value || !selectedPositionKey.value) {
      variantList.value = [{ key: 'general', label: '通用' }];
      return;
    }
    loadingVariants.value = true;
    try {
      variantList.value = await listPositionVariants(selectedIndustryKey.value, selectedPositionKey.value);
    } catch {
      variantList.value = [{ key: 'general', label: '通用' }];
    } finally {
      loadingVariants.value = false;
    }
  }

  watch(selectedIndustryKey, () => {
    selectedDepartmentKey.value = '';
    selectedPositionKey.value = '';
    selectedVariant.value = 'general';
    promptResult.value = null;
    departments.value = [];
    positions.value = [];
    variantList.value = [{ key: 'general', label: '通用' }];
    loadDepartments();
  });

  watch(selectedDepartmentKey, () => {
    selectedPositionKey.value = '';
    selectedVariant.value = 'general';
    promptResult.value = null;
    positions.value = [];
    variantList.value = [{ key: 'general', label: '通用' }];
    loadPositions();
  });

  watch(selectedPositionKey, async () => {
    selectedVariant.value = 'general';
    promptResult.value = null;
    await loadVariants();
    if (selectedPositionKey.value) {
      loadPrompt();
    }
  });

  watch(selectedVariant, () => {
    if (!selectedPositionKey.value) return;
    promptResult.value = null;
    loadPrompt();
  });

  const availableVariants = computed(() => {
    if (!selectedPositionKey.value) return [];
    return variantList.value.map((v) => ({
      label: v.label || v.key,
      value: v.key,
    }));
  });

  const selectedIndustry = computed(() => industries.value.find((i) => i.key === selectedIndustryKey.value));
  const selectedDepartment = computed(() => departments.value.find((d) => d.key === selectedDepartmentKey.value));
  const selectedPosition = computed(() => positions.value.find((p) => p.key === selectedPositionKey.value));

  function reset() {
    selectedIndustryKey.value = '';
    selectedDepartmentKey.value = '';
    selectedPositionKey.value = '';
    selectedVariant.value = 'general';
    promptResult.value = null;
    variantList.value = [{ key: 'general', label: '通用' }];
    industries.value = [];
    departments.value = [];
    positions.value = [];
  }

  return {
    industries,
    departments,
    positions,
    selectedIndustryKey,
    selectedDepartmentKey,
    selectedPositionKey,
    selectedVariant,
    promptResult,
    loadingIndustries,
    loadingDepartments,
    loadingPositions,
    loadingPrompt,
    loadingVariants,
    availableVariants,
    selectedIndustry,
    selectedDepartment,
    selectedPosition,
    loadIndustries,
    reset,
  };
}
