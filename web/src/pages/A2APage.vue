<template>
  <q-page class="app-standard-page app-registry-page a2a-page">
    <AppPageHero
      kicker="Agent-to-Agent"
      title="A2A 管理"
      subtitle="发现 AgentCard、注册远程 Agent、查看调用审计、测试 Invoke（Admin 鉴权 + 工作区策略）。"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="refresh"
          label="刷新"
          :loading="loading"
          @click="onReload"
        />
      </template>
    </AppPageHero>

    <A2ARuntimeConfigBanner :config="runtimeConfig" />

    <div class="app-tab-shell">
      <q-tabs v-model="tab" dense align="left" class="text-primary">
        <q-tab name="discover" label="发现" />
        <q-tab name="gateway" label="Gateway" />
        <q-tab name="remote" label="远程注册" />
        <q-tab name="federation" :label="t('a2a.federation.tab')" />
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
          v-model:workspace="discover.discoverWorkspace"
          v-model:capability="discover.discoverCapability"
          :agents="discover.agents"
          :loading="loading"
          :columns="discover.cardColumns"
          @discover="discover.loadDiscover"
        />
      </q-tab-panel>
      <q-tab-panel name="gateway" class="q-pa-none">
        <A2AGatewayPanel
          v-model:workspace="gateway.gatewayWorkspace"
          v-model:capability="gateway.gatewayCapability"
          v-model:check-health="gateway.gatewayCheckHealth"
          :entries="gateway.gatewayEntries"
          :loading="gateway.gatewayLoading"
          :columns="gateway.gatewayColumns"
          @discover="gateway.loadGateway"
        />
      </q-tab-panel>
      <q-tab-panel name="remote" class="q-pa-none">
        <AppPageToolbar>
          <q-input
            v-model="remote.remoteWorkspace"
            class="app-page-toolbar__field"
            dense
            outlined
            label="筛选工作区"
            hint="留空列出全部"
          />
          <template #actions>
            <q-btn
              outline
              no-caps
              color="primary"
              label="刷新列表"
              :loading="remote.remoteLoading"
              @click="remote.loadRemote"
            />
          </template>
        </AppPageToolbar>
        <div class="a2a-remote-split">
          <A2ARemoteAgentPanel
            ref="remoteAgentPanelRef"
            :loading="remote.remoteRegisterLoading"
            :discovering="remote.remoteDiscoverLoading"
            :preview="remote.remotePreview"
            @register="onRemoteRegister"
            @discover="remote.previewRemote"
          />
          <AppRegistryTable
            row-key="id"
            :rows="remote.remoteAgents"
            :columns="remote.remoteColumns"
            :loading="remote.remoteLoading"
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
                <q-badge
                  :color="props.row.enabled ? 'positive' : 'grey'"
                  :label="props.row.enabled ? '启用' : '禁用'"
                />
              </q-td>
            </template>
            <template #body-cell-auth_type="props">
              <q-td :props="props">
                {{ a2aAuthTypeLabel(props.row.auth_type) }}
              </q-td>
            </template>
            <template #body-cell-healthy="props">
              <q-td :props="props">
                <AppStatusChip
                  v-if="props.row.last_health_at"
                  :status="props.row.healthy ? 'healthy' : 'unhealthy'"
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
                    @click="remote.confirmRemoveRemote(props.row.id, props.row.display_name)"
                  />
                </div>
              </q-td>
            </template>
          </AppRegistryTable>
        </div>
      </q-tab-panel>
      <q-tab-panel name="federation" class="q-pa-none">
        <div class="app-tab-shell">
          <q-tabs v-model="fedTab" dense align="left" class="text-primary">
            <q-tab name="orgs" :label="t('a2a.federation.subOrgs')" />
            <q-tab name="directory" :label="t('a2a.federation.subDirectory')" />
            <q-tab name="invoke" :label="t('a2a.federation.subInvoke')" />
            <q-tab name="audit" :label="t('a2a.federation.subAudit')" />
          </q-tabs>
        </div>
        <q-tab-panels v-model="fedTab" animated>
          <q-tab-panel name="orgs" class="q-pa-none">
            <FederationOrgPanel
              ref="federationOrgPanelRef"
              :orgs="fedOrgs.list"
              :loading="fedOrgs.orgsLoading"
              :register-loading="fedOrgs.registerLoading"
              :syncing-org-id="fedOrgs.syncingOrgId"
              :columns="fedOrgs.orgColumns"
              @register="onFedRegister"
              @set-trust="fedOrgs.submitSetTrust"
              @sync="fedOrgs.syncOrg"
              @remove="fedOrgs.confirmRemoveOrg"
            />
          </q-tab-panel>
          <q-tab-panel name="directory" class="q-pa-none">
            <FederationDirectoryPanel
              v-model:capability="fedDirectory.dirCapability"
              v-model:org-id="fedDirectory.dirOrgId"
              :entries="fedDirectory.entries"
              :orgs="fedOrgs.list"
              :loading="fedDirectory.dirLoading"
              :columns="fedDirectory.dirColumns"
              @search="fedDirectory.loadDirectory"
            />
          </q-tab-panel>
          <q-tab-panel name="invoke" class="q-pa-none">
            <FederationInvokePanel
              v-model:org-id="fedInvoke.invokeForm.org_id"
              v-model:agent-id="fedInvoke.invokeForm.agent_id"
              v-model:capability="fedInvoke.invokeForm.capability"
              v-model:payload-json="fedInvoke.invokeForm.payload_json"
              v-model:timeout-seconds="fedInvoke.invokeForm.timeout_seconds"
              :entries="fedDirectory.entries"
              :loading="fedInvoke.invokeLoading"
              :result="fedInvoke.invokeResult"
              @invoke="fedInvoke.submitInvoke"
            />
          </q-tab-panel>
          <q-tab-panel name="audit" class="q-pa-none">
            <FederationAuditPanel
              v-model:callee-org-id="fedAudit.auditFilters.callee_org_id"
              v-model:decision="fedAudit.auditFilters.decision"
              v-model:status="fedAudit.auditFilters.status"
              :rows="fedAudit.log.items"
              :orgs="fedOrgs.list"
              :total="fedAudit.log.total"
              :loading="fedAudit.auditLoading"
              :columns="fedAudit.auditColumns"
              :page="fedAudit.auditPage"
              :page-size="fedAudit.auditPageSize"
              @search="fedAudit.onAuditSearch"
              @page-change="fedAudit.onAuditPage"
              @page-size-change="fedAudit.onAuditPageSize"
            />
          </q-tab-panel>
        </q-tab-panels>
      </q-tab-panel>
      <q-tab-panel name="audit" class="q-pa-none">
        <A2AAuditPanel
          :page="audit.auditPage"
          :page-size="audit.auditPageSize"
          :rows="audit.auditRows"
          :total="audit.auditTotal"
          :loading="audit.auditLoading"
          :columns="audit.auditColumns"
          :status-color="audit.auditStatusColor"
          @page-change="audit.onAuditPage"
          @page-size-change="audit.onAuditPageSize"
        />
      </q-tab-panel>
      <q-tab-panel name="invoke" class="q-pa-none">
        <A2AInvokePanel
          v-model:callee-agent-id="invoke.invokeForm.callee_agent_id"
          v-model:capability="invoke.invokeForm.capability"
          v-model:payload-json="invoke.invokeForm.payload_json"
          v-model:timeout-seconds="invoke.invokeForm.timeout_seconds"
          v-model:workspace="invoke.invokeForm.workspace"
          :discovered-agents="discover.agents"
          :loading="invoke.invokeLoading"
          :result="invoke.invokeResult"
          @invoke="invoke.submitInvoke"
        />
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
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
import FederationOrgPanel from '../components/a2a/federation/FederationOrgPanel.vue';
import FederationDirectoryPanel from '../components/a2a/federation/FederationDirectoryPanel.vue';
import FederationInvokePanel from '../components/a2a/federation/FederationInvokePanel.vue';
import FederationAuditPanel from '../components/a2a/federation/FederationAuditPanel.vue';
import { useA2APage } from '../features/a2a/useA2APage';
import { useFederationPage } from '../features/a2a/useFederationPage';
import { a2aAuthTypeLabel } from '../features/a2a/a2aTableUi';
import type { RegisterRemoteAgentInput } from '../features/a2a/types';
import type { RegisterFederationOrgInput } from '../features/a2a/federationTypes';

const { t } = useI18n();
const { tab, error, loading, reload, runtimeConfig, discover, invoke, audit, remote, gateway } = useA2APage();
const {
  fedTab,
  reload: reloadFederation,
  orgs: fedOrgs,
  directory: fedDirectory,
  invoke: fedInvoke,
  audit: fedAudit,
} = useFederationPage();

const remoteAgentPanelRef = ref<InstanceType<typeof A2ARemoteAgentPanel> | null>(null);
const federationOrgPanelRef = ref<InstanceType<typeof FederationOrgPanel> | null>(null);

async function onRemoteRegister(input: RegisterRemoteAgentInput) {
  await remote.submitRemoteRegister(input);
  remoteAgentPanelRef.value?.resetForm();
}

async function onFedRegister(input: RegisterFederationOrgInput) {
  const ok = await fedOrgs.submitRegister(input);
  if (ok) federationOrgPanelRef.value?.closeRegisterDialog();
}

function onReload() {
  if (tab.value === 'federation') reloadFederation();
  else reload();
}
</script>
