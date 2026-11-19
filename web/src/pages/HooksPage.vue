<template>
  <q-page class="app-standard-page app-registry-page hooks-page">
    <AppPageHero :kicker="t('hooksPage.kicker')" :title="t('hooksPage.title')" :subtitle="t('hooksPage.subtitle')">
      <template #actions>
        <q-btn outline rounded no-caps icon="send" :label="t('hooksPage.btnDeliveries')" to="/hooks/deliveries" />
        <q-btn outline rounded no-caps icon="history" :label="t('hooksPage.btnPluginRuns')" to="/plugins/runs" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          icon="add"
          :label="t('hooksPage.btnCreate')"
          @click="openCreate"
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
        debounce="200"
        :label="t('hooksPage.search')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="filterPoint"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('hooksPage.filterPoint')"
        :options="callbackPointOptions"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('hooksPage.btnReset')" @click="resetFilters" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('hooksPage.btnRefresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageToolbar>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      :action-label="t('hooksPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      :title="t('hooksPage.emptyList')"
      icon="webhook"
      :action-label="t('hooksPage.btnCreate')"
      action-icon="add"
      @action="openCreate"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        :action-label="t('hooksPage.retry')"
        action-icon="refresh"
        @action="loadRows"
      />

      <div class="app-registry-table-shell">
        <HooksTable
          :rows="pagedRows"
          :loading="loading"
          :toggling-id="busyId"
          @toggle-enabled="toggleEnabled"
          @edit="openEdit"
          @remove="confirmDelete"
        />

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :loading="loading"
          :label="t('hooksPage.paginationLabel')"
        />
      </div>
    </template>

    <AppDialog
      v-model="editorOpen"
      persistent
      size="xl"
      :title="editingId ? t('hooksPage.dialogTitleEdit') : t('hooksPage.dialogTitleCreate')"
    >
      <div class="q-gutter-md app-form-wide">
        <div class="app-form-field-grid app-form-field-grid--2col">
          <q-input
            v-model="form.key"
            dense
            outlined
            :label="t('hooksPage.fieldKey')"
            :disable="Boolean(editingId)"
          />
          <q-input v-model="form.name" dense outlined :label="t('hooksPage.fieldName')" />
          <q-toggle v-model="form.enabled" :label="t('hooksPage.fieldEnabled')" />
        </div>
        <q-input
          v-model="form.description"
          class="app-field-long"
          dense
          outlined
          type="textarea"
          autogrow
          :label="t('hooksPage.fieldDescription')"
        />
        <callback-editor v-model="form.rule" v-model:sort-order="form.sort_order" v-model:valid="formValid" />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('hooksPage.btnCancel')" @click="editorOpen = false" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('hooksPage.btnSave')"
          :loading="saving"
          :disable="!formValid || !form.key?.trim() || !form.name?.trim()"
          @click="saveHook"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CallbackEditor from '../components/hooks/CallbackEditor.vue';
import HooksTable from '../components/hooks/HooksTable.vue';
import { useHooksPage } from '../features/hooks/useHooksPage';
import { resolvePageState } from './pageState';

const {
  t,
  loading,
  saving,
  error,
  forbidden,
  search,
  filterPoint,
  callbackPointOptions,
  editorOpen,
  editingId,
  busyId,
  form,
  formValid,
  page,
  pageSize,
  total,
  pagedRows,
  pageMax,
  resetFilters,
  loadRows,
  openCreate,
  openEdit,
  saveHook,
  toggleEnabled,
  confirmDelete,
} = useHooksPage();

/** 五态判定：错误优先于空态；有数据时刷新失败只加 partial 横幅，列表照常可用。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: pagedRows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
