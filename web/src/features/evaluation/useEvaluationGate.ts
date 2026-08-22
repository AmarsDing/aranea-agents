import { ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useEvaluationStore } from '../../stores/evaluation';

export function useEvaluationGate() {
  const $q = useQuasar();
  const { t } = useI18n();
  const evaluationStore = useEvaluationStore();

  const gateOpen = ref(false);
  const gateLoading = ref(false);
  const gateSaving = ref(false);
  const gateForm = ref({
    enabled: false,
    agent_id: '',
    dataset_id: '',
    metric: 'exact_match',
    min_score: 0,
    max_drop: 0,
    mode: 'advisory',
  });

  async function openGate() {
    gateOpen.value = true;
    gateLoading.value = true;
    try {
      const cfg = await evaluationStore.loadGateConfig();
      gateForm.value = {
        enabled: cfg.enabled,
        agent_id: cfg.agent_id,
        dataset_id: cfg.dataset_id,
        metric: cfg.metric || 'exact_match',
        min_score: cfg.min_score,
        max_drop: cfg.max_drop,
        mode: cfg.mode || 'advisory',
      };
    } catch (e) {
      console.warn('[evaluation] load gate config failed:', e);
    } finally {
      gateLoading.value = false;
    }
  }

  async function saveGate() {
    if (gateForm.value.enabled && (!gateForm.value.agent_id || !gateForm.value.dataset_id)) {
      $q.notify({ type: 'warning', message: t('evaluationPage.gateNeedTarget') });
      return;
    }
    gateSaving.value = true;
    try {
      await evaluationStore.saveGateConfig({ ...gateForm.value });
      gateOpen.value = false;
      $q.notify({ type: 'positive', message: t('evaluationPage.gateSaved') });
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: e instanceof Error ? e.message : t('evaluationPage.gateSaveFailed'),
      });
    } finally {
      gateSaving.value = false;
    }
  }

  return { gateOpen, gateLoading, gateSaving, gateForm, openGate, saveGate };
}
