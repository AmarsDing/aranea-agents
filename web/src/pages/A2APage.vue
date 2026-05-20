<template>
  <q-page class="app-page-cream a2a-page q-pa-sm q-pa-md-md">
    <section class="a2a-hero">
      <div>
        <div class="a2a-kicker">Agent-to-Agent</div>
        <h1 class="a2a-title">A2A 管理</h1>
        <p class="a2a-subtitle">
          发现 AgentCard、注册远程 Agent、查看调用审计、测试 Invoke（Admin 鉴权 + 工作区策略）。
        </p>
      </div>
      <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="reload" />
    </section>

    <A2ARuntimeConfigBanner />

    <q-tabs v-model="tab" dense align="left" class="text-primary q-mb-md">
      <q-tab name="discover" label="发现" />
      <q-tab name="remote" label="远程注册" />
      <q-tab name="audit" label="审计" />
      <q-tab name="invoke" label="Invoke" />
    </q-tabs>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="reload" />
      </template>
    </q-banner>

    <q-tab-panels v-model="tab" animated>
      <q-tab-panel name="discover" class="q-pa-none">
        <A2ADiscoverPanel
          v-model:workspace="discoverWorkspace"
          v-model:capability="discoverCapability"
          :agents="agents"
          :loading="loading"
          :columns="cardColumns"
          @discover="loadDiscover"
        />
      </q-tab-panel>
      <q-tab-panel name="remote" class="q-pa-none q-gutter-md">
        <div class="row q-col-gutter-md q-mb-md">
          <q-input
            v-model="remoteWorkspace"
            class="col-12 col-md-4"
            dense
            outlined
            label="筛选工作区"
            hint="留空列出全部"
          />
          <div class="col-12 col-md-4 flex items-center">
            <q-btn outline color="primary" label="刷新列表" :loading="remoteLoading" @click="loadRemote" />
          </div>
        </div>
        <A2ARemoteAgentPanel
          :loading="remoteLoading"
          :discovering="remoteDiscoverLoading"
          :preview="remotePreview"
          @register="submitRemoteRegister"
          @discover="previewRemote"
        />
        <q-table
          flat
          bordered
          row-key="id"
          title="已注册远程 Agent"
          :rows="remoteAgents"
          :columns="remoteColumns"
          :loading="remoteLoading"
          no-data-label="暂无远程注册"
        >
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-badge :color="props.row.enabled ? 'positive' : 'grey'" :label="props.row.enabled ? '启用' : '禁用'" />
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <q-btn flat dense color="negative" icon="delete" @click="removeRemote(props.row.id)" />
            </q-td>
          </template>
        </q-table>
      </q-tab-panel>
      <q-tab-panel name="audit" class="q-pa-none">
        <A2AAuditPanel :rows="auditRows" :loading="auditLoading" :columns="auditColumns" :status-color="auditStatusColor" />
      </q-tab-panel>
      <q-tab-panel name="invoke" class="q-pa-none">
        <A2AInvokePanel
          v-model:callee-agent-id="invokeForm.callee_agent_id"
          v-model:capability="invokeForm.capability"
          v-model:payload-json="invokeForm.payload_json"
          v-model:timeout-seconds="invokeForm.timeout_seconds"
          v-model:workspace="invokeForm.workspace"
          :discovered-agents="agents"
          :loading="invokeLoading"
          :result="invokeResult"
          @invoke="submitInvoke"
        />
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
import A2ADiscoverPanel from "../components/a2a/A2ADiscoverPanel.vue";
import A2AAuditPanel from "../components/a2a/A2AAuditPanel.vue";
import A2AInvokePanel from "../components/a2a/A2AInvokePanel.vue";
import A2ARemoteAgentPanel from "../components/a2a/A2ARemoteAgentPanel.vue";
import A2ARuntimeConfigBanner from "../components/a2a/A2ARuntimeConfigBanner.vue";
import { useA2APage } from "../features/a2a/useA2APage";

const {
  agents,
  auditRows,
  remoteAgents,
  loading,
  tab,
  auditLoading,
  invokeLoading,
  remoteLoading,
  remoteDiscoverLoading,
  error,
  invokeResult,
  remotePreview,
  discoverWorkspace,
  discoverCapability,
  remoteWorkspace,
  invokeForm,
  cardColumns,
  remoteColumns,
  auditColumns,
  auditStatusColor,
  loadDiscover,
  loadRemote,
  submitInvoke,
  submitRemoteRegister,
  previewRemote,
  removeRemote,
  reload
} = useA2APage();
</script>

<style scoped>
.a2a-hero {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}
.a2a-kicker {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--q-primary);
  font-weight: 600;
}
.a2a-title {
  margin: 0.25rem 0;
  font-size: 1.75rem;
  font-weight: 700;
}
.a2a-subtitle {
  margin: 0;
  color: #666;
  max-width: 42rem;
}
</style>
