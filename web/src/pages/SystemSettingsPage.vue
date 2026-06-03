<template>
  <q-page class="app-standard-page system-settings-page">
    <div class="app-page-shell">
      <AppPageHero
        class="q-mb-md"
        :kicker="t('settingsPage.kicker')"
        :title="t('settingsPage.title')"
        :subtitle="t('settingsPage.subtitle')"
      />

      <q-card flat class="app-settings-shell">
        <q-tabs
          v-model="settingsTab"
          dense
          align="left"
          class="app-settings-tabs"
          active-color="primary"
          indicator-color="primary"
        >
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
                        <span class="section-title__text">{{ t('settingsPage.pathsTitle', '路径') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.rootDirHint') }}</p>
                    </div>
                  </div>
                  <div class="settings-field-stack">
                    <q-input
                      v-model="rootDir"
                      class="app-field-long"
                      :label="t('settingsPage.rootDir')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="workDir"
                      class="app-field-long"
                      :label="t('settingsPage.workDir')"
                      :hint="t('settingsPage.workDirHint')"
                      outlined
                      dense
                    />
                  </div>
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="link" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.a2aPublicBaseTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.a2aPublicBaseHint') }}</p>
                    </div>
                  </div>
                  <q-input
                    v-model.trim="a2aPublicBaseUrl"
                    class="app-field-long"
                    :label="t('settingsPage.a2aPublicBaseUrl')"
                    :hint="effectiveA2AHint"
                    outlined
                    dense
                  />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="vpn_key" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.credentialKeyTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.credentialKeyHint') }}</p>
                    </div>
                  </div>
                  <q-banner
                    dense
                    rounded
                    :class="credentialKeyConfigured ? 'settings-info-banner' : 'settings-warning-banner'"
                  >
                    <template #avatar>
                      <q-icon
                        :name="credentialKeyConfigured ? 'lock' : 'lock_open'"
                        :color="credentialKeyConfigured ? 'positive' : 'warning'"
                      />
                    </template>
                    {{
                      credentialKeyConfigured
                        ? t('settingsPage.credentialKeyConfigured')
                        : t('settingsPage.credentialKeyPending')
                    }}
                  </q-banner>
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="account_balance_wallet" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.globalQuotaTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.globalQuotaHint') }}</p>
                    </div>
                  </div>
                  <q-input
                    v-model.number="globalMonthlyUsd"
                    class="app-field-sm"
                    :label="t('settingsPage.globalQuotaUsd')"
                    outlined
                    dense
                    type="number"
                    min="0"
                    step="0.01"
                    prefix="$"
                  />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="travel_explore" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.webResearchTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.webResearchHint') }}</p>
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
                        <span class="section-title__text">{{ t('settingsPage.mcpAdhocTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.mcpAdhocHint') }}</p>
                    </div>
                  </div>
                  <q-toggle v-model="mcpAllowAdhocHttp" :label="t('settingsPage.mcpAdhocToggle')" />
                </section>

                <section class="settings-section">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="model_training" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.knowledgeEmbedTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.knowledgeEmbedHint') }}</p>
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
                        <span class="section-title__text">{{ t('settingsPage.evalLLM.title') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.evalLLM.hint') }}</p>
                    </div>
                  </div>
                  <div class="app-form-field-grid app-form-field-grid--2col">
                    <q-input
                      v-model="evalLLMForm.simProvider"
                      :label="t('settingsPage.evalLLM.simProvider')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.simModel"
                      :label="t('settingsPage.evalLLM.simModel')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.judgeProvider"
                      :label="t('settingsPage.evalLLM.judgeProvider')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.judgeModel"
                      :label="t('settingsPage.evalLLM.judgeModel')"
                      outlined
                      dense
                    />
                  </div>
                  <q-banner v-if="evalLLMConfigured" dense rounded class="settings-info-banner q-mt-md">
                    <template #avatar>
                      <q-icon name="check_circle" color="positive" />
                    </template>
                    {{ t('settingsPage.evalLLM.configured') }}
                  </q-banner>
                </section>
              </div>

              <div class="settings-footer">
                <div v-if="lastSavedLabel" class="settings-footer__timestamp">{{ lastSavedLabel }}</div>
                <div class="settings-footer__actions">
                  <q-btn
                    outline
                    color="primary"
                    no-caps
                    :loading="loading"
                    :label="t('settingsPage.reload')"
                    icon="refresh"
                    @click="load"
                  />
                  <q-btn
                    color="primary"
                    unelevated
                    no-caps
                    :loading="saving"
                    :label="t('settingsPage.save')"
                    icon="save"
                    @click="save"
                  />
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
import { ref } from 'vue';
import KnowledgeEmbedderFields from '../components/knowledge/KnowledgeEmbedderFields.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import WebResearchFields from '../components/settings/WebResearchFields.vue';
import SystemSettingsCatalogTab from './SystemSettingsCatalogTab.vue';
import { useSystemSettingsPage } from '../features/system-settings/useSystemSettingsPage';

const settingsTab = ref('general');
const {
  t,
  rootDir,
  workDir,
  a2aPublicBaseUrl,
  effectiveA2AHint,
  globalMonthlyUsd,
  mcpAllowAdhocHttp,
  credentialKeyConfigured,
  knowledgeEmbedForm,
  evalLLMForm,
  webResearchForm,
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
} = useSystemSettingsPage();
</script>
