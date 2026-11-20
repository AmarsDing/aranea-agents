<template>
  <!--
    Harness › 连接 › 跨实例协作。原来 6 个页签 + 联邦 4 个子页签按「要做什么」收成 4 个：
    目录（发现 + Gateway）· 接入对端（远程注册 + 联邦组织 / 目录）· 试调用（直接 + 联邦）· 调用记录（A2A + 联邦审计）。
  -->
  <q-page class="app-standard-page cap-page a2a-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :loading="loading"
        :aria-label="t('connect.common.refresh')"
        @click="onReload"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden && !hasAnyData" variant="forbidden" />

    <template v-else>
      <A2ARuntimeConfigBanner :config="runtimeConfig" :error="runtimeConfigError" @retry="reloadRuntimeConfig" />

      <CapStats :stats="stats" :label="t('connect.common.stripLabel')" :active="tab" @pick="pickStat" />

      <div class="cap-bar">
        <div class="cap-seg" role="tablist" :aria-label="t('menu.a2a')">
          <button
            v-for="item in tabs"
            :key="item.key"
            type="button"
            role="tab"
            :aria-selected="tab === item.key"
            :aria-pressed="tab === item.key"
            @click="tab = item.key"
          >
            {{ item.label }}
          </button>
        </div>
        <span class="cap-bar__spacer" />
        <div v-if="subTabs.length" class="cap-seg" role="group">
          <button
            v-for="item in subTabs"
            :key="item.key"
            type="button"
            :aria-pressed="sub[tab] === item.key"
            @click="sub[tab] = item.key"
          >
            {{ item.label }}
          </button>
        </div>
      </div>

      <!-- 一个区块读失败只在这里提示，不再把整页所有页签都挡掉（首次安装时会挡住「接入对端」） -->
      <AppStatePanel
        v-if="error"
        variant="partial"
        dense
        :description="error"
        :action-label="t('shell.common.retry')"
        action-icon="refresh"
        @action="onReload"
      />

      <template v-if="tab === 'directory'">
        <p v-if="!discover.agents.length && !loading" class="cap-note cap-note--info">
          <q-icon name="info_outline" size="16px" />
          <span class="cap-note__body">{{ t('connect.a2a.directoryWhat') }} {{ t('connect.a2a.directoryEmpty') }}</span>
        </p>
        <A2ADiscoverPanel
          v-if="sub.directory === 'list'"
          v-model:workspace="discover.discoverWorkspace"
          v-model:capability="discover.discoverCapability"
          :agents="discover.agents"
          :loading="loading"
          :columns="discover.cardColumns"
          @discover="discover.loadDiscover"
        />
        <A2AGatewayPanel
          v-else
          v-model:workspace="gateway.gatewayWorkspace"
          v-model:capability="gateway.gatewayCapability"
          v-model:check-health="gateway.gatewayCheckHealth"
          :entries="gateway.gatewayEntries"
          :loading="gateway.gatewayLoading"
          :columns="gateway.gatewayColumns"
          @discover="gateway.loadGateway"
        />
      </template>

      <template v-else-if="tab === 'peers'">
        <template v-if="sub.peers === 'remote'">
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
                  <AppStatusChip :status="props.row.enabled ? 'enabled' : 'disabled'" />
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
                      :aria-label="t('connect.common.remove')"
                      @click="remote.confirmRemoveRemote(props.row.id, props.row.display_name)"
                    />
                  </div>
                </q-td>
              </template>
            </AppRegistryTable>
          </div>
        </template>
        <template v-else>
          <p class="cap-note">
            <q-icon name="construction" size="16px" />
            <span class="cap-note__body">{{ t('connect.a2a.federationGap') }}</span>
          </p>
          <FederationOrgPanel
            v-if="sub.peers === 'orgs'"
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
          <FederationDirectoryPanel
            v-else
            v-model:capability="fedDirectory.dirCapability"
            v-model:org-id="fedDirectory.dirOrgId"
            :entries="fedDirectory.entries"
            :orgs="fedOrgs.list"
            :loading="fedDirectory.dirLoading"
            :columns="fedDirectory.dirColumns"
            @search="fedDirectory.loadDirectory"
          />
        </template>
      </template>

      <template v-else-if="tab === 'invoke'">
        <p class="cap-note cap-note--info">
          <q-icon name="info_outline" size="16px" />
          <span class="cap-note__body">{{ t('connect.a2a.invokeHint') }}</span>
        </p>
        <A2AInvokePanel
          v-if="sub.invoke === 'direct'"
          v-model:callee-agent-id="invoke.invokeForm.callee_agent_id"
          v-model:capability="invoke.invokeForm.capability"
          v-model:payload-json="invoke.invokeForm.payload_json"
          v-model:timeout-seconds="invoke.invokeForm.timeout_seconds"
          v-model:workspace="invoke.invokeForm.workspace"
          :discovered-agents="discover.agents"
          :loading="invoke.invokeLoading"
          :result="invoke.invokeResult"
          @invoke="onDirectInvoke"
        />
        <FederationInvokePanel
          v-else
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
      </template>

      <template v-else>
        <A2AAuditPanel
          v-if="sub.audit === 'direct'"
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
        <FederationAuditPanel
          v-else
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
      </template>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import CapStats from '../components/capability/CapStats.vue';
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

