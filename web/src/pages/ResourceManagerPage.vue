<template>
  <q-page :class="['app-standard-page app-registry-page resource-manager-page', { 'is-dark': isDark }]">
    <template v-if="isProviderResource">
      <AppPageHero kicker="LLM Provider" :title="pageTitle" :subtitle="pageSubtitle">
        <template #actions>
          <q-btn
            color="primary"
            unelevated
            rounded
            icon="add"
            :label="t('resourceManager.addProvider')"
            @click="openCreate"
          />
        </template>
      </AppPageHero>

      <q-banner v-if="credentialEncryptionAvailable === false" dense rounded class="app-banner-warning q-mx-md q-mt-sm">
        <template #avatar>
          <q-icon name="lock_open" color="warning" />
        </template>
        {{ t('resourceManager.credentialWarning') }}
      </q-banner>

      <!-- 五态：已有数据但刷新失败 → 局部失败横幅，表格照常渲染 -->
      <AppStatePanel
        v-if="loadError && total > 0"
        variant="partial"
        dense
        class="q-mx-md q-mt-sm"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="reload"
      />

      <!-- 五态：加载失败且无数据 → 只渲染可重试错误面板，不与空态并存。
           403 错误码未透出（useResourceManagerPage 仅保留 message），forbidden 分支未接上。 -->
      <AppStatePanel
        v-else-if="loadError"
        variant="error"
        class="q-mx-md q-mt-sm"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="reload"
      />

      <q-card flat bordered class="app-entity-glass-panel provider-card">
        <q-card-section class="app-page-toolbar__body">
          <q-input
            v-model="keyword"
            class="app-page-toolbar__search provider-control"
            dense
            outlined
            clearable
            debounce="200"
            :placeholder="t('resourceManager.searchPlaceholder')"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <q-select
            v-model="providerTypeFilter"
            class="app-page-toolbar__field provider-control"
            dense
            outlined
            multiple
            use-chips
            emit-value
            map-options
            :label="t('resourceManager.providerType')"
            :options="providerTypeFilterOptions"
          />
        </q-card-section>
        <q-separator />

        <!-- 五态：首屏加载无数据 → 骨架面板 -->
        <AppStatePanel
          v-if="loading && !pagedProviderRows.length && !loadError"
          variant="loading"
          :rows="4"
          class="q-ma-md"
        />

        <!-- 五态：成功但无数据 → 空态 -->
        <AppStatePanel
          v-else-if="!loading && !loadError && !pagedProviderRows.length"
          variant="empty"
          :title="t('resourceManager.emptyTitle')"
          :description="t('resourceManager.emptyHint')"
          icon="manage_search"
          :action-label="t('resourceManager.addProvider')"
          action-icon="add"
          @action="openCreate"
        />

        <div v-else-if="pagedProviderRows.length" class="app-registry-table-shell provider-table-shell">
          <ProviderModelsTable
            :rows="pagedProviderRows"
            :loading="loading"
            :saving="saving"
            :list-key-state="listKeyState"
            :fetch-provider-logo-svg="fetchProviderLogoSvg"
            @toggle-enabled="toggleEnabled"
            @toggle-reveal-key="toggleListKeyReveal"
            @trend="openTrend"
            @edit="openEdit"
            @delete="confirmRemoveRow"
          />

          <AppRegistryPagination
            v-model:page="page"
            v-model:page-size="rowsPerPage"
            :page-max="pageCount"
            :total="total"
            :loading="loading"
            :label="t('resourceManager.modelsUnit')"
            :page-size-options="[10, 20, 50]"
          />
        </div>
      </q-card>
    </template>

    <template v-else>
      <!-- 五态：已有数据但刷新失败 → 局部失败横幅，表格照常渲染 -->
      <AppStatePanel
        v-if="loadError && filteredRows.length > 0"
        variant="partial"
        dense
        class="q-mb-md"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="reload"
      />

      <!-- 五态：加载失败且无数据 → 只渲染可重试错误面板，不与空态并存 -->
      <AppStatePanel
        v-else-if="loadError"
        variant="error"
        class="q-mb-md"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="reload"
      />

      <q-card flat bordered class="resource-card">
        <q-card-section class="row items-center q-col-gutter-md">
          <div class="col-12 col-md">
            <div class="text-h6">{{ pageTitle }}</div>
            <div class="text-caption text-grey-7">{{ pageSubtitle }}</div>
          </div>
          <div class="app-field-md">
            <q-input v-model="keyword" dense outlined clearable debounce="200" :label="t('common.search')">
              <template #prepend><q-icon name="search" /></template>
            </q-input>
          </div>
          <div class="col-auto">
            <q-btn
              color="primary"
              unelevated
              rounded
              icon="add"
              :label="t('resourceManager.addNew')"
              @click="openCreate"
            />
          </div>
        </q-card-section>
        <q-separator />

        <!-- 五态：首屏加载无数据 → 骨架面板 -->
        <AppStatePanel
          v-if="loading && !filteredRows.length && !loadError"
          variant="loading"
          :rows="4"
          class="q-ma-md"
        />

        <!-- 五态：成功但无数据 → 空态 -->
        <AppStatePanel
          v-else-if="!loading && !loadError && !filteredRows.length"
          variant="empty"
          :title="t('resourceManager.emptyTitle')"
          :description="t('resourceManager.emptyHint')"
          icon="dashboard"
          :action-label="t('resourceManager.addNew')"
          action-icon="add"
          @action="openCreate"
        />

        <AppRegistryTable
          v-else-if="filteredRows.length > 0"
          :shell="false"
          :rows="filteredRows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          :pagination="{ rowsPerPage: 10 }"
        >
          <template #body-cell-status="props">
            <q-td :props="props">
              <AppStatusChip :status="props.row.enabled ? 'enabled' : 'disabled'" />
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props" class="q-gutter-xs">
              <q-btn
                flat
                dense
                round
                icon="edit"
                color="primary"
                :aria-label="`编辑 ${props.row.name}`"
                @click="openEdit(props.row)"
              />
              <q-btn
                flat
                dense
                round
                icon="delete"
                color="negative"
                :aria-label="`删除 ${props.row.name}`"
                @click="confirmRemoveRow(props.row)"
              />
            </q-td>
          </template>
        </AppRegistryTable>
      </q-card>
    </template>

    <AppDialog v-model="dialogOpen" persistent size="xl" :title="dialogTitle" :subtitle="dialogSubtitle">
      <template v-if="isProviderResource">
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
            useCatalogModelPicker
              ? applyCatalogModel(String($event ?? ''))
              : applyModelPreset(String($event ?? ''))
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
      </template>

      <div v-else class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="form.key" dense outlined label="标识" />
        <q-input v-model="form.name" dense outlined label="名称" />
        <q-input
          v-model="form.description"
          class="app-grid-span-full"
          dense
          outlined
          autogrow
          type="textarea"
          label="描述"
        />
        <q-input v-model="form.provider" dense outlined label="Provider" />
        <q-input v-model="form.model" dense outlined label="模型" />
        <q-input v-model="form.parent_id" dense outlined label="父级 ID" />
        <q-input v-model="form.agent_id" dense outlined label="Agent ID" />
        <q-input v-model.number="form.sort_order" dense outlined type="number" label="排序" />
        <q-toggle v-model="form.enabled" color="primary" label="启用" />
        <q-input
          v-model="form.config_json"
          class="app-grid-span-full"
          dense
          outlined
          autogrow
          type="textarea"
          label="配置 JSON"
        />
        <q-input
          v-model="form.metadata_json"
          class="app-grid-span-full"
          dense
          outlined
          autogrow
          type="textarea"
          label="元数据 JSON"
        />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('common.cancel')" @click="dialogOpen = false" />
        <span v-if="isProviderResource && !saving && submitBlockReason" class="provider-dialog-block-hint">
          <q-icon name="info_outline" size="14px" class="q-mr-xs" />{{ submitBlockReason }}
        </span>
        <q-space />
        <template v-if="isProviderResource">
          <q-btn v-if="providerStep > 1" flat no-caps :label="t('common.back')" @click="providerStep -= 1" />
          <q-btn v-if="providerStep < 4" flat no-caps :label="t('common.next')" @click="providerStep += 1" />
        </template>
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
          <q-tooltip v-if="isProviderResource && !saving && !canSubmitNewProviderModel">
            {{ submitBlockReason }}
          </q-tooltip>
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
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import ProviderModelsTable from '../components/platform/ProviderModelsTable.vue';
import ProviderTrendDialog from '../components/platform/ProviderTrendDialog.vue';
import ProviderWizardStep1Connect from '../components/platform/ProviderWizardStep1Connect.vue';
import ProviderWizardStep2Specs from '../components/platform/ProviderWizardStep2Specs.vue';
import ProviderWizardStep3HA from '../components/platform/ProviderWizardStep3HA.vue';
import ProviderWizardStep4Advanced from '../components/platform/ProviderWizardStep4Advanced.vue';
import { useResourceManagerPage } from '../features/platform/useResourceManagerPage';
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
  categoryOptions,
  providerTypeOptions,
  providerTypeFilterOptions,
  haCandidateProviderTypeOptions,
  variantOptions,
  form,
  providerForm,
  providerHAForm,
  columns,
  isProviderResource,
  pageTitle,
  pageSubtitle,
  filteredRows,
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
</script>
