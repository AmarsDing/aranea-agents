<template>
  <q-page class="app-page-cream hooks-page q-pa-md">
    <section class="hooks-hero row items-center justify-between q-mb-md">
      <div>
        <div class="hooks-kicker">Callback rules</div>
        <h1 class="hooks-title">Hook / Callback rules</h1>
        <p class="hooks-subtitle text-grey-7">
          Configure lifecycle hooks for Agent, Model, Tool, and Runner events (log, notify, block, modify).
        </p>
      </div>
      <q-btn color="primary" rounded unelevated icon="add" label="New Hook" @click="openCreate" />
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="search" class="col-12 col-md-6" dense outlined clearable debounce="200" label="Search">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-btn outline rounded icon="refresh" label="Refresh" :loading="loading" @click="loadRows" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="Retry" @click="loadRows" />
      </template>
    </q-banner>

    <q-table flat bordered :rows="filteredRows" :columns="columns" row-key="id" :loading="loading">
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
          <q-chip dense outline color="primary">{{ ruleOf(props.row).callback_point }}</q-chip>
          <q-chip dense outline>{{ ruleOf(props.row).action.type }}</q-chip>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)" />
          <q-btn flat dense round icon="delete" color="negative" @click="confirmDelete(props.row)" />
        </q-td>
      </template>
    </q-table>

    <q-dialog v-model="editorOpen" persistent maximized>
      <q-card>
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingId ? "Edit Hook" : "New Hook" }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <div class="row q-col-gutter-md">
            <q-input v-model="form.key" class="col-12 col-md-4" dense outlined label="Key" :disable="Boolean(editingId)" />
            <q-input v-model="form.name" class="col-12 col-md-4" dense outlined label="Name" />
            <q-toggle v-model="form.enabled" class="col-12 col-md-4" label="Enabled" />
          </div>
          <q-input v-model="form.description" dense outlined type="textarea" autogrow label="Description" />
          <callback-editor v-model="form.rule" v-model:sort-order="form.sort_order" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn color="primary" unelevated label="Save" :loading="saving" @click="saveHook" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import CallbackEditor from "../components/hooks/CallbackEditor.vue";
import { defaultHookRuleConfig, parseHookConfig, type HookRow, type HookRuleConfig } from "../features/hooks/types";
import { useHooksStore } from "../stores/hooks";

const $q = useQuasar();
const hooksStore = useHooksStore();
const { hooks: storeRows, loading: storeLoading } = storeToRefs(hooksStore);
const loading = storeLoading;
const saving = ref(false);
const error = ref("");
const search = ref("");
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
  { name: "name", label: "Name", field: "name", align: "left" as const },
  { name: "key", label: "Key", field: "key", align: "left" as const },
  { name: "rule", label: "Rule", field: "id", align: "left" as const },
  { name: "enabled", label: "Enabled", field: "enabled", align: "center" as const },
  { name: "actions", label: "Actions", field: "id", align: "right" as const }
];

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return rows.value;
  return rows.value.filter((r) => r.name.toLowerCase().includes(q) || r.key.toLowerCase().includes(q));
});

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

<style scoped>
.hooks-kicker {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6b7280;
}
.hooks-title {
  margin: 4px 0;
  font-size: 28px;
  font-weight: 700;
}
</style>
