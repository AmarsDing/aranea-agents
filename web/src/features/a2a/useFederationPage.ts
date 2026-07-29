/**
 * 联邦 A2A 网络 Tab 编排（设计 F.9）：组织 / 目录 / 调用 / 审计 四个子面板。
 * 数据全部经 useA2AStore 联邦 action；通知与确认框收口在此，面板保持纯展示。
 */
import { computed, onMounted, reactive, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useA2AStore } from '../../stores/a2a';
import { A2A_AUDIT_PAGE_SIZE_DEFAULT } from '../constants/queryLimits';
import type { FederatedInvokeResult, FederationOrg, RegisterFederationOrgInput } from './federationTypes';
import { federationAuditColumns, federationDirectoryColumns, federationOrgColumns } from './federationUi';

export function useFederationPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const a2aStore = useA2AStore();
  const { federationOrgs, federationAgents, federationAuditLog } = storeToRefs(a2aStore);

  const fedTab = ref('orgs');

  // ---------- 组织面板 ----------

  const orgsLoading = ref(false);
  const registerLoading = ref(false);
  const syncingOrgId = ref('');
  const orgColumns = computed(() => federationOrgColumns(t));

  async function loadOrgs() {
    orgsLoading.value = true;
    try {
      await a2aStore.loadFederationOrgs();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.orgLoadFailed') });
    } finally {
      orgsLoading.value = false;
    }
  }

  /** 注册组织；成功返回 true（Page 据此关闭 Dialog）。 */
  async function submitRegister(input: RegisterFederationOrgInput): Promise<boolean> {
    if (!input.name.trim() || !input.domain.trim()) {
      $q.notify({ type: 'warning', message: t('a2a.federation.orgNameRequired') });
      return false;
    }
    registerLoading.value = true;
    try {
      await a2aStore.registerFederation(input);
      $q.notify({ type: 'positive', message: t('a2a.federation.orgRegistered') });
      await loadDirectory();
      return true;
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.orgRegisterFailed') });
      return false;
    } finally {
      registerLoading.value = false;
    }
  }

  async function submitSetTrust(payload: { id: string; trust_level: string }) {
    try {
      await a2aStore.updateFederationTrust(payload.id, payload.trust_level);
      $q.notify({ type: 'positive', message: t('a2a.federation.orgTrustUpdated') });
      await loadDirectory();
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: e instanceof Error ? e.message : t('a2a.federation.orgTrustUpdateFailed'),
      });
    }
  }

  async function syncOrg(org: FederationOrg) {
    syncingOrgId.value = org.id;
    try {
      const synced = await a2aStore.syncFederationCards(org.id);
      $q.notify({ type: 'positive', message: t('a2a.federation.orgSynced', { count: synced }) });
      await loadDirectory();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.orgSyncFailed') });
    } finally {
      syncingOrgId.value = '';
    }
  }

  function confirmRemoveOrg(org: FederationOrg) {
    $q.dialog({
      title: t('a2a.federation.orgDeleteConfirmTitle'),
      message: t('a2a.federation.orgDeleteConfirmMessage', { name: org.name || org.domain }),
      cancel: true,
      persistent: true,
    }).onOk(() => {
      void removeOrg(org.id);
    });
  }

  async function removeOrg(id: string) {
    orgsLoading.value = true;
    try {
      await a2aStore.removeFederationOrg(id);
      $q.notify({ type: 'positive', message: t('a2a.federation.orgDeleted') });
      await loadDirectory();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.orgDeleteFailed') });
    } finally {
      orgsLoading.value = false;
    }
  }

  // ---------- 目录面板 ----------

  const dirCapability = ref('');
  const dirOrgId = ref('');
  const dirLoading = ref(false);
  const dirColumns = computed(() => federationDirectoryColumns(t));

  async function loadDirectory() {
    dirLoading.value = true;
    try {
      await a2aStore.loadFederationDirectory({
        capability: dirCapability.value.trim(),
        org_id: dirOrgId.value,
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.dirLoadFailed') });
    } finally {
      dirLoading.value = false;
    }
  }

  // ---------- 调用面板 ----------

  const invokeForm = ref({
    org_id: '',
    agent_id: '',
    capability: '',
    payload_json: '{}',
    timeout_seconds: 30,
  });
  const invokeLoading = ref(false);
  const invokeResult = ref<FederatedInvokeResult | null>(null);

  async function submitInvoke() {
    const form = invokeForm.value;
    if (!form.org_id || !form.agent_id || !form.capability) {
      $q.notify({ type: 'warning', message: t('a2a.federation.invokeSelectRequired') });
      return;
    }
    invokeLoading.value = true;
    invokeResult.value = null;
    try {
      invokeResult.value = await a2aStore.invokeFederated({
        org_id: form.org_id,
        agent_id: form.agent_id,
        capability: form.capability,
        payload_json: form.payload_json.trim() || '{}',
        timeout_seconds: form.timeout_seconds || 30,
      });
      if (fedTab.value === 'audit') await loadAudit();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.invokeFailed') });
    } finally {
      invokeLoading.value = false;
    }
  }

  // ---------- 审计面板 ----------

  const auditFilters = ref({ callee_org_id: '', decision: '', status: '' });
  const auditPage = ref(1);
  const auditPageSize = ref(A2A_AUDIT_PAGE_SIZE_DEFAULT);
  const auditLoading = ref(false);
  const auditColumns = computed(() => federationAuditColumns(t));

  async function loadAudit() {
    auditLoading.value = true;
    try {
      const limit = auditPageSize.value;
      const total = federationAuditLog.value.total;
      const maxPage = Math.max(1, Math.ceil(Math.max(0, total) / limit));
      if (total > 0 && auditPage.value > maxPage) auditPage.value = maxPage;
      await a2aStore.loadFederationAudit({
        callee_org_id: auditFilters.value.callee_org_id,
        decision: auditFilters.value.decision,
        status: auditFilters.value.status,
        limit,
        offset: (auditPage.value - 1) * limit,
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('a2a.federation.auditLoadFailed') });
    } finally {
      auditLoading.value = false;
    }
  }

  function onAuditSearch() {
    auditPage.value = 1;
    void loadAudit();
  }

  function onAuditPage(page: number) {
    if (page === auditPage.value) return;
    auditPage.value = page;
    void loadAudit();
  }

  function onAuditPageSize(pageSize: number) {
    auditPageSize.value = pageSize;
    auditPage.value = 1;
    void loadAudit();
  }

  // ---------- 汇总 ----------

  function reload() {
    if (fedTab.value === 'orgs') void loadOrgs();
    else if (fedTab.value === 'directory') void loadDirectory();
    else if (fedTab.value === 'audit') void loadAudit();
    else if (fedTab.value === 'invoke') void loadDirectory();
  }

  onMounted(() => {
    void loadOrgs();
    void loadDirectory();
    void loadAudit();
  });

  return {
    fedTab,
    reload,

    orgs: reactive({
      list: federationOrgs,
      orgsLoading,
      registerLoading,
      syncingOrgId,
      orgColumns,
      loadOrgs,
      submitRegister,
      submitSetTrust,
      syncOrg,
      confirmRemoveOrg,
    }),

    directory: reactive({
      entries: federationAgents,
      dirCapability,
      dirOrgId,
      dirLoading,
      dirColumns,
      loadDirectory,
    }),

    invoke: reactive({
      invokeForm,
      invokeLoading,
      invokeResult,
      submitInvoke,
    }),

    audit: reactive({
      log: federationAuditLog,
      auditFilters,
      auditPage,
      auditPageSize,
      auditLoading,
      auditColumns,
      loadAudit,
      onAuditSearch,
      onAuditPage,
      onAuditPageSize,
    }),
  };
}
