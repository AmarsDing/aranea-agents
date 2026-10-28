<template>
  <q-page class="app-standard-page ui-preview-page">
    <AppPageHero
      kicker="ARANEA / DESIGN SYSTEM"
      :title="t('uiSystem.preview.pageTitle')"
      :subtitle="t('uiSystem.preview.pageSubtitle')"
    >
      <template #actions>
        <q-btn
          outline
          color="primary"
          :icon="$q.dark.isActive ? 'light_mode' : 'dark_mode'"
          :label="$q.dark.isActive ? t('uiSystem.preview.toLight') : t('uiSystem.preview.toDark')"
          @click="$q.dark.toggle()"
        />
        <q-btn color="primary" unelevated :label="t('uiSystem.preview.openDialog')" icon="open_in_new" @click="dialogOpen = true" />
      </template>
    </AppPageHero>
    <AppWorkspaceLaunchpad :label="t('uiSystem.preview.destinations')" :items="launchpadItems" />

    <section class="ui-preview-section" aria-labelledby="preview-controls">
      <div class="ui-preview-section__head">
        <h2 id="preview-controls">{{ t('uiSystem.preview.controls') }}</h2>
        <span>{{ t('uiSystem.preview.consistent') }}</span>
      </div>
      <div class="ui-preview-columns">
        <q-card flat
          ><q-card-section>
            <h3 class="ui-preview-label">{{ t('uiSystem.preview.hierarchy') }}</h3>
            <div class="ui-preview-actions">
              <q-btn color="primary" unelevated :label="t('uiSystem.preview.primaryAction')" @click="notice = t('uiSystem.preview.primaryNotice')" />
              <q-btn outline color="primary" :label="t('uiSystem.preview.secondaryAction')" @click="notice = t('uiSystem.preview.secondaryNotice')" />
              <q-btn flat color="primary" :label="t('uiSystem.preview.textAction')" @click="notice = t('uiSystem.preview.textNotice')" />
              <q-btn color="negative" outline :label="t('uiSystem.preview.dangerAction')" @click="dialogOpen = true" />
              <q-btn color="primary" unelevated :label="t('uiSystem.preview.unavailable')" disable />
              <q-btn color="primary" unelevated :label="t('uiSystem.preview.working')" loading />
              <q-btn round flat icon="info" :aria-label="t('uiSystem.preview.tooltipAction')"
                ><q-tooltip>{{ t('uiSystem.preview.tooltip') }}</q-tooltip></q-btn
              >
              <q-btn outline :label="t('uiSystem.preview.more')" icon-right="expand_more"
                ><q-menu
                  ><q-list>
                    <q-item v-close-popup clickable @click="notice = t('uiSystem.preview.detailsNotice')"
                      ><q-item-section avatar><q-icon name="visibility" /></q-item-section
                      ><q-item-section>{{ t('uiSystem.preview.viewDetails') }}</q-item-section></q-item
                    >
                    <q-item v-close-popup clickable @click="notice = t('uiSystem.preview.editNotice')"
                      ><q-item-section avatar><q-icon name="edit" /></q-item-section
                      ><q-item-section>{{ t('uiSystem.preview.edit') }}</q-item-section></q-item
                    >
                  </q-list></q-menu
                ></q-btn
              >
            </div>
            <q-banner v-if="notice" class="app-info-banner q-mt-md" role="status">{{ notice }}</q-banner>
          </q-card-section></q-card
        >
        <q-card flat
          ><q-card-section>
            <h3 class="ui-preview-label">{{ t('uiSystem.preview.choices') }}</h3>
            <div class="ui-preview-actions">
              <q-chip color="positive" icon="check_circle">{{ t('uiSystem.preview.availableLabel') }}</q-chip
              ><q-chip color="warning" icon="schedule">{{ t('uiSystem.preview.pendingLabel') }}</q-chip>
              <q-chip color="negative" icon="error_outline">{{ t('uiSystem.preview.failedLabel') }}</q-chip><q-chip color="info" icon="info">{{ t('uiSystem.preview.infoLabel') }}</q-chip>
            </div>
            <div class="ui-preview-actions q-mt-sm">
              <q-checkbox v-model="checked" :label="t('uiSystem.preview.selectItem')" /><q-toggle v-model="enabled" :label="t('uiSystem.preview.enableFeature')" />
              <q-radio v-model="radio" val="standard" :label="t('uiSystem.preview.standard')" /><q-radio
                v-model="radio"
                val="compact"
                :label="t('uiSystem.preview.compact')"
              />
            </div>
            <q-linear-progress
              :value="0.62"
              color="primary"
              class="q-mt-md"
              :aria-label="t('uiSystem.preview.progress')"
            /> </q-card-section
        ></q-card>
      </div>
    </section>

    <section class="ui-preview-section" aria-labelledby="preview-form">
      <div class="ui-preview-section__head">
        <h2 id="preview-form">{{ t('uiSystem.preview.inputs') }}</h2>
        <span>{{ t('uiSystem.preview.visibleLabels') }}</span>
      </div>
      <q-card flat
        ><q-card-section
          ><q-form @submit.prevent="notice = t('uiSystem.preview.formNotice')">
            <div class="ui-preview-fields">
              <q-input v-model="sampleText" outlined dense :label="t('uiSystem.preview.nameLabel')" :hint="t('uiSystem.preview.tabHint')" />
              <q-select
                v-model="sampleSelect"
                outlined
                dense
                :label="t('uiSystem.preview.stateLabel')"
                :options="selectOptions"
                emit-value
                map-options
              />
              <q-select
                v-model="selectedTags"
                multiple
                use-chips
                emit-value
                map-options
                outlined
                dense
                :label="t('uiSystem.preview.multiTags')"
                :options="tagOptions"
              />
              <q-input model-value="" outlined dense :label="t('uiSystem.preview.requiredLabel')" error :error-message="t('uiSystem.preview.requiredError')" />
              <q-input :model-value="t('uiSystem.preview.systemManagement')" outlined dense :label="t('uiSystem.preview.readonlyLabel')" readonly />
              <q-input :model-value="t('uiSystem.preview.lockedValue')" outlined dense :label="t('uiSystem.preview.disabledLabel')" disable />
            </div>
            <div class="app-actions-bar q-mt-md">
              <q-btn type="submit" color="primary" unelevated :label="t('uiSystem.preview.submit')" />
            </div> </q-form></q-card-section
      ></q-card>
    </section>

    <section class="ui-preview-section" aria-labelledby="preview-table">
      <div class="ui-preview-section__head">
        <h2 id="preview-table">{{ t('uiSystem.preview.lists') }}</h2>
        <span>{{ t('uiSystem.preview.tableHint') }}</span>
      </div>
      <div class="ui-preview-actions q-mb-md">
        <q-toggle v-model="showEmpty" :label="t('uiSystem.preview.empty')" /><q-toggle
          v-model="tableLoading"
          :label="t('uiSystem.preview.loading')"
        /><q-badge color="primary">{{ t('uiSystem.preview.selected', { count: selectedRows.length }) }}</q-badge>
      </div>
      <AppRegistryTable
        v-model:selected="selectedRows"
        :rows="showEmpty ? [] : tablePreview.rows"
        :columns="tablePreview.columns"
        :loading="tableLoading"
        row-key="id"
        selection="multiple"
        :pagination="{ rowsPerPage: 5 }"
        :external-pagination="false"
        :no-data-label="t('uiSystem.preview.noData')"
      >
        <template #body-cell-name="props"
          ><q-td :props="props"
            ><div class="app-registry-cell-primary">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub">{{ props.row.description }}</div></q-td
          ></template
        >
        <template #body-cell-state="props"
          ><q-td :props="props"
            ><q-chip dense :color="props.row.color">{{ props.row.state }}</q-chip></q-td
          ></template
        >
        <template #body-cell-actions="props"
          ><q-td :props="props"
            ><q-btn round flat dense icon="more_horiz" :aria-label="t('uiSystem.preview.rowAction', { name: props.row.name })"
              ><q-menu
                ><q-list
                  ><q-item v-close-popup clickable @click="notice = t('uiSystem.preview.rowNotice', { name: props.row.name })"
                    ><q-item-section>{{ t('uiSystem.preview.viewDetails') }}</q-item-section></q-item
                  ></q-list
                ></q-menu
              ></q-btn
            ></q-td
          ></template
        >
      </AppRegistryTable>
    </section>

    <section class="ui-preview-section" aria-labelledby="preview-tabs">
      <div class="ui-preview-section__head">
        <h2 id="preview-tabs">{{ t('uiSystem.preview.groups') }}</h2>
        <span>{{ t('uiSystem.preview.groupsHint') }}</span>
      </div>
      <div class="app-tab-shell">
        <q-tabs v-model="activeTab" align="left" no-caps
          ><q-tab name="overview" :label="t('uiSystem.preview.overview')" /><q-tab name="details" :label="t('uiSystem.preview.details')" /><q-tab
            name="history"
            :label="t('uiSystem.preview.history')"
        /></q-tabs>
      </div>
      <q-tab-panels v-model="activeTab">
        <q-tab-panel name="overview"
          ><q-banner class="bg-info"
            ><template #avatar><q-icon name="info_outline" /></template>{{ t('uiSystem.preview.information') }}</q-banner
          ></q-tab-panel
        >
        <q-tab-panel name="details"
          ><q-expansion-item :label="t('uiSystem.preview.expandDetails')" icon="article"
            ><q-card
              ><q-card-section>{{ t('uiSystem.preview.expansion') }}</q-card-section></q-card
            ></q-expansion-item
          ></q-tab-panel
        >
        <q-tab-panel name="history"
          ><q-banner class="bg-warning"
            ><template #avatar><q-icon name="schedule" /></template>{{ t('uiSystem.preview.waiting') }}</q-banner
          ></q-tab-panel
        >
      </q-tab-panels>
    </section>

    <q-dialog v-model="dialogOpen"
      ><q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between"
          ><h2 class="app-glass-dialog__title">{{ t('uiSystem.preview.confirmTitle') }}</h2>
          <q-btn v-close-popup flat round icon="close" :aria-label="t('uiSystem.preview.close')"
        /></q-card-section>
        <q-separator />
        <q-card-section class="app-glass-dialog__body"
          ><p class="ui-card-description">{{ t('uiSystem.preview.dialogDescription') }}</p>
          <q-input v-model="sampleText" outlined :label="t('uiSystem.preview.sampleName')" class="q-mt-md"
        /></q-card-section>
        <q-card-actions align="right" class="app-actions-bar"
          ><q-btn v-close-popup flat :label="t('uiSystem.preview.cancel')" /><q-btn
            v-close-popup
            color="primary"
            unelevated
            :label="t('uiSystem.preview.confirm')"
            @click="notice = t('uiSystem.preview.confirmNotice')"
        /></q-card-actions> </q-card
    ></q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppWorkspaceLaunchpad from '../components/layout/AppWorkspaceLaunchpad.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import { workspaceShortcuts } from '../config/workspaceShortcuts';
