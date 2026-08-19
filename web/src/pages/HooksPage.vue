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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('hooksPage.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

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

    <q-dialog v-model="editorOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--xl app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">
            {{ editingId ? t('hooksPage.dialogTitleEdit') : t('hooksPage.dialogTitleCreate') }}
          </div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md app-form-wide">
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
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps :label="t('hooksPage.btnCancel')" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="t('hooksPage.btnSave')"
            :loading="saving"
            :disable="!formValid || !form.key?.trim() || !form.name?.trim()"
            @click="saveHook"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CallbackEditor from '../components/hooks/CallbackEditor.vue';
import HooksTable from '../components/hooks/HooksTable.vue';
import { useHooksPage } from '../features/hooks/useHooksPage';

const {
  t,
  loading,
  saving,
  error,
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
</script>
