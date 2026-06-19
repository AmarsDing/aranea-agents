import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { listTaskPlans, getTaskPlan } from './api';
import type { TaskPlanSummaryData, TaskPlanDetailData } from './types';

export type ObservabilityTab = 'plans' | 'teamRuns' | 'graphExecutions' | 'metrics' | 'flowLogs';

export function useObservabilityDashboard() {
  const { t } = useI18n();
  const $q = useQuasar();

  const activeTab = ref<ObservabilityTab>('plans');

  // Plans state
  const sessionIdInput = ref('');
  const plansLoading = ref(false);
  const plans = ref<TaskPlanSummaryData[]>([]);
  const selectedPlan = ref<TaskPlanDetailData | null>(null);
  const planDetailLoading = ref(false);

  async function loadPlans() {
    const sid = sessionIdInput.value.trim();
    if (!sid) {
      $q.notify({ type: 'warning', message: t('observabilityPage.sessionIdRequired') });
      return;
    }
    plansLoading.value = true;
    selectedPlan.value = null;
    try {
      plans.value = await listTaskPlans(sid);
      if (plans.value.length === 0) {
        $q.notify({ type: 'info', message: t('observabilityPage.noPlansFound') });
      }
    } catch {
      $q.notify({ type: 'negative', message: t('observabilityPage.loadPlansFailed') });
      plans.value = [];
    } finally {
      plansLoading.value = false;
    }
  }

  async function loadPlanDetail(planId: string) {
    const sid = sessionIdInput.value.trim();
    if (!planId || !sid) return;
    planDetailLoading.value = true;
    try {
      selectedPlan.value = await getTaskPlan(planId, sid);
    } catch {
      $q.notify({ type: 'negative', message: t('observabilityPage.loadPlanDetailFailed') });
      selectedPlan.value = null;
    } finally {
      planDetailLoading.value = false;
    }
  }

  function clearPlans() {
    plans.value = [];
    selectedPlan.value = null;
  }

  return {
    activeTab,
    sessionIdInput,
    plansLoading,
    plans,
    selectedPlan,
    planDetailLoading,
    loadPlans,
    loadPlanDetail,
    clearPlans,
    t,
  };
}
