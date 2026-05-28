<template>
  <div class="agent-hooks-panel">
    <div class="row items-center justify-between q-mb-md">
      <p class="agent-hooks-panel__hint q-ma-none">
        为此 Agent 配置回调规则（<code>condition.agent_id</code> 预填为 ID 或 Key）。
      </p>
      <q-btn flat rounded dense no-caps icon="open_in_new" label="全局 Hook" :to="{ name: 'hooks' }" />
    </div>

    <q-banner v-if="loadError" rounded class="bg-negative text-white">
      {{ loadError }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="reload" />
      </template>
    </q-banner>

    <HooksTable
      variant="agent"
      :rows="scopedRows"
      :loading="loading"
      @edit="openEdit"
    />

    <q-expansion-item v-model="editorExpanded" dense-toggle icon="add" label="添加 Agent 回调规则" default-opened>
      <q-card flat bordered class="q-pa-md q-mt-sm">
        <callback-editor v-model="draftRule" v-model:sort-order="draftSort" :agent-id="agentId" :agent-key="agentKey" />
        <div class="row justify-end q-mt-md">
          <q-btn color="primary" unelevated label="创建 Hook" :loading="saving" @click="createScopedHook" />
        </div>
      </q-card>
    </q-expansion-item>

    <q-dialog v-model="editOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">编辑 Hook</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body">
          <callback-editor v-if="editRow" v-model="editRule" v-model:sort-order="editSort" :agent-id="agentId" :agent-key="agentKey" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="保存" :loading="saving" @click="saveEdit" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import CallbackEditor from "../hooks/CallbackEditor.vue";
import HooksTable from "../hooks/HooksTable.vue";
import { useAgentHooksPanel } from "../../features/agents/useAgentHooksPanel";

const props = defineProps<{
  agentId: string;
  agentKey: string;
}>();

const {
  loading,
  saving,
  loadError,
  scopedRows,
  editorExpanded,
  draftRule,
  draftSort,
  editOpen,
  editRow,
  editRule,
  editSort,
  createScopedHook,
  openEdit,
  saveEdit,
  reload
} = useAgentHooksPanel(() => props.agentId, () => props.agentKey);
</script>
