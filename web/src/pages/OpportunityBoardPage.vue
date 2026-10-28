<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      :title="t('uiSystem.operations.opportunityTitle')"
      :subtitle="t('uiSystem.operations.opportunitySubtitle')"
    />
    <q-banner v-if="error" class="bg-negative q-mb-md" role="alert"
      >{{ error
      }}<template #action
        ><q-btn flat :label="t('uiSystem.operations.retry')" :disable="loading" @click="refresh" /></template
    ></q-banner>
    <q-card flat class="app-registry-panel"
      ><q-card-section>
        <div class="ui-preview-actions q-mb-md">
          <q-btn-toggle
            v-model="audience"
            toggle-color="primary"
            :options="ui.audienceOptions"
            :disable="saving"
            @update:model-value="refresh"
          /><q-btn
            flat
            round
            icon="refresh"
            :aria-label="t('uiSystem.operations.refreshOpportunities')"
            :loading="loading"
            @click="refresh"
          />
        </div>
        <q-form class="ui-operations-form" @submit.prevent="create"
          ><q-input
            v-model="title"
            dense
            outlined
            :label="t('uiSystem.operations.opportunityRequired')"
            :rules="[required]"
            class="app-field-md" /><q-btn
            type="submit"
            color="primary"
            unelevated
            :label="t('uiSystem.operations.createOpportunity')"
            :loading="saving"
            :disable="!title.trim() || saving || loading"
        /></q-form> </q-card-section
    ></q-card>
    <q-card flat
      ><q-list separator :aria-busy="loading">
        <q-item v-for="item in rows" :key="item.ID" class="ui-operations-row"
          ><q-item-section
            ><q-item-label class="app-registry-cell-primary">{{ item.Title }}</q-item-label
            ><q-item-label caption>{{ item.Audience }} · {{ item.Status }}</q-item-label
            ><q-item-label v-if="item.Summary" caption>{{ item.Summary }}</q-item-label></q-item-section
          ><q-item-section side
            ><div class="ui-preview-actions">
              <q-btn
                flat
                color="primary"
                :label="t('uiSystem.operations.confirm')"
                :disable="saving || loading"
                @click="ack(item.ID)"
              /><q-btn
                outline
                color="primary"
                :label="t('uiSystem.operations.continueSourcing')"
                :to="{ name: 'sourcing', query: { opportunity_id: item.ID, title: item.Title } }"
              /></div></q-item-section
        ></q-item>
        <q-item v-if="!rows.length"
          ><q-item-section class="ui-card-description"
            ><q-spinner v-if="loading" color="primary" /><span v-else>{{
              t('uiSystem.operations.noOpportunities')
            }}</span></q-item-section
          ></q-item
        >
      </q-list></q-card
    >
  </q-page>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useOpportunityBoardStore } from '../stores/mvp';
import { createMvpUi } from '../features/mvp/mvpUi';
import AppPageHero from '../components/layout/AppPageHero.vue';
const { t } = useI18n();
const ui = computed(() => createMvpUi(t));
const store = useOpportunityBoardStore();
const { rows, loading, saving, error } = storeToRefs(store);
const audience = ref('');
const title = ref('');
const required = (value: string) => !!value.trim() || t('uiSystem.operations.opportunityMissing');
const refresh = () => store.load(audience.value);
async function create() {
  if (!title.value.trim() || saving.value || loading.value) return;
  if (await store.create(title.value.trim(), audience.value)) {
    title.value = '';
    await refresh();
  }
}
async function ack(id: string) {
  if (saving.value || loading.value) return;
  if (await store.acknowledge(id)) await refresh();
}
onMounted(refresh);
</script>
