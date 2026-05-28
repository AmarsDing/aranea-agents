<template>
  <q-page class="app-standard-page system-settings-page">
    <div class="app-page-shell">
      <AppPageHero
        class="q-mb-md"
        :kicker="t('settingsPage.kicker', 'System')"
        :title="t('settingsPage.title')"
        :subtitle="t('settingsPage.subtitle', '全局路径、A2A、配额与嵌入模型配置。')"
      />

      <q-card flat class="app-settings-shell">
        <q-tabs v-model="settingsTab" dense align="left" class="app-settings-tabs" active-color="primary" indicator-color="primary">
          <q-tab name="general" label="常规" icon="tune" />
          <q-tab name="catalog" label="模型目录" icon="dns" />
        </q-tabs>
        <q-separator />
        <q-tab-panels v-model="settingsTab" animated>
          <q-tab-panel name="general" class="q-pa-none">
            <q-card-section class="app-settings-shell__body">
              <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

              <div class="settings-grid">
                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="folder_open" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.pathsTitle", "路径") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.rootDirHint") }}</p>
                    </div>
                  </div>
                  <div class="settings-field-stack">
                    <q-input v-model="rootDir" class="app-field-long" :label="t('settingsPage.rootDir')" outlined dense />
                    <q-input v-model="workDir" class="app-field-long" :label="t('settingsPage.workDir')" :hint="t('settingsPage.workDirHint')" outlined dense />
                  </div>
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="link" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.a2aPublicBaseTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.a2aPublicBaseHint") }}</p>
                    </div>
                  </div>
                  <q-input v-model.trim="a2aPublicBaseUrl" class="app-field-long" :label="t('settingsPage.a2aPublicBaseUrl')" :hint="effectiveA2AHint" outlined dense />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="vpn_key" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.credentialKeyTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.credentialKeyHint") }}</p>
                    </div>
                  </div>
                  <q-banner dense rounded :class="credentialKeyConfigured ? 'settings-info-banner' : 'settings-warning-banner'">
                    <template #avatar>
                      <q-icon :name="credentialKeyConfigured ? 'lock' : 'lock_open'" :color="credentialKeyConfigured ? 'positive' : 'warning'" />
                    </template>
                    {{
                      credentialKeyConfigured
                        ? t("settingsPage.credentialKeyConfigured")
                        : t("settingsPage.credentialKeyPending")
                    }}
                  </q-banner>
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="account_balance_wallet" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.globalQuotaTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.globalQuotaHint") }}</p>
                    </div>
                  </div>
                  <q-input v-model.number="globalMonthlyUsd" class="app-field-sm" :label="t('settingsPage.globalQuotaUsd')" outlined dense type="number" min="0" step="0.01" prefix="$" />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="travel_explore" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.webResearchTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.webResearchHint") }}</p>
                    </div>
                  </div>
                  <web-research-fields
                    :form="webResearchForm"
                    :configured="webResearchConfigured"
                    :has-api-key="webResearchHasApiKey"
                    :testing="webResearchTesting"
                    show-status
                    @test="testWebResearchConnection"
                  />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="hub" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.mcpAdhocTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.mcpAdhocHint") }}</p>
                    </div>
                  </div>
                  <q-toggle v-model="mcpAllowAdhocHttp" :label="t('settingsPage.mcpAdhocToggle')" />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="model_training" size="sm" color="primary" />
                        <span class="section-title__text">{{ t("settingsPage.knowledgeEmbedTitle") }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t("settingsPage.knowledgeEmbedHint") }}</p>
                    </div>
                  </div>
                  <knowledge-embedder-fields
                    :form="knowledgeEmbedForm"
                    :configured="knowledgeEmbedConfigured"
                    :has-api-key="knowledgeEmbedHasApiKey"
                    show-status
                  />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="rate_review" size="sm" color="primary" />
                        <span class="section-title__text">评估 LLM（UserSim / Judge）</span>
                      </div>
                      <p class="settings-section__hint">
                        持久化到 system_settings；运行时 env（KRATOS_EVAL_SIM_* / KRATOS_EVAL_JUDGE_*）优先。Judge 未填时回退 Sim。
                      </p>
                    </div>
                  </div>
                  <div class="app-form-field-grid app-form-field-grid--2col">
                    <q-input v-model="evalLLMForm.simProvider" label="UserSim Provider" outlined dense />
                    <q-input v-model="evalLLMForm.simModel" label="UserSim Model" outlined dense />
                    <q-input v-model="evalLLMForm.judgeProvider" label="Judge Provider（可选）" outlined dense />
                    <q-input v-model="evalLLMForm.judgeModel" label="Judge Model（可选）" outlined dense />
                  </div>
                  <q-banner v-if="evalLLMConfigured" dense rounded class="settings-info-banner q-mt-md">
                    <template #avatar>
                      <q-icon name="check_circle" color="positive" />
                    </template>
                    评估 LLM 已配置
                  </q-banner>
                </section>
              </div>

              <div class="settings-footer">
                <div v-if="lastSavedLabel" class="settings-footer__timestamp">{{ lastSavedLabel }}</div>
                <div class="settings-footer__actions">
                  <q-btn outline color="primary" no-caps :loading="loading" :label="t('settingsPage.reload')" icon="refresh" @click="load" />
                  <q-btn color="primary" unelevated no-caps :loading="saving" :label="t('settingsPage.save')" icon="save" @click="save" />
                </div>
              </div>
            </q-card-section>
          </q-tab-panel>
          <q-tab-panel name="catalog" class="q-pa-md">
            <SystemSettingsCatalogTab />
          </q-tab-panel>
        </q-tab-panels>
      </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useSystemSettingsStore } from "../stores/system-settings";
