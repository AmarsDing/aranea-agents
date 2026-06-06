<template>
  <q-page class="app-standard-page app-registry-page a2a-page">
    <AppPageHero
      kicker="Agent-to-Agent"
      title="A2A 管理"
      subtitle="发现 AgentCard、注册远程 Agent、查看调用审计、测试 Invoke（Admin 鉴权 + 工作区策略）。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="reload" />
      </template>
    </AppPageHero>

    <A2ARuntimeConfigBanner :config="runtimeConfig" />

    <div class="app-tab-shell">
      <q-tabs v-model="tab" dense align="left" class="text-primary">
        <q-tab name="discover" label="发现" />
        <q-tab name="gateway" label="Gateway" />
        <q-tab name="remote" label="远程注册" />
        <q-tab name="audit" label="审计" />
        <q-tab name="invoke" label="Invoke" />
      </q-tabs>
    </div>

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
      <q-tab-panel name="gateway" class="q-pa-none">
        <A2AGatewayPanel
          v-model:workspace="gatewayWorkspace"
          v-model:capability="gatewayCapability"
          v-model:check-health="gatewayCheckHealth"
          :entries="gatewayEntries"
          :loading="gatewayLoading"
          :columns="gatewayColumns"
          @discover="loadGateway"
        />
      </q-tab-panel>
      <q-tab-panel name="remote" class="q-pa-none q-gutter-md">
        <AppPageToolbar>
          <q-input
            v-model="remoteWorkspace"
            class="app-page-toolbar__field"
            dense
            outlined
            label="筛选工作区"
            hint="留空列出全部"
          />
          <template #actions>
            <q-btn outline no-caps color="primary" label="刷新列表" :loading="remoteLoading" @click="loadRemote" />
          </template>
        </AppPageToolbar>
        <A2ARemoteAgentPanel
          ref="remoteAgentPanelRef"
          :loading="remoteRegisterLoading"
          :discovering="remoteDiscoverLoading"
          :preview="remotePreview"
          @register="onRemoteRegister"
          @discover="previewRemote"
        />
        <AppRegistryTable
          row-key="id"
          :rows="remoteAgents"
          :columns="remoteColumns"
          :loading="remoteLoading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
          no-data-label="暂无远程注册"
        >
          <template #body-cell-display_name="props">
            <q-td :props="props">
              <AppRegistryHoverTip :text="props.row.remote_url" empty-label="暂无 URL">
                <span class="app-registry-cell-primary ellipsis">{{
                  props.row.display_name || props.row.remote_url || '—'
                }}</span>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-badge :color="props.row.enabled ? 'positive' : 'grey'" :label="props.row.enabled ? '启用' : '禁用'" />
            </q-td>
          </template>
          <template #body-cell-auth_type="props">
            <q-td :props="props">
              {{ a2aAuthTypeLabel(props.row.auth_type) }}
            </q-td>
          </template>
          <template #body-cell-healthy="props">
            <q-td :props="props">
              <q-badge
                v-if="props.row.last_health_at"
                :color="props.row.healthy ? 'positive' : 'negative'"
                :label="props.row.healthy ? '健康' : '异常'"
              />
              <span v-else class="text-grey-6">未探测</span>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn
                  flat
                  dense
                  round
                  color="negative"
                  icon="delete"
                  aria-label="删除"
                  @click="confirmRemoveRemote(props.row.id, props.row.display_name)"
                />
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
      </q-tab-panel>
      <q-tab-panel name="audit" class="q-pa-none">
        <A2AAuditPanel
          :rows="auditRows"
          :total="auditTotal"
          :loading="auditLoading"
          :columns="auditColumns"
          :status-color="auditStatusColor"
        />
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
import { ref } from 'vue';
import { useQuasar } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import A2ADiscoverPanel from '../components/a2a/A2ADiscoverPanel.vue';
import A2AAuditPanel from '../components/a2a/A2AAuditPanel.vue';
import A2AInvokePanel from '../components/a2a/A2AInvokePanel.vue';
import A2ARemoteAgentPanel from '../components/a2a/A2ARemoteAgentPanel.vue';
import A2ARuntimeConfigBanner from '../components/a2a/A2ARuntimeConfigBanner.vue';
import A2AGatewayPanel from '../components/a2a/A2AGatewayPanel.vue';
import { useA2APage } from '../features/a2a/useA2APage';
import { a2aAuthTypeLabel } from '../features/a2a/a2aTableUi';
import type { RegisterRemoteAgentInput } from '../features/a2a/types';

const {
  agents,
  auditRows,
  auditTotal,
  remoteAgents,
  gatewayEntries,
  loading,
  tab,
  auditLoading,
  invokeLoading,
  remoteLoading,
  remoteDiscoverLoading,
  remoteRegisterLoading,
  gatewayLoading,
  error,
  invokeResult,
  remotePreview,
  discoverWorkspace,
  discoverCapability,
  remoteWorkspace,
  gatewayWorkspace,
  gatewayCapability,
  gatewayCheckHealth,
  invokeForm,
  cardColumns,
  remoteColumns,
  auditColumns,
  gatewayColumns,
  auditStatusColor,
  loadDiscover,
  loadRemote,
  submitInvoke,
  submitRemoteRegister,
  previewRemote,
  removeRemote,
  loadGateway,
  reload,
  runtimeConfig,
} = useA2APage();

const $q = useQuasar();

const remoteAgentPanelRef = ref<InstanceType<typeof A2ARemoteAgentPanel> | null>(null);

async function onRemoteRegister(input: RegisterRemoteAgentInput) {
  await submitRemoteRegister(input);
  remoteAgentPanelRef.value?.resetForm();
}

function confirmRemoveRemote(id: string, name: string) {
  $q.dialog({
    title: '确认删除',
    message: `确定要删除远程 Agent「${name || id}」吗？`,
    cancel: true,
    persistent: true,
  }).onOk(() => {
    removeRemote(id);
  });
}
</script>
