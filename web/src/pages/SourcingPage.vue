<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero :title="t('uiSystem.operations.sourcingTitle')" :subtitle="t('uiSystem.operations.sourcingSubtitle')"
      ><template #actions
        ><q-btn
          outline
          color="primary"
          icon="refresh"
          :label="t('uiSystem.operations.refresh')"
          :loading="loading"
          @click="refresh" /></template
    ></AppPageHero>
    <q-banner v-if="error" class="bg-negative q-mb-md" role="alert"
      >{{ error
      }}<template #action
        ><q-btn flat :label="t('uiSystem.operations.retry')" :disable="loading" @click="refresh" /></template
    ></q-banner>
    <q-banner v-if="opportunityId" class="app-info-banner"
      >{{ t('uiSystem.operations.currentOpportunity', { name: opportunityTitle || opportunityId })
      }}<template #action
        ><q-btn
          flat
          :label="t('uiSystem.operations.clearOpportunity')"
          :disable="saving"
          @click="clearPrefill" /></template
    ></q-banner>
    <q-card flat class="app-registry-panel"
      ><q-card-section
        ><q-form @submit.prevent="create">
          <div class="app-form-field-grid">
            <q-input
              v-model="name"
              dense
              outlined
              :label="t('uiSystem.operations.supplierRequired')"
              :rules="[required]"
            /><q-input v-model="url" dense outlined :label="t('uiSystem.operations.sourceLink')" /><q-input
              v-model="price"
              dense
              outlined
              :label="t('uiSystem.operations.priceRange')"
            />
          </div>
          <div class="app-actions-bar q-mt-md">
            <q-btn
              type="submit"
              color="primary"
              unelevated
              :label="t('uiSystem.operations.addSupplier')"
              :loading="saving"
              :disable="!name.trim() || saving || loading"
            />
          </div> </q-form></q-card-section
    ></q-card>
    <AppRegistryTable
      :rows="rows"
      :columns="ui.supplierColumns"
      :loading="loading"
      row-key="ID"
      :external-pagination="false"
      :pagination="{ rowsPerPage: 20 }"
      :no-data-label="t('uiSystem.operations.noSuppliers')"
    />
  </q-page>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import { useSourcingBoardStore } from '../stores/mvp';
import { createMvpUi } from '../features/mvp/mvpUi';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
const { t } = useI18n();
const ui = computed(() => createMvpUi(t));
const store = useSourcingBoardStore();
const { rows, loading, saving, error } = storeToRefs(store);
const route = useRoute();
const router = useRouter();
const name = ref('');
const url = ref('');
const price = ref('');
const opportunityId = ref('');
const opportunityTitle = ref('');
const required = (value: string) => !!value.trim() || t('uiSystem.operations.supplierMissing');
function applyQuery() {
  opportunityId.value = String(route.query.opportunity_id || '');
  opportunityTitle.value = String(route.query.title || '');
}
function clearPrefill() {
  void router.replace({ path: '/sourcing' });
}
const refresh = () => store.load(opportunityId.value);
async function create() {
  if (!name.value.trim() || saving.value || loading.value) return;
  if (
    await store.create({
      Name: name.value.trim(),
      URL: url.value.trim(),
      PriceRange: price.value.trim(),
      Status: 'suggested',
      OpportunityID: opportunityId.value || undefined,
    })
  ) {
    name.value = '';
    url.value = '';
    price.value = '';
    await refresh();
  }
}
watch(
  () => route.query,
  () => {
    applyQuery();
    void refresh();
  },
);
onMounted(() => {
  applyQuery();
  void refresh();
});
</script>
