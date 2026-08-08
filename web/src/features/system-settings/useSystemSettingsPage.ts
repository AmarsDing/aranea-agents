import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useSystemSettingsStore } from '../../stores/system-settings';
import { useA2AStore } from '../../stores/a2a';
import { knowledgeEmbedFromSettings, knowledgeEmbedToPatch } from './knowledge-embed';
import { DEFAULT_EVAL_LLM_FORM, evalLLMFromSettings } from './eval-llm';
import { DEFAULT_WEB_RESEARCH_FORM, webResearchFromSettings, webResearchToPatch } from './web-research';
import { DEFAULT_SPEECH_FORM, speechFromSettings, speechToPatch } from './speech';
import { DEFAULT_KNOWLEDGE_EMBED_FORM } from '../knowledge/embedder-constants';

function usdToMicroUsd(usd: number | null | undefined): number {
  if (usd == null || !Number.isFinite(usd) || usd <= 0) return 0;
  return Math.round(usd * 1_000_000);
}

function microUsdToUsd(micro: number | undefined): number | null {
  if (micro == null || !Number.isFinite(micro) || micro <= 0) return null;
  return micro / 1_000_000;
}

export function useSystemSettingsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const settingsStore = useSystemSettingsStore();
  const a2aStore = useA2AStore();

  const rootDir = ref('');
  const workDir = ref('');
  const a2aPublicBaseUrl = ref('');
  const effectiveA2AUrl = ref('');
  const globalMonthlyUsd = ref<number | null>(null);
  const mcpAllowAdhocHttp = ref(false);
  const credentialKeyConfigured = ref(false);
  const knowledgeEmbedForm = reactive({ ...DEFAULT_KNOWLEDGE_EMBED_FORM });
  const evalLLMForm = reactive({ ...DEFAULT_EVAL_LLM_FORM });
  const webResearchForm = reactive({ ...DEFAULT_WEB_RESEARCH_FORM });
  const speechForm = reactive({ ...DEFAULT_SPEECH_FORM });
  // Loaded snapshot for the speech diff-patch (see speechToPatch): keeps the
  // env-fallback semantics intact on unrelated saves.
  const speechLoaded = ref({ ...DEFAULT_SPEECH_FORM });
  const speechAsrConfigured = ref(false);
  const speechAsrHasApiKey = ref(false);
  const speechTtsConfigured = ref(false);
  const speechTtsHasApiKey = ref(false);
  const knowledgeEmbedConfigured = ref(false);
  const webResearchConfigured = ref(false);
  const webResearchHasApiKey = ref(false);
  const webResearchTesting = ref(false);
  const knowledgeEmbedHasApiKey = ref(false);
  const evalLLMConfigured = ref(false);
  const updateTime = ref<string | undefined>(undefined);
  const loading = ref(false);
  const saving = ref(false);
  const error = ref('');

  const lastSavedLabel = computed(() => {
    const ts = updateTime.value;
    if (!ts) return '';
    return t('settingsPage.lastSaved', { time: ts });
  });

  const effectiveA2AHint = computed(() => {
    if (!effectiveA2AUrl.value) return t('settingsPage.a2aPublicBaseEmptyHint');
    return t('settingsPage.a2aPublicBaseEffective', { url: effectiveA2AUrl.value });
  });

  function syncFormFromSettings(res: NonNullable<typeof settingsStore.settings>) {
    rootDir.value = res.rootDirectory ?? '';
    workDir.value = res.workDirectory ?? '';
    a2aPublicBaseUrl.value = res.a2aPublicBaseUrl ?? '';
    globalMonthlyUsd.value = microUsdToUsd(res.globalMonthlyMicroUsd);
    mcpAllowAdhocHttp.value = Boolean(res.mcpAllowAdhocHttp);
    credentialKeyConfigured.value = Boolean(res.credentialEncryptionKeyConfigured);
    Object.assign(knowledgeEmbedForm, knowledgeEmbedFromSettings(res.knowledgeEmbed));
    Object.assign(evalLLMForm, evalLLMFromSettings(res.evalLlm));
    Object.assign(webResearchForm, webResearchFromSettings(res.webResearch));
    const speechNext = speechFromSettings(res.speech);
    Object.assign(speechForm, speechNext);
    speechLoaded.value = speechNext;
    speechAsrConfigured.value = Boolean(res.speech?.asr?.configured);
    speechAsrHasApiKey.value = Boolean(res.speech?.asr?.hasApiKey);
    speechTtsConfigured.value = Boolean(res.speech?.tts?.configured);
    speechTtsHasApiKey.value = Boolean(res.speech?.tts?.hasApiKey);
    knowledgeEmbedConfigured.value = Boolean(res.knowledgeEmbed?.configured);
    knowledgeEmbedHasApiKey.value = Boolean(res.knowledgeEmbed?.hasApiKey);
    webResearchConfigured.value = Boolean(res.webResearch?.configured);
    webResearchHasApiKey.value = Boolean(res.webResearch?.hasApiKey);
    evalLLMConfigured.value = Boolean(res.evalLlm?.configured);
    updateTime.value = res.updateTime;
  }

  async function load() {
    loading.value = true;
    error.value = '';
    try {
      await settingsStore.loadSettings();
      const res = settingsStore.settings;
      if (!res) return;
      const a2aCfg = await a2aStore.loadRuntimeConfig().catch(() => null);
      effectiveA2AUrl.value = a2aCfg?.public_base_url ?? '';
      syncFormFromSettings(res);
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  async function testWebResearchConnection() {
    webResearchTesting.value = true;
    error.value = '';
    try {
      const patch = webResearchToPatch(webResearchForm);
      const res = await settingsStore.testWebResearchConnection({
        provider: patch.provider,
        apiKey: patch.apiKey,
        maxResults: patch.maxResults,
        fetchTop: patch.fetchTop,
        searchDepth: patch.searchDepth,
        timeoutSec: patch.timeoutSec,
        httpProxy: patch.httpProxy,
      });
      if (res.ok) {
        $q.notify({
          type: 'positive',
          message: res.message || t('settingsPage.webResearch.testOk'),
        });
      } else {
        $q.notify({
          type: 'negative',
          message: res.message || t('settingsPage.webResearch.testFailed'),
        });
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      $q.notify({ type: 'negative', message: msg || t('settingsPage.webResearch.testFailed') });
    } finally {
      webResearchTesting.value = false;
    }
  }

  async function save() {
    saving.value = true;
    error.value = '';
    try {
      const res = await settingsStore.saveAll({
        rootDirectory: rootDir.value,
        workDirectory: workDir.value,
        globalMonthlyMicroUsd: usdToMicroUsd(globalMonthlyUsd.value),
        a2aPublicBaseUrl: a2aPublicBaseUrl.value,
        mcpAllowAdhocHttp: mcpAllowAdhocHttp.value,
        knowledgeEmbed: knowledgeEmbedToPatch(knowledgeEmbedForm),
        evalLLM: evalLLMForm,
        webResearch: webResearchToPatch(webResearchForm),
        speech: speechToPatch(speechForm, speechLoaded.value),
      });
      const a2aCfg = await a2aStore.loadRuntimeConfig().catch(() => null);
      effectiveA2AUrl.value = a2aCfg?.public_base_url ?? '';
      syncFormFromSettings(res);
      $q.notify({ type: 'positive', message: t('settingsPage.saveOk') });
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : String(e);
      $q.notify({ type: 'negative', message: t('settingsPage.saveFailed') });
    } finally {
      saving.value = false;
    }
  }

  onMounted(load);

  return {
    t,
    rootDir,
    workDir,
    a2aPublicBaseUrl,
    effectiveA2AUrl,
    effectiveA2AHint,
    globalMonthlyUsd,
    mcpAllowAdhocHttp,
    credentialKeyConfigured,
    knowledgeEmbedForm,
    evalLLMForm,
    webResearchForm,
    speechForm,
    speechAsrConfigured,
    speechAsrHasApiKey,
    speechTtsConfigured,
    speechTtsHasApiKey,
    knowledgeEmbedConfigured,
    webResearchConfigured,
    webResearchHasApiKey,
    webResearchTesting,
    knowledgeEmbedHasApiKey,
    evalLLMConfigured,
    lastSavedLabel,
    loading,
    saving,
    error,
    load,
    testWebResearchConnection,
    save,
  };
}
