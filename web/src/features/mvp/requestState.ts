import { i18n } from '../../i18n';
import { ref } from 'vue';

// Four legacy operations pages share the same request-state contract.
// A late response must not overwrite a newer filter, and a second submit is ignored while saving.
export function createMvpRequestState() {
  const loading = ref(false);
  const saving = ref(false);
  const error = ref('');
  let generation = 0;

  async function load<T>(request: () => Promise<T>, apply: (value: T) => void) {
    const current = ++generation;
    loading.value = true;
    error.value = '';
    try {
      const result = await request();
      if (current === generation) apply(result);
    } catch (reason) {
      if (current === generation)
        error.value = reason instanceof Error ? reason.message : i18n.global.t('uiSystem.operations.loadFailed');
    } finally {
      if (current === generation) loading.value = false;
    }
  }

  async function change<T>(request: () => Promise<T>): Promise<T | null> {
    if (saving.value) return null;
    saving.value = true;
    error.value = '';
    try {
      return await request();
    } catch (reason) {
      error.value = reason instanceof Error ? reason.message : i18n.global.t('uiSystem.operations.changeFailed');
      return null;
    } finally {
      saving.value = false;
    }
  }

  return { loading, saving, error, load, change };
}