import { useA2AStore } from "../stores/a2a";
import {
  knowledgeEmbedFromSettings,
  knowledgeEmbedToPatch
} from "../features/system-settings/knowledge-embed";
import { DEFAULT_EVAL_LLM_FORM, evalLLMFromSettings } from "../features/system-settings/eval-llm";
import {
  DEFAULT_WEB_RESEARCH_FORM,
  webResearchFromSettings,
  webResearchToPatch
} from "../features/system-settings/web-research";
import { DEFAULT_KNOWLEDGE_EMBED_FORM } from "../features/knowledge/embedder-constants";
import KnowledgeEmbedderFields from "../components/knowledge/KnowledgeEmbedderFields.vue";
import AppPageHero from "../components/layout/AppPageHero.vue";
import WebResearchFields from "../components/settings/WebResearchFields.vue";
import SystemSettingsCatalogTab from "./SystemSettingsCatalogTab.vue";
const { t } = useI18n();
const settingsTab = ref("general");
const $q = useQuasar();
const settingsStore = useSystemSettingsStore();
const a2aStore = useA2AStore();
const rootDir = ref("");
const workDir = ref("");
const a2aPublicBaseUrl = ref("");
const effectiveA2AUrl = ref("");
const globalMonthlyUsd = ref<number | null>(null);
const mcpAllowAdhocHttp = ref(false);
const credentialKeyConfigured = ref(false);
const knowledgeEmbedForm = reactive({ ...DEFAULT_KNOWLEDGE_EMBED_FORM });
const evalLLMForm = reactive({ ...DEFAULT_EVAL_LLM_FORM });
const webResearchForm = reactive({ ...DEFAULT_WEB_RESEARCH_FORM });
const knowledgeEmbedConfigured = ref(false);
const webResearchConfigured = ref(false);
const webResearchHasApiKey = ref(false);
const webResearchTesting = ref(false);
const knowledgeEmbedHasApiKey = ref(false);
const evalLLMConfigured = ref(false);
const updateTime = ref<string | undefined>(undefined);
const loading = ref(false);
const saving = ref(false);
const error = ref("");

const lastSavedLabel = computed(() => {
  const ts = updateTime.value;
  if (!ts) return "";
  return t("settingsPage.lastSaved", { time: ts });
});

const effectiveA2AHint = computed(() => {
  if (!effectiveA2AUrl.value) return t("settingsPage.a2aPublicBaseEmptyHint");
  return t("settingsPage.a2aPublicBaseEffective", { url: effectiveA2AUrl.value });
});

function usdToMicroUsd(usd: number | null | undefined): number {
  if (usd == null || !Number.isFinite(usd) || usd <= 0) return 0;
  return Math.round(usd * 1_000_000);
}

function microUsdToUsd(micro: number | undefined): number | null {
  if (micro == null || !Number.isFinite(micro) || micro <= 0) return null;
  return micro / 1_000_000;
}

onMounted(load);

