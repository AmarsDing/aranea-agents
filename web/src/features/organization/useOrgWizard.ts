import { ref, computed, watch } from 'vue';
// TECH-DEBT: direct API calls; acceptable for wizard composable scoped to create dialog — issue #org-wizard
import { listCompanies, listDepartments, listPositions, getPositionPrompt, listPositionVariants } from './api';
import type { Company, Department, Position, PositionPromptResult, VariantInfo } from './types';

export function useOrgWizard() {
  const companies = ref<Company[]>([]);
  const departments = ref<Department[]>([]);
  const positions = ref<Position[]>([]);
  const selectedCompanyKey = ref('');
  const selectedDepartmentKey = ref('');
  const selectedPositionKey = ref('');
  const selectedVariant = ref('general');
  const promptResult = ref<PositionPromptResult | null>(null);
  const loadingCompanies = ref(false);
  const loadingDepartments = ref(false);
  const loadingPositions = ref(false);
  const loadingPrompt = ref(false);
  const loadingVariants = ref(false);
  const variantList = ref<VariantInfo[]>([]);

  async function loadCompanies() {
    loadingCompanies.value = true;
    try {
      const result = await listCompanies();
      companies.value = result.items;
    } finally {
      loadingCompanies.value = false;
    }
  }

  async function loadDepartments() {
    if (!selectedCompanyKey.value) {
      departments.value = [];
      return;
    }
    loadingDepartments.value = true;
    try {
      const result = await listDepartments(selectedCompanyKey.value);
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
      const result = await listPositions(selectedCompanyKey.value, selectedDepartmentKey.value);
      positions.value = result.items;
    } finally {
      loadingPositions.value = false;
    }
  }

  async function loadPrompt() {
    if (!selectedCompanyKey.value || !selectedPositionKey.value) return;
    loadingPrompt.value = true;
    try {
      promptResult.value = await getPositionPrompt(
        selectedCompanyKey.value,
        selectedPositionKey.value,
        selectedVariant.value,
      );
    } finally {
      loadingPrompt.value = false;
    }
  }

  async function loadVariants() {
    if (!selectedCompanyKey.value || !selectedPositionKey.value) {
      variantList.value = [{ key: 'general', label: '通用' }];
      return;
    }
    loadingVariants.value = true;
    try {
      variantList.value = await listPositionVariants(selectedCompanyKey.value, selectedPositionKey.value);
    } catch {
      variantList.value = [{ key: 'general', label: '通用' }];
    } finally {
      loadingVariants.value = false;
    }
  }

  watch(selectedCompanyKey, () => {
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

  const selectedCompany = computed(() => companies.value.find((c) => c.key === selectedCompanyKey.value));
  const selectedDepartment = computed(() => departments.value.find((d) => d.key === selectedDepartmentKey.value));
  const selectedPosition = computed(() => positions.value.find((p) => p.key === selectedPositionKey.value));

  function reset() {
    selectedCompanyKey.value = '';
    selectedDepartmentKey.value = '';
    selectedPositionKey.value = '';
    selectedVariant.value = 'general';
    promptResult.value = null;
    variantList.value = [{ key: 'general', label: '通用' }];
    companies.value = [];
    departments.value = [];
    positions.value = [];
  }

  return {
    companies,
    departments,
    positions,
    selectedCompanyKey,
    selectedDepartmentKey,
    selectedPositionKey,
    selectedVariant,
    promptResult,
    loadingCompanies,
    loadingDepartments,
    loadingPositions,
    loadingPrompt,
    loadingVariants,
    availableVariants,
    selectedCompany,
    selectedDepartment,
    selectedPosition,
    loadCompanies,
    reset,
  };
}
