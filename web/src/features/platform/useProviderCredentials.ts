import { ref, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import type { ProviderConfig, ProviderForm } from './types';
import { errorMessage, toNumber } from './providerUtils';
import { usePlatformStore } from '../../stores/platform';
import type { ProviderHAForm } from './types';
import { microPer1KToUsdPer1M, normalizeUsdPer1M } from '../../config/providerRuntimeOverlay';

export function useProviderCredentials(deps: {
  providerForm: ProviderForm;
  providerHAForm: ProviderHAForm;
  editingId: Ref<string>;
  isProviderResource: Ref<boolean> | { value: boolean };
}) {
  const platformStore = usePlatformStore();
  const $q = useQuasar();

  const showApiKey = ref(false);
  const showSecretKey = ref(false);
  const revealingCredentials = ref(false);
  const credentialsLoadedFromServer = ref(false);

  function clearRevealedCredentialsFromForm() {
    if (credentialsLoadedFromServer.value) {
      deps.providerForm.api_key = '';
      deps.providerForm.secret_key = '';
      deps.providerHAForm.haCandidates = deps.providerHAForm.haCandidates.map((c) => ({ ...c, apiKey: '' }));
      credentialsLoadedFromServer.value = false;
    }
  }

  async function loadRevealedCredentials() {
    if (!deps.editingId.value) return;
    revealingCredentials.value = true;
    try {
      const creds = await platformStore.revealCredentials(deps.editingId.value);
      if (creds.api_key) deps.providerForm.api_key = creds.api_key;
      if (creds.secret_key) deps.providerForm.secret_key = creds.secret_key;
      for (const ha of creds.ha_candidates) {
        const idx = deps.providerHAForm.haCandidates.findIndex((c) => c.name.trim() === ha.name.trim());
        if (idx >= 0 && ha.api_key) {
          deps.providerHAForm.haCandidates[idx] = { ...deps.providerHAForm.haCandidates[idx], apiKey: ha.api_key };
        }
      }
      credentialsLoadedFromServer.value = true;
    } catch (error) {
      $q.notify({ type: 'negative', message: errorMessage(error) });
      throw error;
    } finally {
      revealingCredentials.value = false;
    }
  }

  async function toggleApiKeyVisibility() {
    if (
      !showApiKey.value &&
      deps.editingId.value &&
      deps.providerForm.api_key_set &&
      !deps.providerForm.api_key.trim()
    ) {
      try {
        await loadRevealedCredentials();
        showApiKey.value = true;
      } catch {
        /* notified */
      }
      return;
    }
    if (showApiKey.value) {
      clearRevealedCredentialsFromForm();
      showApiKey.value = false;
      return;
    }
    showApiKey.value = true;
  }

  async function toggleSecretKeyVisibility() {
    if (
      !showSecretKey.value &&
      deps.editingId.value &&
      deps.providerForm.secret_id.trim() &&
      !deps.providerForm.secret_key.trim()
    ) {
      try {
        await loadRevealedCredentials();
        showSecretKey.value = true;
      } catch {
        /* notified */
      }
      return;
    }
    if (showSecretKey.value) {
      if (credentialsLoadedFromServer.value) {
        deps.providerForm.secret_key = '';
        if (!showApiKey.value) {
          deps.providerForm.api_key = '';
          deps.providerHAForm.haCandidates = deps.providerHAForm.haCandidates.map((c) => ({ ...c, apiKey: '' }));
          credentialsLoadedFromServer.value = false;
        }
      }
      showSecretKey.value = false;
      return;
    }
    showSecretKey.value = true;
  }

  function loadUsdPricingFromConfig(config: ProviderConfig) {
    const cost = config.cost;
    if (cost && typeof cost === 'object') {
      deps.providerForm.input_price_usd_per_1m = normalizeUsdPer1M(toNumber(cost.input_usd_per_1m, 0));
      deps.providerForm.output_price_usd_per_1m = normalizeUsdPer1M(toNumber(cost.output_usd_per_1m, 0));
      deps.providerForm.cache_read_usd_per_1m = normalizeUsdPer1M(toNumber(cost.cache_read_usd_per_1m, 0));
      deps.providerForm.cache_write_usd_per_1m = normalizeUsdPer1M(toNumber(cost.cache_write_usd_per_1m, 0));
      deps.providerForm.reasoning_price_usd_per_1m = normalizeUsdPer1M(toNumber(cost.reasoning_usd_per_1m, 0));
      deps.providerForm.embedding_price_usd_per_1m = normalizeUsdPer1M(toNumber(cost.embedding_usd_per_1m, 0));
      return;
    }
    deps.providerForm.input_price_usd_per_1m = microPer1KToUsdPer1M(toNumber(config.input_price_micro_usd_per_1k, 0));
    deps.providerForm.output_price_usd_per_1m = microPer1KToUsdPer1M(toNumber(config.output_price_micro_usd_per_1k, 0));
    deps.providerForm.cache_read_usd_per_1m = microPer1KToUsdPer1M(
      toNumber(config.cached_input_price_micro_usd_per_1k, 0),
    );
    deps.providerForm.reasoning_price_usd_per_1m = microPer1KToUsdPer1M(
      toNumber(config.reasoning_price_micro_usd_per_1k, 0),
    );
    deps.providerForm.embedding_price_usd_per_1m = microPer1KToUsdPer1M(
      toNumber(config.embedding_price_micro_usd_per_1k, 0),
    );
  }

  return {
    showApiKey,
    showSecretKey,
    revealingCredentials,
    credentialsLoadedFromServer,
    toggleApiKeyVisibility,
    toggleSecretKeyVisibility,
    loadRevealedCredentials,
    clearRevealedCredentialsFromForm,
    loadUsdPricingFromConfig,
  };
}