import { createThemePreviewUi, type ThemePreviewRow } from '../features/ui/themePreviewUi';
const $q = useQuasar();
const { t } = useI18n();
const tablePreview = computed(() => createThemePreviewUi(t));
const sampleText = ref('Aranea Workspace');
const sampleSelect = ref('active');
const selectedTags = ref(['design']);
const selectedRows = ref<ThemePreviewRow[]>([]);
const checked = ref(true);
const enabled = ref(true);
const radio = ref('standard');
const dialogOpen = ref(false);
const showEmpty = ref(false);
const tableLoading = ref(false);
const activeTab = ref('overview');
const notice = ref('');
const selectOptions = computed(() => [
  { label: t('uiSystem.preview.available'), value: 'active' },
  { label: t('uiSystem.preview.pending'), value: 'pending' },
]);
const tagOptions = computed(() =>
  ['sources', 'design', 'development'].map((value) => ({ label: t(`uiSystem.preview.${value}`), value })),
);
const launchpadItems = computed(() =>
  workspaceShortcuts.map((item) => ({
    ...item,
    title: t(`uiSystem.launchpad.${item.key}.title`),
    description: t(`uiSystem.launchpad.${item.key}.description`),
    action: t('uiSystem.launchpad.start'),
  })),
);
</script>
