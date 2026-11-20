<template>
  <!-- Harness › 连接 › 模型：Agent 引用却没登记的模型 → 关键数 → 筛选 → 模型卡 / 表 + 检视面板 -->
  <q-page :class="['app-standard-page cap-page resource-manager-page', { 'is-dark': isDark }]">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        v-if="rows.length"
        unelevated
        no-caps
        color="primary"
        icon="add"
        :label="t('connect.models.create')"
        @click="openCreate"
      />
    </Teleport>

    <p v-if="credentialEncryptionAvailable === false" class="cap-note">
      <q-icon name="lock_open" size="16px" />
      <span class="cap-note__body">{{ t('resourceManager.credentialWarning') }}</span>
    </p>

    <!-- 关系：Agent 按 provider/model 两个字符串找模型，这里找不到的那些 Agent 建不起来 -->
    <section v-if="missing.length" class="cap-section cx-missing" role="alert">
      <div class="cap-section__head">
        <q-icon name="link_off" size="20px" class="cx-missing__icon" />
        <h2 class="cap-section__title">{{ t('connect.models.missingTitle', { count: missingAgentCount }) }}</h2>
      </div>
      <p class="cap-section__hint">{{ t('connect.models.missingDesc') }}</p>
      <ul class="cx-rows" role="list">
        <li v-for="item in missing" :key="`${item.provider}/${item.model}`">
          <i class="cap-dot cap-dot--alert" aria-hidden="true" />
          <span class="cx-rows__main cx-num">
            {{ t('connect.models.missingRow', { provider: item.provider, model: item.model }) }}
          </span>
          <span class="cx-rows__side">{{ t('connect.models.missingAgents', { count: item.agents.length }) }}</span>
          <q-btn flat dense no-caps icon="add" :label="t('connect.models.register')" @click="openCreate" />
        </li>
      </ul>
    </section>
    <p v-if="onDisabled > 0" class="cap-note">
      <q-icon name="block" size="16px" />
      <span class="cap-note__body">{{ t('connect.models.onDisabled', { count: onDisabled }) }}</span>
    </p>

    <AppStatePanel v-if="!loaded && loading" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="loadError && !rows.length"
      variant="error"
      :description="loadError"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="reload"
    />

    <ConnectGuide
      v-else-if="!rows.length"
      icon="model_training"
      :title="t('connect.models.guideTitle')"
      :description="t('connect.models.guideDesc')"
      :steps="[t('connect.models.guideStep1'), t('connect.models.guideStep2'), t('connect.models.guideStep3')]"
      :action-label="t('connect.models.create')"
      @action="openCreate"
    />

    <template v-else>
      <AppStatePanel
        v-if="loadError"
        variant="partial"
        dense
        :description="loadError"
        :action-label="t('shell.common.retry')"
        action-icon="refresh"
        @action="reload"
      />
      <CapStats
        :stats="stats"
        :label="t('connect.common.stripLabel')"
        :active="stateFilter"
        @pick="stateFilter = stateFilter === $event ? '' : $event"
      />
      <div class="cap-bar">
        <q-input
          v-model="keyword"
          class="cap-bar__search"
          dense
          outlined
          clearable
          debounce="150"
          :placeholder="t('connect.models.search')"
          :aria-label="t('connect.models.search')"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select
          v-model="providerTypeFilter"
          dense
          outlined
          multiple
          use-chips
          emit-value
          map-options
          :label="t('resourceManager.providerType')"
          :options="providerTypeFilterOptions"
        />
        <span class="cap-bar__spacer" />
        <span class="cap-bar__meta cx-num">{{
          t('connect.common.shownOf', { shown: total, total: rows.length })
        }}</span>
        <q-btn
          flat
          dense
          round
          icon="refresh"
          :loading="loading"
          :aria-label="t('connect.common.refresh')"
          @click="reloadAll"
        />
      </div>

      <div class="cx-workspace" :class="{ 'cx-workspace--inspecting': inspectorOpen }">
        <div class="cx-workspace__main">
          <p v-if="!pagedProviderRows.length" class="cap-note cap-note--info">
            <q-icon name="filter_alt_off" size="16px" />
            <span class="cap-note__body">{{ t('connect.common.noMatch') }}</span>
          </p>
          <div v-else class="app-registry-table-shell provider-table-shell">
            <ProviderModelsTable
              :rows="pagedProviderRows"
              :loading="loading"
              :saving="saving"
              :list-key-state="listKeyState"
              :fetch-provider-logo-svg="fetchProviderLogoSvg"
              :used-by="usedByCount"
              @row-click="onRowClick"
              @toggle-enabled="toggleEnabled"
              @toggle-reveal-key="toggleListKeyReveal"
              @trend="openTrend"
              @edit="openEdit"
              @delete="removeWithImpact"
            />
            <AppRegistryPagination
              v-if="total > rowsPerPage"
              v-model:page="page"
              v-model:page-size="rowsPerPage"
              :page-max="pageCount"
              :total="total"
              :loading="loading"
              :label="t('resourceManager.modelsUnit')"
              :page-size-options="[10, 20, 50]"
            />
          </div>
        </div>

        <AppRegistryInspector
          v-model="inspectorOpen"
          :title="inspected ? modelDisplayName(inspected) : ''"
          :eyebrow="inspected ? `MODEL / ${inspected.provider}` : undefined"
        >
          <div v-if="inspected" class="cx-inspect">
            <div class="cx-inspect__lead">
              <ProviderLogo :provider-id="inspected.provider || ''" size="28px" :fetch-svg="fetchProviderLogoSvg" />
              <span class="cap-card__status">
                <i class="cap-dot" :class="inspected.enabled ? 'cap-dot--ok' : 'cap-dot--off'" aria-hidden="true" />
                {{ inspected.enabled ? t('connect.common.enabled') : t('connect.common.disabled') }}
              </span>
            </div>
            <dl class="cx-facts">
              <div>
                <dt>{{ t('connect.models.facts.provider') }}</dt>
                <dd>{{ providerDisplayName(inspected, inspectedConfig) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.model') }}</dt>
                <dd class="cx-num">{{ inspected.model }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.type') }}</dt>
                <dd class="cx-num">{{ inspectedConfig.provider_type || 'openai' }}</dd>
              </div>
              <div v-if="inspectedConfig.api_base_url">
                <dt>{{ t('connect.models.facts.baseUrl') }}</dt>
                <dd class="cx-num">{{ inspectedConfig.api_base_url }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.context') }}</dt>
                <dd class="cx-num">{{ formatContextWindow(inspectedConfig.context_window_k) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.key') }}</dt>
                <dd>
                  <i class="cap-dot" :class="hasKey ? 'cap-dot--ok' : 'cap-dot--watch'" aria-hidden="true" />
                  {{ hasKey ? t('connect.models.facts.keySet') : t('connect.models.facts.keyMissing') }}
                </dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.pricing') }}</dt>
                <dd>
                  {{
                    rowPricingNotConfigured(inspected)
                      ? t('connect.models.facts.pricingMissing')
                      : t('connect.models.facts.pricingSet')
                  }}
                </dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.ha') }}</dt>
                <dd>{{ haText }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.channel') }}</dt>
                <dd>
                  {{
                    isLocalChannelModel(inspected) ? t('connect.models.facts.local') : t('connect.models.facts.cloud')
                  }}
                </dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.calls30d') }}</dt>
                <dd class="cx-num">{{ formatCount(inspectedConfig.usage_call_count_30d) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.cost30d') }}</dt>
                <dd class="cx-num">{{ formatMicroUsd(inspectedConfig.usage_cost_micro_usd_30d) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.models.facts.success30d') }}</dt>
                <dd class="cx-num">{{ formatPercent(inspectedConfig.success_rate_30d) }}</dd>
              </div>
            </dl>
            <section class="cx-inspect__section">
              <h3 class="cx-inspect__section-title">
                {{ t('connect.models.agentsTitle') }}
                <span class="cx-rows__side">{{ inspectedAgents.length }}</span>
              </h3>
              <p v-if="usageFailed" class="cx-inspect__empty">{{ t('connect.models.usedByUnknown') }}</p>
              <p v-else-if="!inspectedAgents.length" class="cx-inspect__empty">{{ t('connect.models.usedByNone') }}</p>
              <ul v-else class="cx-rows" role="list">
                <li v-for="agent in inspectedAgents.slice(0, 8)" :key="agent.id">
                  <q-icon name="smart_toy" size="14px" />
                  <router-link class="cx-rows__main" :to="`/agents/${agent.id}/settings`">
                    {{ agent.display_name || agent.agent_key }}
                  </router-link>
                  <span class="cx-rows__side">{{ agent.agent_key }}</span>
                </li>
                <li v-if="inspectedAgents.length > 8">
                  <span class="cx-rows__main">
                    {{ t('connect.models.agentsMore', { count: inspectedAgents.length - 8 }) }}
                  </span>
                </li>
              </ul>
            </section>
          </div>
          <template v-if="inspected" #footer>
            <div class="cx-inspect__actions">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="edit"
                :label="t('connect.common.edit')"
                @click="openEdit(inspected)"
              />
              <q-btn flat no-caps icon="query_stats" :label="t('connect.models.trend')" @click="openTrend(inspected)" />
              <q-btn
                flat
                no-caps
                color="negative"
                icon="delete"
                :label="t('connect.common.remove')"
                @click="removeWithImpact(inspected)"
              />
            </div>
          </template>
        </AppRegistryInspector>
      </div>
    </template>

    <AppDialog v-model="dialogOpen" persistent size="xl" :title="dialogTitle" :subtitle="dialogSubtitle">
      <nav class="provider-wizard-nav" role="tablist" aria-label="Provider 配置步骤">
        <button
          v-for="step in providerWizardSteps"
          :key="step.id"
          type="button"
          role="tab"
          class="provider-wizard-step"
          :class="{ 'is-active': providerStep === step.id, 'is-done': providerStep > step.id }"
          :aria-selected="providerStep === step.id"
          @click="providerStep = step.id"
        >
          <span class="provider-wizard-step__index">{{ step.id }}</span>
          <span class="provider-wizard-step__text">
            <span class="provider-wizard-step__label">{{ step.title }}</span>
            <span class="provider-wizard-step__caption">{{ step.caption }}</span>
          </span>
        </button>
      </nav>

      <ProviderWizardStep1Connect
        v-show="providerStep === 1"
        v-model:provider-form="providerForm"
        :editing-id="editingId"
        :provider-add-mode="providerAddMode"
        :catalog-provider-search="catalogProviderSearch"
        :catalog-provider-id="catalogProviderId"
        :catalog-provider-hint="catalogProviderHint"
        :catalog-provider-doc-url="catalogProviderDocUrl"
        :catalog-provider-options="catalogProviderOptions"
        :catalog-loading="catalogLoading"
        :catalog-models-hint="catalogModelsHint"
        :catalog-models-loading="catalogModelsLoading"
        :provider-preset-key="providerPresetKey"
        :provider-preset-options="providerPresetOptions"
        :provider-runtime-locked="providerRuntimeLocked"
        :provider-runtime-summary="providerRuntimeSummary"
        :provider-type-options="providerTypeOptions"
        :show-api-key="showApiKey"
        :api-key-field-hint="apiKeyFieldHint"
        :api-key-masked-placeholder="apiKeyMaskedPlaceholder"
        :revealing-credentials="revealingCredentials"
        :use-catalog-model-picker="useCatalogModelPicker"
        :provider-model-options="providerModelOptions"
        :provider-code-rule="providerCodeRule"
        :provider-runtime-binding-preview="providerRuntimeBindingPreview"
        :provider-identity-changed="providerIdentityChanged"
        :variant-options="variantOptions"
        :current-auth-type="currentAuthType"
        :show-secret-key="showSecretKey"
        :secret-key-masked-placeholder="secretKeyMaskedPlaceholder"
        :can-inspect-provider-model="canInspectProviderModel"
        :checking-model="checkingModel"
        :filter-catalog-models-local="filterCatalogModelsLocal"
        @update:provider-add-mode="setProviderAddMode($event === 'custom' ? 'custom' : 'catalog')"
        @update:catalog-provider-search="
          catalogProviderSearch = $event ?? '';
          reloadCatalogProviders();
        "
        @update:catalog-provider-id="applyCatalogProvider($event)"
        @update:provider-preset-key="applyProviderPreset($event)"
        @toggle-api-key-visibility="toggleApiKeyVisibility"
        @set-custom-model-value="setCustomModelValue"
        @update:model-api-id="
          useCatalogModelPicker ? applyCatalogModel(String($event ?? '')) : applyModelPreset(String($event ?? ''))
        "
        @inspect-current-provider-model="inspectCurrentProviderModel"
        @toggle-secret-key-visibility="toggleSecretKeyVisibility"
      />

      <ProviderWizardStep2Specs
        v-show="providerStep === 2"
        v-model:provider-form="providerForm"
        :provider-add-mode="providerAddMode"
        :category-options="categoryOptions"
        :catalog-pricing-missing="catalogPricingMissing"
        :show-pricing-warning="showPricingWarning"
        :pricing-warning-message="pricingWarningMessage"
      />

      <ProviderWizardStep3HA
        v-show="providerStep === 3"
        :provider-h-a-form="providerHAForm"
        :ha-candidate-provider-type-options="haCandidateProviderTypeOptions"
        @update:ha-form="updateHAForm"
      />

      <ProviderWizardStep4Advanced v-show="providerStep === 4" v-model:provider-form="providerForm" />

      <template #actions>
        <q-btn flat no-caps :label="t('common.cancel')" @click="dialogOpen = false" />
        <span v-if="!saving && submitBlockReason" class="provider-dialog-block-hint">
          <q-icon name="info_outline" size="14px" class="q-mr-xs" />{{ submitBlockReason }}
        </span>
        <q-space />
        <q-btn v-if="providerStep > 1" flat no-caps :label="t('common.back')" @click="providerStep -= 1" />
        <q-btn v-if="providerStep < 4" flat no-caps :label="t('common.next')" @click="providerStep += 1" />
        <span class="provider-dialog-save-wrap">
          <q-btn
            unelevated
            no-caps
            class="provider-dialog-save"
            :label="editingId ? t('common.save') : t('common.create')"
            :loading="saving"
            :disable="saving || !canSubmitNewProviderModel"
            @click="saveRow"
          />
          <q-tooltip v-if="!saving && !canSubmitNewProviderModel">{{ submitBlockReason }}</q-tooltip>
        </span>
      </template>
    </AppDialog>

    <ProviderTrendDialog
      v-model="trendDialogOpen"
      :row="trendRow"
      :metric="trendMetric"
      :metric-options="trendMetricOptions"
      :overview="trendOverview"
      :loading="trendOverviewLoading"
      :range="trendRange"
      :range-options="trendRangeOptions"
      :range-label="trendRangeLabel"
      @update:metric="trendMetric = $event"
      @update:range="trendRange = $event"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CapStats from '../components/capability/CapStats.vue';
import ConnectGuide from '../components/connect/ConnectGuide.vue';
import ProviderLogo from '../components/platform/ProviderLogo.vue';
import ProviderModelsTable from '../components/platform/ProviderModelsTable.vue';
import ProviderTrendDialog from '../components/platform/ProviderTrendDialog.vue';
import ProviderWizardStep1Connect from '../components/platform/ProviderWizardStep1Connect.vue';
import ProviderWizardStep2Specs from '../components/platform/ProviderWizardStep2Specs.vue';
import ProviderWizardStep3HA from '../components/platform/ProviderWizardStep3HA.vue';
import ProviderWizardStep4Advanced from '../components/platform/ProviderWizardStep4Advanced.vue';
import {
  formatContextWindow,
  formatCount,
  formatMicroUsd,
  formatPercent,
  getProviderConfig,
  isLocalChannelModel,
  modelDisplayName,
  providerDisplayName,
  providerHasApiKey,
  rowPricingNotConfigured,
} from '../components/platform/providerModelUi';
import { confirm } from '../composables/useAppConfirm';
import { useResourceManagerPage } from '../features/platform/useResourceManagerPage';
import { useModelUsage } from '../features/platform/useModelUsage';
import type { PlatformResource } from '../features/platform/types';
import { useModelCatalogStore } from '../stores/model-catalog';

const { t } = useI18n();

const providerWizardSteps = computed(() => [
  { id: 1, title: t('resourceManager.step1Title'), caption: t('resourceManager.step1Caption') },
  { id: 2, title: t('resourceManager.step2Title'), caption: t('resourceManager.step2Caption') },
  { id: 3, title: t('resourceManager.step3Title'), caption: t('resourceManager.step3Caption') },
  { id: 4, title: t('resourceManager.step4Title'), caption: t('resourceManager.step4Caption') },
]);

const {
  isDark,
  rows,
  loaded,
  loading,
  loadError,
  reload,
  saving,
  checkingModel,
  keyword,
  dialogOpen,
  editingId,
  page,
  rowsPerPage,
  showApiKey,
  showSecretKey,
  revealingCredentials,
  trendDialogOpen,
  trendRow,
  providerPresetKey,
  providerStep,
  providerAddMode,
  catalogProviderId,
  catalogProviderHint,
  catalogProviderDocUrl,
  catalogModelsHint,
  catalogProviderSearch,
  reloadCatalogProviders,
  useCatalogModelPicker,
  providerRuntimeLocked,
  providerRuntimeSummary,
  catalogProviderOptions,
  catalogLoading,
  catalogModelsLoading,
  filterCatalogModelsLocal,
  providerTypeFilter,
  stateFilter,
  categoryOptions,
  providerTypeOptions,
  providerTypeFilterOptions,
  haCandidateProviderTypeOptions,
  variantOptions,
  providerForm,
  providerHAForm,
  total,
  pageCount,
  pagedProviderRows,
  providerPresetOptions,
  currentAuthType,
  canInspectProviderModel,
  apiKeyFieldHint,
  apiKeyMaskedPlaceholder,
  secretKeyMaskedPlaceholder,
  showPricingWarning,
  canSubmitNewProviderModel,
  submitBlockReason,
  providerIdentityChanged,
  providerRuntimeBindingPreview,
  catalogPricingMissing,
  providerModelOptions,
  dialogTitle,
  dialogSubtitle,
  pricingWarningMessage,
  openCreate,
  openEdit,
  saveRow,
  applyProviderPreset,
  applyModelPreset,
  applyCatalogProvider,
  applyCatalogModel,
  setProviderAddMode,
  setCustomModelValue,
  inspectCurrentProviderModel,
  toggleApiKeyVisibility,
  toggleSecretKeyVisibility,
  toggleEnabled,
  confirmRemoveRow,
  removeRow,
  openTrend,
  listKeyState,
  toggleListKeyReveal,
  trendOverview,
  trendOverviewLoading,
  trendMetric,
  trendMetricOptions,
  trendRange,
  trendRangeOptions,
  trendRangeLabel,
  providerCodeRule,
  updateHAForm,
  credentialEncryptionAvailable,
} = useResourceManagerPage();

// F-03 fix: pass Store function via props instead of component importing Store
const modelCatalogStore = useModelCatalogStore();
const fetchProviderLogoSvg = (id: string) => modelCatalogStore.fetchProviderLogoSvg(id);

const usage = useModelUsage(rows);
const { missing, onDisabled, agentsUsing } = usage;
const usageFailed = usage.failed;
onMounted(() => void usage.load());

function reloadAll() {
  void reload();
  void usage.load();
}

const missingAgentCount = computed(() => missing.value.reduce((sum, item) => sum + item.agents.length, 0));

function usedByCount(row: PlatformResource): number | null {
  return usage.loaded.value ? agentsUsing(row).length : null;
}

const stats = computed(() => {
  const all = rows.value.length;
  const enabled = rows.value.filter((row) => row.enabled).length;
  const nokey = rows.value.filter((row) => row.enabled && !providerHasApiKey(getProviderConfig(row))).length;
  return [
    { key: '', label: t('connect.common.all'), value: all, clickable: true },
    { key: 'enabled', label: t('connect.common.enabled'), value: enabled, tone: 'ok' as const, clickable: true },
    {
      key: 'nokey',
      label: t('connect.models.nokey'),
      value: nokey,
      tone: nokey ? ('watch' as const) : undefined,
      clickable: true,
    },
    {
      key: 'disabled',
      label: t('connect.common.disabled'),
      value: all - enabled,
      tone: 'off' as const,
      clickable: true,
    },
  ];
});

const inspectedId = ref('');
const inspected = computed(() => rows.value.find((row) => row.id === inspectedId.value) ?? null);
const inspectorOpen = computed({
  get: () => inspected.value !== null,
  set: (open: boolean) => {
    if (!open) inspectedId.value = '';
  },
});
const inspectedConfig = computed(() => (inspected.value ? getProviderConfig(inspected.value) : {}));
const inspectedAgents = computed(() => (inspected.value ? agentsUsing(inspected.value) : []));
const hasKey = computed(() => providerHasApiKey(inspectedConfig.value));
const haText = computed(() =>
  inspectedConfig.value.ha_mode
    ? t('connect.models.facts.haOn', {
        mode: inspectedConfig.value.ha_mode,
        count: (inspectedConfig.value.ha_candidates || []).length,
      })
    : t('connect.models.facts.haOff'),
);

function onRowClick(_event: Event, row: PlatformResource) {
  inspectedId.value = row.id;
}

/** 删除前先说清后果：还有 Agent 在用时后端会拒绝（只查 agents 表），直接告诉要先换掉谁 */
async function removeWithImpact(row: PlatformResource) {
  const users = usage.loaded.value ? agentsUsing(row).length : 0;
  if (users > 0) {
    await confirm({
      title: t('connect.common.remove'),
      message: t('connect.models.deleteBlocked', { count: users, name: modelDisplayName(row) }),
      tone: 'danger',
      confirmLabel: t('connect.common.inspect'),
    }).then((ok) => {
      if (ok) inspectedId.value = row.id;
    });
    return;
  }
  if (!usage.loaded.value) {
    await confirmRemoveRow(row);
    return;
  }
  const ok = await confirm({
    title: t('connect.common.remove'),
    message: t('connect.models.deleteMessage', { name: modelDisplayName(row) }),
    tone: 'danger',
  });
  if (ok) await confirmRemoveRowSilently(row);
}

async function confirmRemoveRowSilently(row: PlatformResource) {
  // confirmRemoveRow 自带一次笼统的确认；这里已经说清后果确认过，直接走删除
  await removeRow(row);
  if (inspectedId.value === row.id) inspectedId.value = '';
}
</script>

<style scoped>
.cx-missing {
  border-color: color-mix(in srgb, var(--nt-danger) 45%, transparent);
}

.cx-missing__icon {
  color: var(--nt-danger);
}
</style>