async function load() {
  loading.value = true;
  error.value = "";
  try {
    await settingsStore.loadSettings();
    const res = settingsStore.settings;
    if (!res) return;
    const a2aCfg = await a2aStore.loadRuntimeConfig().catch(() => null);
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    a2aPublicBaseUrl.value = res.a2aPublicBaseUrl ?? "";
    effectiveA2AUrl.value = a2aCfg?.public_base_url ?? "";
    globalMonthlyUsd.value = microUsdToUsd(res.globalMonthlyMicroUsd);
    mcpAllowAdhocHttp.value = Boolean(res.mcpAllowAdhocHttp);
    credentialKeyConfigured.value = Boolean(res.credentialEncryptionKeyConfigured);
    Object.assign(knowledgeEmbedForm, knowledgeEmbedFromSettings(res.knowledgeEmbed));
    Object.assign(evalLLMForm, evalLLMFromSettings(res.evalLlm));
    Object.assign(webResearchForm, webResearchFromSettings(res.webResearch));
    knowledgeEmbedConfigured.value = Boolean(res.knowledgeEmbed?.configured);
    knowledgeEmbedHasApiKey.value = Boolean(res.knowledgeEmbed?.hasApiKey);
    webResearchConfigured.value = Boolean(res.webResearch?.configured);
    webResearchHasApiKey.value = Boolean(res.webResearch?.hasApiKey);
    evalLLMConfigured.value = Boolean(res.evalLlm?.configured);
    updateTime.value = res.updateTime;
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

async function testWebResearchConnection() {
  webResearchTesting.value = true;
  error.value = "";
  try {
    const patch = webResearchToPatch(webResearchForm);
    const res = await settingsStore.testWebResearchConnection({
      provider: patch.provider,
      apiKey: patch.apiKey,
      maxResults: patch.maxResults,
      fetchTop: patch.fetchTop,
      searchDepth: patch.searchDepth,
      timeoutSec: patch.timeoutSec,
      httpProxy: patch.httpProxy
    });
    if (res.ok) {
      $q.notify({
        type: "positive",
        message: res.message || t("settingsPage.webResearch.testOk")
      });
    } else {
      $q.notify({
        type: "negative",
        message: res.message || t("settingsPage.webResearch.testFailed")
      });
    }
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    $q.notify({ type: "negative", message: msg || t("settingsPage.webResearch.testFailed") });
  } finally {
    webResearchTesting.value = false;
  }
}

async function save() {
  saving.value = true;
  error.value = "";
  try {
    const res = await settingsStore.saveAll({
      rootDirectory: rootDir.value,
      workDirectory: workDir.value,
      globalMonthlyMicroUsd: usdToMicroUsd(globalMonthlyUsd.value),
      a2aPublicBaseUrl: a2aPublicBaseUrl.value,
      mcpAllowAdhocHttp: mcpAllowAdhocHttp.value,
      knowledgeEmbed: knowledgeEmbedToPatch(knowledgeEmbedForm),
      evalLLM: evalLLMForm,
      webResearch: webResearchToPatch(webResearchForm)
    });
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    a2aPublicBaseUrl.value = res.a2aPublicBaseUrl ?? "";
    const a2aCfg = await a2aStore.loadRuntimeConfig().catch(() => null);
    effectiveA2AUrl.value = a2aCfg?.public_base_url ?? "";
    globalMonthlyUsd.value = microUsdToUsd(res.globalMonthlyMicroUsd);
    mcpAllowAdhocHttp.value = Boolean(res.mcpAllowAdhocHttp);
    credentialKeyConfigured.value = Boolean(res.credentialEncryptionKeyConfigured);
    Object.assign(knowledgeEmbedForm, knowledgeEmbedFromSettings(res.knowledgeEmbed));
    Object.assign(evalLLMForm, evalLLMFromSettings(res.evalLlm));
    Object.assign(webResearchForm, webResearchFromSettings(res.webResearch));
    knowledgeEmbedConfigured.value = Boolean(res.knowledgeEmbed?.configured);
    knowledgeEmbedHasApiKey.value = Boolean(res.knowledgeEmbed?.hasApiKey);
    webResearchConfigured.value = Boolean(res.webResearch?.configured);
    webResearchHasApiKey.value = Boolean(res.webResearch?.hasApiKey);
    evalLLMConfigured.value = Boolean(res.evalLlm?.configured);
    updateTime.value = res.updateTime;
    $q.notify({ type: "positive", message: t("settingsPage.saveOk") });
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
    $q.notify({ type: "negative", message: t("settingsPage.saveFailed") });
  } finally {
    saving.value = false;
  }
}
</script>
