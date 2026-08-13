<template>
  <q-page class="app-standard-page app-registry-page plugins-page">
    <AppPageHero
      :kicker="t('pluginsPage.kicker')"
      :title="t('pluginsPage.title')"
      :subtitle="t('pluginsPage.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          color="primary"
          icon="history"
          :label="t('pluginsPage.btnRuns')"
          to="/plugins/runs"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="250"
        :label="t('pluginsPage.search')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="category"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('pluginsPage.filterCategory')"
        :options="categoryOptions"
      />
      <q-select
        v-model="enabled"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('pluginsPage.filterEnabled')"
        :options="enabledOptions"
      />
      <q-select
        v-model="callbackPoint"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('pluginsPage.filterCallback')"
        :options="callbackPointOptions"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('pluginsPage.btnReset')" @click="resetFilters" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('pluginsPage.btnRefresh')"
          :loading="loading"
          @click="() => loadRows()"
        />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('pluginsPage.retry')" class="text-white" @click="() => loadRows()" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <PluginsTable
        :rows="rows"
        :loading="loading"
        :toggling-id="togglingId"
        @toggle-enabled="toggleEnabled"
        @view-detail="openDetail"
        @edit-config="openConfig"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('pluginsPage.paginationLabel')"
      />
    </div>

    <PluginDetailDialog
      v-model:open="detailOpen"
      v-model:scope-mode="scopeMode"
      v-model:scope-agent-id="scopeAgentId"
      :target="detailTarget"
      :agent-options="agentOptions"
      :saving-scope="savingScope"
      :bumping-sort="bumpingSort"
      @bump-sort="(delta) => detailTarget && bumpSort(detailTarget, delta)"
      @save-scope="saveScope"
    />

    <PluginConfigDialog
      v-model:open="configOpen"
      v-model:config-text="configText"
      v-model:mode="configMode"
      :target="configTarget"
      :config-error="configError"
      :saving="savingConfig"
      @save="saveConfig"
      @validation-error="onSchemaValidationError"
    />
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import PluginConfigDialog from '../components/plugins/PluginConfigDialog.vue';
import PluginDetailDialog from '../components/plugins/PluginDetailDialog.vue';
import PluginsTable from '../components/plugins/PluginsTable.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { usePluginsPage } from '../features/plugins/usePluginsPage';

const { t } = useI18n();

const {
  rows,
  total,
  page,
  pageSize,
  pageMax,
  search,
  category,
  enabled,
  callbackPoint,
  loading,
  error,
  togglingId,
  detailOpen,
  detailTarget,
  scopeMode,
  scopeAgentId,
  agentOptions,
  savingScope,
  bumpingSort,
  configOpen,
  configTarget,
  configText,
  configMode,
  savingConfig,
  configError,
  categoryOptions,
  enabledOptions,
  callbackPointOptions,
  loadRows,
  resetFilters,
  toggleEnabled,
  openDetail,
  openConfig,
  saveConfig,
  bumpSort,
  saveScope,
  onSchemaValidationError,
} = usePluginsPage();
</script>
