<template>
  <div class="model-catalog-tab q-gutter-md">
    <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.catalogTitle') }}</h2>
      <p class="app-settings-section__hint">
        {{ t('catalogTab.catalogHint') }}
      </p>

      <div v-if="status" class="catalog-status-grid q-mb-md">
        <div class="catalog-stat">
          <span class="catalog-stat__label">{{ t('catalogTab.statusLabel') }}</span>
          <span class="catalog-stat__value">{{
            status.catalogLoaded ? t('catalogTab.loaded') : t('catalogTab.notLoaded')
          }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">Provider</span>
          <span class="catalog-stat__value">{{ status.providerCount ?? 0 }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">Models</span>
          <span class="catalog-stat__value">{{ status.modelCount ?? 0 }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">{{ t('catalogTab.lastSync') }}</span>
          <span class="catalog-stat__value">{{ lastSyncLabel }}</span>
        </div>
        <div class="catalog-stat catalog-stat--wide">
          <span class="catalog-stat__label">{{ t('catalogTab.localPath') }}</span>
          <span class="catalog-stat__value text-caption">{{ status.localPath || '—' }}</span>
        </div>
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.policyTitle') }}</h2>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input
          v-model="policyForm.sourceUrl"
          :label="t('catalogTab.sourceUrl')"
          outlined
          dense
          class="app-field-long"
        />
        <q-select
          v-model="policyForm.syncPolicy"
          :label="t('catalogTab.syncPolicy')"
          outlined
          dense
          emit-value
          map-options
          :options="syncPolicyOptions"
        />
        <q-input
          v-model.number="policyForm.syncIntervalHours"
          type="number"
          min="1"
          :label="t('catalogTab.syncInterval')"
          outlined
          dense
        />
        <q-select
          v-model="policyForm.autoApply"
          :label="t('catalogTab.autoApply')"
          outlined
          dense
          emit-value
          map-options
          :options="autoApplyOptions"
        />
      </div>
      <div class="app-actions-bar app-actions-bar--start q-mt-md">
        <q-btn
          color="primary"
          unelevated
          no-caps
          :loading="savingPolicy"
          :label="t('catalogTab.savePolicy')"
          @click="savePolicy"
        />
        <q-btn
          outline
          color="primary"
          no-caps
          :loading="syncing"
          :label="t('catalogTab.syncNow')"
          @click="runSync(false)"
        />
        <q-btn flat color="secondary" no-caps :loading="syncing" label="Dry Run" @click="runSync(true)" />
        <q-btn flat color="primary" no-caps :loading="loading" :label="t('catalogTab.refresh')" @click="loadAll" />
      </div>
      <div v-if="status?.lastSyncSummary" class="text-caption text-grey-7 q-mt-sm">
        {{ t('catalogTab.recent', { summary: status.lastSyncSummary }) }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.migrationTitle') }}</h2>
      <p class="app-settings-section__hint">
        {{ t('catalogTab.migrationHint') }}
      </p>
      <div class="app-actions-bar app-actions-bar--start q-mb-md">
        <q-btn
          outline
          color="primary"
          no-caps
          :loading="loadingMigration"
          :label="t('catalogTab.previewImpact')"
          @click="loadMigrationPreview"
        />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :loading="applyingMigration"
          :disable="!migrationItems?.length"
          :label="t('catalogTab.applyMigration')"
          @click="runApplyMigration"
        />
      </div>
      <q-list v-if="migrationItems?.length" bordered class="q-mb-md rounded-borders">
        <q-item v-for="item in migrationItems" :key="item.legacyProvider">
          <q-item-section>
            <q-item-label>{{ item.legacyProvider }} → {{ item.catalogProvider }}</q-item-label>
            <q-item-label caption>
              LLM {{ item.llmRows ?? 0 }} · Agent {{ item.agents ?? 0 }} · Session {{ item.sessions ?? 0 }} · Eval
              {{ item.evalFields ?? 0 }} · Runtime {{ item.runtimeSettings ?? 0 }} · Skill {{ item.skills ?? 0 }} ·
              Embed {{ item.knowledgeEmbed ?? 0 }} · Research {{ item.webResearch ?? 0 }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else-if="migrationLoaded" class="text-caption text-grey-7 q-mb-md">{{ t('catalogTab.noMigration') }}</div>
      <div v-if="migrationRules.length" class="text-caption text-grey-7 q-mb-sm">
        {{ t('catalogTab.builtinRules', { version: migrationVersion || '—' }) }}
        <span v-if="migrationLastApplied"> · {{ t('catalogTab.lastApplied', { time: migrationLastApplied }) }}</span>
      </div>
      <q-list v-if="migrationRules.length" bordered dense separator class="rounded-borders">
        <q-item v-for="rule in migrationRules" :key="rule.legacy">
          <q-item-section>
            <q-item-label>{{ rule.legacy }} → {{ rule.catalog }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.browseTitle') }}</h2>
      <p class="app-settings-section__hint">{{ t('catalogTab.browseHint') }}</p>
      <div class="row q-col-gutter-sm q-mb-md items-end">
        <div class="col-grow">
          <q-input
            v-model="providerBrowseQ"
            dense
            outlined
            clearable
            debounce="300"
            :label="t('catalogTab.searchProvider')"
            @update:model-value="loadProviderBrowse(true)"
          />
        </div>
        <div class="col-auto">
          <q-btn
            flat
            no-caps
            :disable="providerBrowseOffset <= 0"
            :label="t('catalogTab.prevPage')"
            @click="providerBrowsePrev"
          />
          <q-btn
            flat
            no-caps
            :disable="providerBrowseOffset + providerBrowseItems.length >= providerBrowseTotal"
            :label="t('catalogTab.nextPage')"
            @click="providerBrowseNext"
          />
        </div>
      </div>
      <q-list v-if="providerBrowseItems.length" bordered separator class="rounded-borders q-mb-md">
        <q-item v-for="p in providerBrowseItems" :key="p.id">
          <q-item-section>
            <q-item-label>{{ p.name || p.id }}</q-item-label>
            <q-item-label caption>{{ p.id }} · {{ p.modelCount ?? 0 }} models</q-item-label>
          </q-item-section>
          <q-item-section side>
            <a
              v-if="providerDocHref(p.doc)"
              :href="providerDocHref(p.doc)"
              target="_blank"
              rel="noopener"
              class="text-caption"
              >{{ t('catalogTab.docLink') }}</a
            >
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="text-caption text-grey-7 q-mb-md">{{ t('catalogTab.noMatchingProvider') }}</div>
      <div class="text-caption text-grey-7 q-mb-sm">
        {{
          t('catalogTab.showing', {
            from: providerBrowseOffset + 1,
            to: providerBrowseOffset + providerBrowseItems.length,
            total: providerBrowseTotal,
          })
        }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.jsonBrowseTitle') }}</h2>
      <p class="app-settings-section__hint">
        {{ t('catalogTab.jsonBrowseHint') }}
      </p>
      <q-input
        :model-value="jsonFilter"
        dense
        outlined
        clearable
        debounce="300"
        :placeholder="t('catalogTab.jsonSearchPlaceholder')"
        class="q-mb-sm app-field-long"
        @update:model-value="onJsonFilterChange"
      />
      <q-banner v-if="jsonSearchLegacyMode" rounded class="bg-warning text-dark q-mb-sm">
        {{ t('catalogTab.legacyModeBanner') }}
      </q-banner>
      <q-banner v-if="jsonSearchTruncated" rounded class="bg-info text-white q-mb-sm">
        {{ t('catalogTab.truncatedBanner', { cap: jsonSearchCap }) }}
      </q-banner>
      <div class="row q-col-gutter-sm q-mb-sm items-center">
        <span class="text-caption text-grey-7">
          {{ t('catalogTab.matched', { total: jsonSearchTotal }) }}
          <template v-if="jsonSearchBlocks.length === 1">
            · {{ t('catalogTab.currentItem', { index: jsonSearchOffset + 1 }) }}</template
          >
        </span>
        <q-space />
        <q-btn
          flat
          dense
          no-caps
          :disable="jsonSearchOffset <= 0"
          :label="t('catalogTab.prevPage')"
          @click="jsonSearchPrev"
        />
        <q-btn
          flat
          dense
          no-caps
          :disable="jsonSearchOffset + jsonSearchLimit >= jsonSearchTotal"
          :label="t('catalogTab.nextPage')"
          @click="jsonSearchNext"
        />
      </div>
      <div v-if="jsonSearchLoading" class="row justify-center q-py-lg">
        <q-spinner color="primary" size="32px" />
      </div>
      <JsonCodeViewer v-else-if="jsonSearchDisplayText" :text="jsonSearchDisplayText" scroll-height="480px" />
      <div v-else class="text-caption text-grey-7 q-py-md">
        {{ jsonSearchError || (jsonSearchQuery ? t('catalogTab.noMatchResult') : t('catalogTab.noCatalogData')) }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">{{ t('catalogTab.syncLogsTitle') }}</h2>
      <q-list v-if="logs.length" bordered separator class="rounded-borders">
        <q-item v-for="entry in logs" :key="entry.id">
          <q-item-section>
            <q-item-label>{{ entry.id }}</q-item-label>
            <q-item-label caption>{{ entry.message }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-badge :color="entry.status === 'ok' ? 'positive' : 'negative'" :label="entry.status || '?'" />
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="text-caption text-grey-7">{{ t('catalogTab.noSyncLogs') }}</div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useModelCatalogTab } from '../features/model-catalog/useModelCatalogTab';
import JsonCodeViewer from '../components/common/JsonCodeViewer.vue';

const {
  loading,
  savingPolicy,
  syncing,
  error,
  status,
  policyForm,
  logs,
  loadingMigration,
  applyingMigration,
  migrationLoaded,
  migrationItems,
  migrationRules,
  migrationVersion,
  migrationLastApplied,
  providerBrowseQ,
  providerBrowseItems,
  providerBrowseTotal,
  providerBrowseOffset,
  jsonFilter,
  jsonSearchBlocks,
  jsonSearchTotal,
  jsonSearchOffset,
  jsonSearchLimit,
  jsonSearchCap,
  jsonSearchLoading,
  jsonSearchError,
  jsonSearchLegacyMode,
  jsonSearchTruncated,
  syncPolicyOptions,
  autoApplyOptions,
  lastSyncLabel,
  jsonSearchDisplayText,
  jsonSearchQuery,
  loadAll,
  savePolicy,
  runSync,
  loadMigrationPreview,
  runApplyMigration,
  loadProviderBrowse,
  providerBrowsePrev,
  providerBrowseNext,
  providerDocHref,
  onJsonFilterChange,
  loadJsonSearch,
  jsonSearchPrev,
  jsonSearchNext,
  t,
} = useModelCatalogTab();
</script>
