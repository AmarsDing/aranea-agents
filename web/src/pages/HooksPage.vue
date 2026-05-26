<template>
  <q-page class="app-standard-page app-registry-page hooks-page">
    <AppPageHero
      kicker="Callback rules"
      title="Hook / Callback rules"
      subtitle="Configure lifecycle hooks for Agent, Model, Tool, and Runner events (log, notify, block, modify)."
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="send" label="投递队列" to="/hooks/deliveries" />
        <q-btn outline rounded no-caps icon="history" label="运行记录" to="/plugins/runs" />
        <q-btn color="primary" rounded unelevated no-caps icon="add" label="New Hook" @click="openCreate" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="200" label="Search">
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
        label="回调点"
        :options="callbackPointOptions"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="Refresh" :loading="loading" @click="loadRows" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="Retry" @click="loadRows" />
      </template>
    </q-banner>

    <AppRegistryTable
      :rows="pagedRows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-toggle
            :model-value="props.row.enabled"
            color="primary"
            :disable="busyId === props.row.id"
            @update:model-value="toggleEnabled(props.row, Boolean($event))"
          />
        </q-td>
      </template>
      <template #body-cell-rule="props">
        <q-td :props="props">
          <div class="app-registry-chip-wrap">
          <q-chip dense outline color="primary">{{ ruleOf(props.row).callback_point }}</q-chip>
          <q-chip dense outline>{{ ruleOf(props.row).action.type }}</q-chip>
          </div>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
          <q-btn flat dense round icon="history" color="secondary" @click="openRuns(props.row)">
            <q-tooltip>查看阻断/错误记录</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)" />
          <q-btn flat dense round icon="delete" color="negative" @click="confirmDelete(props.row)" />
          </div>
        </q-td>
      </template>
    </AppRegistryTable>

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="filteredRows.length"
      :loading="loading"
      label="条 Hook"
    />

    <q-dialog v-model="editorOpen" persistent maximized>
      <q-card>
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingId ? "Edit Hook" : "New Hook" }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md app-form-wide">
          <div class="app-form-field-grid app-form-field-grid--2col">
            <q-input v-model="form.key" dense outlined label="Key" :disable="Boolean(editingId)" />
            <q-input v-model="form.name" dense outlined label="Name" />
            <q-toggle v-model="form.enabled" label="Enabled" />
          </div>
          <q-input v-model="form.description" class="app-field-long" dense outlined type="textarea" autogrow label="Description" />
          <callback-editor v-model="form.rule" v-model:sort-order="form.sort_order" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="Cancel" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="Save" :loading="saving" @click="saveHook" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import CallbackEditor from "../components/hooks/CallbackEditor.vue";
import { CALLBACK_POINT_OPTIONS } from "../features/callback/constants";
import { defaultHookRuleConfig, parseHookConfig, type HookRow, type HookRuleConfig } from "../features/hooks/types";
import { useHooksStore } from "../stores/hooks";
import { registryColWidth } from "../features/ui/registryTableColumns";

const $q = useQuasar();
const router = useRouter();
const hooksStore = useHooksStore();
const { hooks: storeRows, loading: storeLoading } = storeToRefs(hooksStore);
const loading = storeLoading;
const saving = ref(false);
const error = ref("");
const search = ref("");
const filterPoint = ref("");
const callbackPointOptions = CALLBACK_POINT_OPTIONS;
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

const columns = [
  { name: "name", label: "Name", field: "name", align: "left" as const, ...registryColWidth("14%") },
  { name: "key", label: "Key", field: "key", align: "left" as const, ...registryColWidth("14%") },
  { name: "rule", label: "Rule", field: "id", align: "left" as const, ...registryColWidth("13%") },
  { name: "enabled", label: "Enabled", field: "enabled", align: "center" as const, ...registryColWidth("64px") },
  { name: "actions", label: "Actions", field: "id", align: "right" as const, ...registryColWidth("108px") }
];

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

function openRuns(row: HookRow) {
  const rule = ruleOf(row);
  void router.push({
    path: "/plugins/runs",
    query: {
      plugin_key: `hook:${row.key}`,
      callback_point: rule.callback_point
    }
  });
}

function ruleOf(row: HookRow) {
  return parseHookConfig(row.config_json);
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
    $q.notify({ type: "warning", message: "Key and name are required" });
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
    await loadRows();
    $q.notify({ type: "positive", message: "Saved" });
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
    row.enabled = enabled;
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
  } finally {
    busyId.value = "";
  }
}

function confirmDelete(row: HookRow) {
  $q.dialog({
    title: "Delete Hook",
    message: `Delete ${row.name}?`,
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await hooksStore.removeHook(row.id);
    await loadRows();
  });
}

onMounted(loadRows);
</script>
