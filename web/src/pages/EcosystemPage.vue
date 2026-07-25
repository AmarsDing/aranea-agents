<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      :kicker="t('ecosystemPage.kicker')"
      :title="t('ecosystemPage.title')"
      :subtitle="t('ecosystemPage.subtitle')"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          icon="refresh"
          :label="t('common.refresh')"
          :loading="loading"
          @click="load"
        />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="add"
          :label="t('ecosystemPage.publish')"
          @click="publishOpen = true"
        />
      </template>
    </AppPageHero>

    <q-banner rounded class="app-info-banner q-mb-md">
      {{ t('ecosystemPage.previewBanner') }}
    </q-banner>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        :label="t('common.search')"
        @update:model-value="debouncedLoad"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
    </AppPageToolbar>

    <q-card v-if="!loading && products.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="storefront" />
        <div class="text-h6 q-mt-md">{{ t('ecosystemPage.emptyTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">{{ t('ecosystemPage.emptyHint') }}</div>
      </q-card-section>
    </q-card>

    <div v-else class="row q-col-gutter-md">
      <div v-for="p in products" :key="p.id" class="col-12 col-md-6 col-lg-4">
        <q-card flat class="app-glass-panel full-height column">
          <q-card-section class="col">
            <div class="text-subtitle1 text-weight-bold">{{ p.display_name || p.name }}</div>
            <div class="text-caption text-grey-7">{{ typeLabel(p.type) }} · v{{ p.version }}</div>
            <div class="text-body2 q-mt-sm">{{ p.description || t('ecosystemPage.noDescription') }}</div>
            <div class="text-caption q-mt-sm">{{ t('ecosystemPage.installCount', { count: p.install_count }) }}</div>
          </q-card-section>
          <q-card-actions align="right">
            <q-btn
              v-if="!p.installed"
              flat
              color="primary"
              no-caps
              :label="t('ecosystemPage.install')"
              :loading="installingId === p.id"
              @click="install(p)"
            />
            <q-chip v-else dense color="positive" text-color="white">{{ t('ecosystemPage.installed') }}</q-chip>
          </q-card-actions>
        </q-card>
      </div>
    </div>

    <q-dialog v-model="publishOpen">
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('ecosystemPage.publishDialogTitle') }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-sm">
          <q-input v-model="draft.name" class="app-field-md" dense outlined :label="t('ecosystemPage.fieldName')" />
          <q-input
            v-model="draft.display_name"
            class="app-field-md"
            dense
            outlined
            :label="t('ecosystemPage.fieldDisplayName')"
          />
          <q-input
            v-model="draft.description"
            class="app-field-long"
            dense
            outlined
            autogrow
            type="textarea"
            :label="t('ecosystemPage.fieldDescription')"
          />
          <q-select
            v-model="draft.type"
            dense
            outlined
            emit-value
            map-options
            :label="t('ecosystemPage.fieldType')"
            :options="typeOptions"
          />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps :label="t('common.cancel')" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="t('ecosystemPage.publish')"
            :loading="publishing"
            :disable="!draft.name || !draft.display_name || !draft.type"
            @click="publish"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import { useEcosystemPage } from '../features/ecosystem/useEcosystemPage';

const { t } = useI18n();

const {
  products,
  search,
  loading,
  publishing,
  publishOpen,
  installingId,
  draft,
  load,
  debouncedLoad,
  install,
  publish,
} = useEcosystemPage();

const typeOptions = computed(() => [
  { label: t('ecosystemPage.typeSkillPack'), value: 'skill_pack' },
  { label: t('ecosystemPage.typeAgentTemplate'), value: 'agent_template' },
  { label: t('ecosystemPage.typeToolBundle'), value: 'tool_bundle' },
]);

const typeLabelMap: Record<string, string> = {
  skill_pack: 'typeSkillPack',
  agent_template: 'typeAgentTemplate',
  tool_bundle: 'typeToolBundle',
};

function typeLabel(type: string): string {
  const key = typeLabelMap[type];
  return key ? t(`ecosystemPage.${key}`) : type;
}
</script>
