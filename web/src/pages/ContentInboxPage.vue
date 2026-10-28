<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero :title="t('uiSystem.operations.contentTitle')" :subtitle="t('uiSystem.operations.contentSubtitle')" />
    <q-banner v-if="error" class="bg-negative q-mb-md" role="alert"
      >{{ error
      }}<template #action
        ><q-btn flat :label="t('uiSystem.operations.retry')" :disable="loading" @click="refresh" /></template
    ></q-banner>
    <q-banner v-if="feedback" class="app-info-banner" role="status">{{ feedback }}</q-banner>
    <q-card flat class="app-registry-panel"
      ><q-card-section>
        <q-form @submit.prevent="create">
          <h2 class="ui-preview-label">{{ t('uiSystem.operations.manual') }}</h2>
          <div class="app-form-field-grid">
            <q-input
              v-model="title"
              :label="t('uiSystem.operations.titleRequired')"
              dense
              outlined
              :rules="[required]"
            /><q-input v-model="url" :label="t('uiSystem.operations.sourceLink')" dense outlined />
          </div>
          <q-input
            v-model="body"
            type="textarea"
            autogrow
            :label="t('uiSystem.operations.body')"
            dense
            outlined
            class="q-mt-sm"
          />
          <div class="app-actions-bar q-mt-md">
            <q-btn
              type="submit"
              color="primary"
              unelevated
              :label="t('uiSystem.operations.createContent')"
              :loading="saving"
              :disable="!title.trim() || saving || loading"
            />
          </div>
        </q-form> </q-card-section
    ></q-card>
    <q-card flat class="app-registry-panel"
      ><q-card-section class="ui-operations-grid">
        <q-form @submit.prevent="pullRSS"
          ><h2 class="ui-preview-label">{{ t('uiSystem.operations.rssSource') }}</h2>
          <q-input v-model="feedURL" label="RSS Feed URL" dense outlined />
          <div class="app-actions-bar q-mt-sm">
            <q-btn
              type="submit"
              outline
              color="primary"
              :label="t('uiSystem.operations.pullRSS')"
              :loading="saving"
              :disable="!feedURL.trim() || saving || loading"
            /></div
        ></q-form>
        <q-form @submit.prevent="pullSearch"
          ><h2 class="ui-preview-label">{{ t('uiSystem.operations.searchImport') }}</h2>
          <q-input v-model="searchQuery" :label="t('uiSystem.operations.query')" dense outlined /><q-input
            v-model="searchHitURL"
            :label="t('uiSystem.operations.resultLink')"
            dense
            outlined
            class="q-mt-sm" />
          <div class="app-actions-bar q-mt-sm">
            <q-btn
              type="submit"
              outline
              color="primary"
              :label="t('uiSystem.operations.importSearch')"
              :loading="saving"
              :disable="!searchQuery.trim() || !searchHitURL.trim() || saving || loading"
            /></div
        ></q-form> </q-card-section
    ></q-card>
    <q-card flat class="app-registry-panel"
      ><q-card-section class="ui-preview-actions">
        <q-select
          v-model="sourceType"
          dense
          outlined
          emit-value
          map-options
          :label="t('uiSystem.operations.source')"
          :options="ui.sourceOptions"
          class="app-field-sm"
          @update:model-value="refresh"
        />
        <q-select
          v-model="status"
          dense
          outlined
          emit-value
          map-options
          :label="t('uiSystem.operations.state')"
          :options="ui.contentStatusOptions"
          class="app-field-sm"
          @update:model-value="refresh"
        />
        <q-btn
          flat
          round
          icon="refresh"
          :aria-label="t('uiSystem.operations.refreshContent')"
          :loading="loading"
          @click="refresh"
        /> </q-card-section
    ></q-card>
    <AppRegistryTable
      :rows="rows"
      :columns="ui.contentColumns"
      :loading="loading"
      row-key="ID"
      :external-pagination="false"
      :pagination="{ rowsPerPage: 20 }"
      :no-data-label="t('uiSystem.operations.noContent')"
    />
  </q-page>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useContentInboxStore } from '../stores/mvp';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import { createMvpUi } from '../features/mvp/mvpUi';
const { t } = useI18n();
const ui = computed(() => createMvpUi(t));
const store = useContentInboxStore();
const { rows, loading, saving, error, feedback } = storeToRefs(store);
const title = ref('');
const body = ref('');
const url = ref('');
const feedURL = ref('');
const searchQuery = ref('');
const searchHitURL = ref('');
const sourceType = ref('');
const status = ref('');
const required = (value: string) => !!value.trim() || t('uiSystem.operations.titleMissing');
const refresh = () => store.load(sourceType.value, status.value);
async function create() {
  if (!title.value.trim() || saving.value || loading.value) return;
  if (
    await store.create({
      Title: title.value.trim(),
      Body: body.value,
      SourceURL: url.value.trim(),
      SourceType: 'manual',
    })
  ) {
    title.value = '';
    body.value = '';
    url.value = '';
    await refresh();
  }
}
async function pullRSS() {
  if (!feedURL.value.trim() || saving.value || loading.value) return;
  if (await store.ingestRSS(feedURL.value.trim())) await refresh();
}
async function pullSearch() {
  if (!searchQuery.value.trim() || !searchHitURL.value.trim() || saving.value || loading.value) return;
  if (await store.ingestSearch(searchQuery.value.trim(), searchHitURL.value.trim(), body.value)) await refresh();
}
onMounted(refresh);
</script>
