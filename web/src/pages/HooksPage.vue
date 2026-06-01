<template>
  <q-page class="app-standard-page app-registry-page hooks-page">
    <AppPageHero
      :kicker="t('hooksPage.kicker')"
      :title="t('hooksPage.title')"
      :subtitle="t('hooksPage.subtitle')"
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="send" :label="t('hooksPage.btnDeliveries')" to="/hooks/deliveries" />
        <q-btn outline rounded no-caps icon="history" :label="t('hooksPage.btnPluginRuns')" to="/plugins/runs" />
        <q-btn color="primary" rounded unelevated no-caps icon="add" :label="t('hooksPage.btnCreate')" @click="openCreate" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="200" :label="t('hooksPage.search')">
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
        <q-btn flat rounded no-caps icon="refresh" :label="t('hooksPage.btnRefresh')" :loading="loading" @click="loadRows" />
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
        :total="filteredRows.length"
        :loading="loading"
        :label="t('hooksPage.paginationLabel')"
      />
    </div>

    <q-dialog v-model="editorOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--xl app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ editingId ? t("hooksPage.dialogTitleEdit") : t("hooksPage.dialogTitleCreate") }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md app-form-wide">
            <div class="app-form-field-grid app-form-field-grid--2col">
              <q-input v-model="form.key" dense outlined :label="t('hooksPage.fieldKey')" :disable="Boolean(editingId)" />
              <q-input v-model="form.name" dense outlined :label="t('hooksPage.fieldName')" />
              <q-toggle v-model="form.enabled" :label="t('hooksPage.fieldEnabled')" />
            </div>
            <q-input v-model="form.description" class="app-field-long" dense outlined type="textarea" autogrow :label="t('hooksPage.fieldDescription')" />
            <callback-editor v-model="form.rule" v-model:sort-order="form.sort_order" />
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat no-caps :label="t('hooksPage.btnCancel')" v-close-popup />
          <q-btn color="primary" unelevated no-caps :label="t('hooksPage.btnSave')" :loading="saving" :disable="!form.key?.trim() || !form.name?.trim()" @click="saveHook" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import CallbackEditor from "../components/hooks/CallbackEditor.vue";
import HooksTable from "../components/hooks/HooksTable.vue";
import { hookRuleOf } from "../components/hooks/hookTableUi";
import { useCallbackPointOptions } from "../features/callback/constants";
import { defaultHookRuleConfig, type HookRow, type HookRuleConfig } from "../features/hooks/types";
import { useHooksStore } from "../stores/hooks";

const { t } = useI18n();
const $q = useQuasar();
const hooksStore = useHooksStore();
const { hooks: storeRows, loading: storeLoading } = storeToRefs(hooksStore);
const loading = storeLoading;
const saving = ref(false);
const error = ref("");
const search = ref("");
const filterPoint = ref("");
const callbackPointOptions = useCallbackPointOptions();
const rows = storeRows;
const editorOpen = ref(false);
const editingId = ref("");
const busyId = ref("");
const form = reactive({
  key: "",
  name: "",
  description: "",
  enabled: true,
  sort_order: 0,
  rule: defaultHookRuleConfig() as HookRuleConfig
});

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase();
  const point = filterPoint.value;
  return rows.value.filter((r) => {
    const rule = ruleOf(r);
    if (point && rule.callback_point !== point) return false;
    if (!q) return true;
    return r.name.toLowerCase().includes(q) || r.key.toLowerCase().includes(q);
  });
});

const page = ref(1);
const pageSize = ref(20);
const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

function resetFilters() {
  search.value = "";
  filterPoint.value = "";
  page.value = 1;
}

watch([search, filterPoint], () => {
  page.value = 1;
});

function ruleOf(row: HookRow) {
  return hookRuleOf(row);
}

async function loadRows() {
  error.value = "";
  try {
    await hooksStore.loadHooks();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}

function openCreate() {
  editingId.value = "";
  form.key = "";
  form.name = "";
  form.description = "";
  form.enabled = true;
  form.sort_order = 0;
  form.rule = defaultHookRuleConfig();
  editorOpen.value = true;
}

function openEdit(row: HookRow) {
  editingId.value = row.id;
  form.key = row.key;
  form.name = row.name;
  form.description = row.description;
  form.enabled = row.enabled;
  form.sort_order = row.sort_order;
  form.rule = ruleOf(row);
  editorOpen.value = true;
}

async function saveHook() {
  if (!form.key.trim() || !form.name.trim()) {
    $q.notify({ type: "warning", message: t("hooksPage.notifyRequired") });
    return;
  }
  saving.value = true;
  try {
    if (editingId.value) {
      await hooksStore.saveHook(editingId.value, {
        key: form.key,
        name: form.name,
        description: form.description,
        enabled: form.enabled,
        sort_order: form.sort_order,
        rule: form.rule
      });
    } else {
      await hooksStore.addHook({
        key: form.key,
        name: form.name,
        description: form.description,
        enabled: form.enabled,
        sort_order: form.sort_order,
        rule: form.rule
      });
    }
    editorOpen.value = false;
    $q.notify({ type: "positive", message: t("hooksPage.notifySaved") });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
  } finally {
    saving.value = false;
  }
}

async function toggleEnabled(row: HookRow, enabled: boolean) {
  busyId.value = row.id;
  try {
    await hooksStore.saveHook(row.id, { enabled });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
  } finally {
    busyId.value = "";
  }
}

function confirmDelete(row: HookRow) {
  $q.dialog({
    title: t("hooksPage.confirmDeleteTitle"),
    message: t("hooksPage.confirmDeleteMessage", { name: row.name }),
    cancel: true,
    persistent: true
  }).onOk(async () => {
    try {
      await hooksStore.removeHook(row.id);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
    }
  });
}

onMounted(loadRows);
</script>
