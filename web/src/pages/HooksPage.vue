<template>
  <q-page class="app-standard-page app-registry-page hooks-page">
    <AppPageHero
      kicker="回调规则"
      title="Hook / 回调规则"
      subtitle="Configure lifecycle hooks for Agent, Model, Tool, and Runner events (log, notify, block, modify)."
    >
      <template #actions>
        <q-btn outline rounded no-caps icon="send" label="投递队列" to="/hooks/deliveries" />
        <q-btn outline rounded no-caps icon="history" label="运行记录" to="/hooks/deliveries" />
        <q-btn color="primary" rounded unelevated no-caps icon="add" label="新建 Hook" @click="openCreate" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="200" label="搜索">
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
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
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
        label="条 Hook"
      />
    </div>

    <q-dialog v-model="editorOpen" persistent maximized>
      <q-card>
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingId ? "编辑 Hook" : "新建 Hook" }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md app-form-wide">
          <div class="app-form-field-grid app-form-field-grid--2col">
            <q-input v-model="form.key" dense outlined label="标识" :disable="Boolean(editingId)" />
            <q-input v-model="form.name" dense outlined label="名称" />
            <q-toggle v-model="form.enabled" label="启用" />
          </div>
          <q-input v-model="form.description" class="app-field-long" dense outlined type="textarea" autogrow label="描述" />
          <callback-editor v-model="form.rule" v-model:sort-order="form.sort_order" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="保存" :loading="saving" :disable="!form.key?.trim() || !form.name?.trim()" @click="saveHook" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import CallbackEditor from "../components/hooks/CallbackEditor.vue";
import HooksTable from "../components/hooks/HooksTable.vue";
import { hookRuleOf } from "../components/hooks/hookTableUi";
import { CALLBACK_POINT_OPTIONS } from "../features/callback/constants";
import { defaultHookRuleConfig, type HookRow, type HookRuleConfig } from "../features/hooks/types";
import { useHooksStore } from "../stores/hooks";

const $q = useQuasar();
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
    $q.notify({ type: "warning", message: "标识和名称为必填" });
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
    $q.notify({ type: "positive", message: "已保存" });
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
    title: "删除 Hook",
    message: `确定删除「${row.name}」？`,
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await hooksStore.removeHook(row.id);
    await loadRows();
  });
}

onMounted(loadRows);
</script>
