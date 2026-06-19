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
          <q-tab name="general" :label="t('settingsPage.tabGeneral')" icon="tune" />
          <q-tab name="catalog" :label="t('settingsPage.tabCatalog')" icon="dns" />
        </q-tabs>
        <q-separator />
        <q-tab-panels v-model="settingsTab" animated>
          <q-tab-panel name="general" class="q-pa-none">
            <q-card-section class="app-settings-shell__body">
              <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

              <div class="settings-grid settings-grid--2col">
                <section class="settings-section settings-section--span">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="folder_open" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.pathsTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.rootDirHint') }}</p>
                    </div>
                  </div>
                  <div class="app-form-field-grid--2col">
                    <q-input
                      v-model="rootDir"
                      class="app-glass-control"
                      :label="t('settingsPage.rootDir')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="workDir"
                      class="app-glass-control"
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
                    class="app-glass-control"
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
                    class="app-glass-control app-field-sm"
                    :label="t('settingsPage.globalQuotaUsd')"
                    outlined
                    dense
                    type="number"
                    min="0"
                    step="0.01"
                    prefix="$"
                  />
                </section>

                <section class="settings-section settings-section--span">
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

                <section class="settings-section settings-section--span">
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

                <section class="settings-section settings-section--span">
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
                      class="app-glass-control"
                      :label="t('settingsPage.evalLLM.simProvider')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.simModel"
                      class="app-glass-control"
                      :label="t('settingsPage.evalLLM.simModel')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.judgeProvider"
                      class="app-glass-control"
                      :label="t('settingsPage.evalLLM.judgeProvider')"
                      outlined
                      dense
                    />
                    <q-input
                      v-model="evalLLMForm.judgeModel"
                      class="app-glass-control"
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

                <section class="settings-section settings-section--span">
                  <div class="section-heading">
                    <div class="section-heading__main">
                      <div class="section-title">
                        <q-icon name="park" size="sm" color="primary" />
                        <span class="section-title__text">{{ t('settingsPage.ecosystemTitle') }}</span>
                      </div>
                      <p class="settings-section__hint">{{ t('settingsPage.ecosystemHint') }}</p>
                    </div>
                  </div>

                  <div v-if="ecosystemLoading" class="row items-center q-py-sm">
                    <q-spinner-dots color="primary" size="28px" />
                    <span class="q-ml-sm text-grey-7">{{ t('settingsPage.ecosystemLoading') }}</span>
                  </div>

                  <template v-else-if="ecosystemEntries.length > 0">
                    <div class="ecosystem-industry-list">
                      <div v-for="[industry, info] in ecosystemEntries" :key="industry" class="ecosystem-industry-row">
                        <div class="ecosystem-industry-row__label">
                          <span class="text-body2 text-weight-medium">{{ industry }}</span>
                          <q-badge v-if="info.loaded" color="positive" outline :label="t('settingsPage.ecosystemLoaded')" class="q-ml-sm" />
                          <q-badge v-else color="grey" outline :label="t('settingsPage.ecosystemNotLoaded')" class="q-ml-sm" />
                        </div>
                        <div v-if="info.loaded" class="ecosystem-industry-row__stats text-caption text-grey-7">
                          <span>Agent: {{ info.agents ?? 0 }}</span>
                          <span class="q-ml-md">Team: {{ info.teams ?? 0 }}</span>
                          <span class="q-ml-md">{{ t('settingsPage.ecosystemTaxonomyNodes') }}: {{ info.taxonomy_nodes ?? 0 }}</span>
                        </div>
                        <div class="ecosystem-industry-row__action">
                          <q-btn
                            v-if="!info.loaded"
                            flat
                            dense
                            no-caps
                            color="primary"
                            icon="download"
                            :label="t('settingsPage.ecosystemLoad')"
                            :loading="ecosystemActionLoading === industry"
                            @click="handleLoadIndustry(industry)"
                          />
                          <q-btn
                            v-else
                            flat
                            dense
                            no-caps
                            color="negative"
                            icon="delete_outline"
                            :label="t('settingsPage.ecosystemUnload')"
                            :loading="ecosystemActionLoading === industry"
                            @click="confirmUnloadIndustry(industry, info)"
                          />
                        </div>
                      </div>
                    </div>

                    <q-btn
                      v-if="unloadedIndustries.length > 0"
                      flat
                      no-caps
                      color="primary"
                      icon="download"
                      :label="t('settingsPage.ecosystemLoadAll')"
                      class="q-mt-sm"
                      :loading="ecosystemActionLoading === '__all__'"
                      @click="handleLoadAll"
                    />
                  </template>

                  <q-banner v-else dense rounded class="settings-info-banner">
                    <template #avatar>
                      <q-icon name="info" color="grey" />
                    </template>
                    {{ t('settingsPage.ecosystemEmpty') }}
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

    <q-dialog v-model="unloadDialogVisible" persistent>
      <q-card class="app-dialog-card app-glass-dialog" style="min-width: 340px">
        <q-card-section>
          <div class="text-h6">{{ t('settingsPage.unloadConfirmTitle') }}</div>
        </q-card-section>
        <q-card-section>
          <p>{{ t('settingsPage.unloadConfirmHint', { industry: unloadTargetIndustry }) }}</p>
          <ul class="q-pl-md q-mt-sm">
            <li>{{ t('settingsPage.unloadConfirmAgents', { count: unloadTargetInfo?.agents ?? 0 }) }}</li>
            <li>{{ t('settingsPage.unloadConfirmTeams', { count: unloadTargetInfo?.teams ?? 0 }) }}</li>
            <li>{{ t('settingsPage.unloadConfirmTaxonomyNodes', { count: unloadTargetInfo?.taxonomy_nodes ?? 0 }) }}</li>
          </ul>
          <q-banner dense rounded class="bg-warning text-dark q-mt-md"> {{ t('settingsPage.unloadConfirmWarning') }} </q-banner>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="t('common.cancel')" />
          <q-btn
            unelevated
            no-caps
            color="negative"
            :label="t('settingsPage.unloadConfirmAction')"
            :loading="ecosystemActionLoading === unloadTargetIndustry"
            @click="handleUnloadConfirmed"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import KnowledgeEmbedderFields from '../components/knowledge/KnowledgeEmbedderFields.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import WebResearchFields from '../components/settings/WebResearchFields.vue';
import SystemSettingsCatalogTab from './SystemSettingsCatalogTab.vue';
import { useSystemSettingsPage } from '../features/system-settings/useSystemSettingsPage';
import { useEcosystemPreset } from '../features/system-settings/useEcosystemPreset';

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

const {
  ecosystemLoading,
  ecosystemActionLoading,
  unloadDialogVisible,
  unloadTargetIndustry,
  unloadTargetInfo,
  ecosystemEntries,
  unloadedIndustries,
  handleLoadIndustry,
  handleLoadAll,
  confirmUnloadIndustry,
  handleUnloadConfirmed,
} = useEcosystemPreset();
</script>