type TabKey = 'directory' | 'peers' | 'invoke' | 'audit';

const { t } = useI18n();
const {
  error,
  forbidden,
  loading,
  reload,
  runtimeConfig,
  runtimeConfigError,
  reloadRuntimeConfig,
  discover,
  invoke,
  audit,
  remote,
  gateway,
} = useA2APage();
const {
  fedTab,
  reload: reloadFederation,
  orgs: fedOrgs,
  directory: fedDirectory,
  invoke: fedInvoke,
  audit: fedAudit,
} = useFederationPage();

const tab = ref<TabKey>('directory');
const sub = reactive<Record<TabKey, string>>({ directory: 'list', peers: 'remote', invoke: 'direct', audit: 'direct' });

const tabs = computed(() => [
  { key: 'directory' as const, label: t('connect.a2a.tabDirectory') },
  { key: 'peers' as const, label: t('connect.a2a.tabPeers') },
  { key: 'invoke' as const, label: t('connect.a2a.tabInvoke') },
  { key: 'audit' as const, label: t('connect.a2a.tabAudit') },
]);

const subTabs = computed(() => {
  const map: Record<TabKey, Array<{ key: string; label: string }>> = {
    directory: [
      { key: 'list', label: t('connect.a2a.dirList') },
      { key: 'gateway', label: t('connect.a2a.dirGateway') },
    ],
    peers: [
      { key: 'remote', label: t('connect.a2a.peersRemote') },
      { key: 'orgs', label: t('connect.a2a.peersOrgs') },
      { key: 'directory', label: t('connect.a2a.peersDirectory') },
    ],
    invoke: [
      { key: 'direct', label: t('connect.a2a.invokeDirect') },
      { key: 'federated', label: t('connect.a2a.invokeFederated') },
    ],
    audit: [
      { key: 'direct', label: t('connect.a2a.auditDirect') },
      { key: 'federated', label: t('connect.a2a.auditFederated') },
    ],
  };
  return map[tab.value];
});

const stats = computed(() => [
  { key: 'directory', label: t('connect.a2a.statLocal'), value: discover.agents.length, clickable: true },
  { key: 'peers', label: t('connect.a2a.statRemote'), value: remote.remoteAgents.length, clickable: true },
  { key: 'peers-orgs', label: t('connect.a2a.statOrgs'), value: fedOrgs.list.length, clickable: false },
  { key: 'audit', label: t('connect.a2a.statCalls'), value: audit.auditTotal, clickable: true },
]);

function pickStat(key: string) {
  if (key === 'directory' || key === 'peers' || key === 'audit') tab.value = key;
}

const remoteAgentPanelRef = ref<InstanceType<typeof A2ARemoteAgentPanel> | null>(null);
const federationOrgPanelRef = ref<InstanceType<typeof FederationOrgPanel> | null>(null);

const hasAnyData = computed(
  () =>
    discover.agents.length > 0 ||
    gateway.gatewayEntries.length > 0 ||
    remote.remoteAgents.length > 0 ||
    fedOrgs.list.length > 0 ||
    fedDirectory.entries.length > 0 ||
    fedAudit.log.items.length > 0 ||
    audit.auditRows.length > 0,
);

// 联邦部分沿用 useFederationPage 的 fedTab 做刷新路由；Gateway 选中时才拉（带健康探测时每个远程最多等 10 秒）
watch(
  () => [tab.value, sub[tab.value]] as const,
  ([key, subKey]) => {
    if (key === 'peers' && subKey !== 'remote') fedTab.value = subKey === 'orgs' ? 'orgs' : 'directory';
    else if (key === 'invoke' && subKey === 'federated') fedTab.value = 'invoke';
    else if (key === 'audit' && subKey === 'federated') fedTab.value = 'audit';
    if (key === 'directory' && subKey === 'gateway' && !gateway.gatewayEntries.length) void gateway.loadGateway();
  },
);

const onFederation = computed(
  () =>
    (tab.value === 'peers' && sub.peers !== 'remote') || (tab.value !== 'directory' && sub[tab.value] === 'federated'),
);

function onReload() {
  if (onFederation.value) reloadFederation();
  else if (tab.value === 'directory')
    void (sub.directory === 'gateway' ? gateway.loadGateway() : discover.loadDiscover());
  else if (tab.value === 'peers') void remote.loadRemote();
  else if (tab.value === 'audit') void audit.loadAudit();
  else reload();
}

async function onRemoteRegister(input: RegisterRemoteAgentInput) {
  if (await remote.submitRemoteRegister(input)) remoteAgentPanelRef.value?.resetForm();
}

async function onFedRegister(input: RegisterFederationOrgInput) {
  const ok = await fedOrgs.submitRegister(input);
  if (ok) federationOrgPanelRef.value?.closeRegisterDialog();
}

async function onDirectInvoke() {
  await invoke.submitInvoke();
  void audit.loadAudit(); // 调用记录页签的总数与列表跟着刷新
}
</script>
