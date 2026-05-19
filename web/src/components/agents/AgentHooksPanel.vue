<template>
  <div class="agent-hooks-panel q-gutter-md">
    <div class="row items-center justify-between">
      <div class="text-body2 text-grey-7">
        为此 Agent 配置回调规则（<code>condition.agent_id</code> 预填为 ID 或 Key）。
      </div>
      <q-btn flat color="primary" icon="open_in_new" label="全局 Hook 管理" :to="{ name: 'hooks' }" />
    </div>

    <q-banner v-if="loadError" rounded class="bg-negative text-white">{{ loadError }}</q-banner>

    <q-table flat bordered :rows="scopedRows" :columns="columns" row-key="id" :loading="loading" hide-pagination :pagination="{ rowsPerPage: 20 }">
      <template #body-cell-rule="props">
        <q-td :props="props">
          <q-chip dense outline color="primary">{{ ruleOf(props.row).callback_point }}</q-chip>
          <q-chip dense outline>{{ ruleOf(props.row).action.type }}</q-chip>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round icon="edit" color="primary" @click="openEdit(props.row)" />
        </q-td>
      </template>
    </q-table>

    <q-expansion-item v-model="editorExpanded" dense-toggle icon="add" label="添加 Agent 回调规则" default-opened>
      <q-card flat bordered class="q-pa-md q-mt-sm">
        <callback-editor v-model="draftRule" v-model:sort-order="draftSort" :agent-id="agentId" :agent-key="agentKey" />
        <div class="row justify-end q-mt-md">
          <q-btn color="primary" unelevated label="创建 Hook" :loading="saving" @click="createScopedHook" />
        </div>
      </q-card>
    </q-expansion-item>

    <q-dialog v-model="editOpen" persistent>
      <q-card style="width: 720px; max-width: 94vw">
        <q-card-section class="text-h6">编辑 Hook</q-card-section>
        <q-separator />
        <q-card-section>
          <callback-editor v-if="editRow" v-model="editRule" v-model:sort-order="editSort" :agent-id="agentId" :agent-key="agentKey" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" v-close-popup />
          <q-btn color="primary" unelevated label="保存" :loading="saving" @click="saveEdit" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import CallbackEditor from "../hooks/CallbackEditor.vue";
import { defaultHookRuleConfig, parseHookConfig, type HookRow, type HookRuleConfig } from "../../features/hooks/types";
import { useHooksStore } from "../../stores/hooks";

const props = defineProps<{
  agentId: string;
  agentKey: string;
}>();

const $q = useQuasar();
const hooksStore = useHooksStore();
const loading = ref(false);
const saving = ref(false);
const loadError = ref("");
const rows = ref<HookRow[]>([]);
const editorExpanded = ref(true);
const draftRule = ref<HookRuleConfig>(defaultHookRuleConfig());
const draftSort = ref(0);
const editOpen = ref(false);
const editRow = ref<HookRow | null>(null);
const editRule = ref<HookRuleConfig>(defaultHookRuleConfig());
const editSort = ref(0);

const columns = [
  { name: "name", label: "名称", field: "name", align: "left" as const },
  { name: "rule", label: "规则", field: "id", align: "left" as const }
];

const scopedRows = computed(() => {
  const id = props.agentId.trim();
  const key = props.agentKey.trim();
  return rows.value.filter((row) => {
    const cond = ruleOf(row).condition?.agent_id?.trim() ?? "";
    if (!cond) return false;
    return cond === id || cond === key;
  });
});

function ruleOf(row: HookRow) {
  return parseHookConfig(row.config_json);
}

function resetDraft() {
  draftRule.value = defaultHookRuleConfig(props.agentId, props.agentKey);
  if (!draftRule.value.condition.agent_id) {
    draftRule.value.condition.agent_id = props.agentId || props.agentKey;
  }
  draftSort.value = 0;
}

watch(
  () => [props.agentId, props.agentKey],
  () => resetDraft(),
  { immediate: true }
);

async function loadRows() {
  loading.value = true;
  loadError.value = "";
  try {
    rows.value = await hooksStore.loadHooks();
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

async function createScopedHook() {
  const key = `${props.agentKey || props.agentId}-hook-${Date.now()}`.replace(/[^a-zA-Z0-9_-]/g, "_");
  saving.value = true;
  try {
    await hooksStore.addHook({
      key,
      name: `${props.agentKey || props.agentId} callback`,
      enabled: true,
      sort_order: draftSort.value,
      rule: draftRule.value
    });
    await loadRows();
    $q.notify({ type: "positive", message: "Hook 已创建" });
    resetDraft();
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
  } finally {
    saving.value = false;
  }
}

function openEdit(row: HookRow) {
  editRow.value = row;
  editRule.value = ruleOf(row);
  editSort.value = row.sort_order;
  editOpen.value = true;
}

async function saveEdit() {
  if (!editRow.value) return;
  saving.value = true;
  try {
    await hooksStore.saveHook(editRow.value.id, { sort_order: editSort.value, rule: editRule.value });
    editOpen.value = false;
    await loadRows();
    $q.notify({ type: "positive", message: "已保存" });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : String(e) });
  } finally {
    saving.value = false;
  }
}

onMounted(loadRows);
</script>
