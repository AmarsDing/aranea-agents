/**
 * 联邦 A2A 网络 UI 常量与枚举映射（设计 F.9 / 需求 §子模块.5）。
 * 列定义为工厂函数（接收 vue-i18n 的 t），配合 composable 中 computed 使用。
 */
import type { ComposerTranslation } from 'vue-i18n';
import type { FederationAgentEntry, FederationAuditEntry, FederationOrg } from './federationTypes';
import { REGISTRY_COL_W, registryCol, registryColActions, type RegistryTableColumn } from '../ui/registryTableColumns';

// ---------- 信任等级 ----------

export function federationTrustLabel(t: ComposerTranslation, level: string): string {
  switch (level) {
    case 'trusted':
      return t('a2a.federation.trustTrusted');
    case 'untrusted':
      return t('a2a.federation.trustUntrusted');
    case 'neutral':
      return t('a2a.federation.trustNeutral');
    default:
      return level || '—';
  }
}

export function federationTrustColor(level: string): string {
  if (level === 'trusted') return 'positive';
  if (level === 'untrusted') return 'negative';
  return 'grey';
}

export function federationTrustOptions(t: ComposerTranslation) {
  return [
    { label: t('a2a.federation.trustTrusted'), value: 'trusted' },
    { label: t('a2a.federation.trustNeutral'), value: 'neutral' },
    { label: t('a2a.federation.trustUntrusted'), value: 'untrusted' },
  ];
}

// ---------- 组织状态 ----------

export function federationOrgStatusLabel(t: ComposerTranslation, status: string): string {
  if (status === 'active') return t('a2a.federation.statusActive');
  if (status === 'suspended') return t('a2a.federation.statusSuspended');
  return status || '—';
}

export function federationOrgStatusColor(status: string): string {
  return status === 'active' ? 'positive' : 'grey';
}

// ---------- 调用决策 ----------

export function federationDecisionLabel(t: ComposerTranslation, decision: string): string {
  switch (decision) {
    case 'allowed':
      return t('a2a.federation.decisionAllowed');
    case 'denied_trust':
      return t('a2a.federation.decisionDeniedTrust');
    case 'denied_policy':
      return t('a2a.federation.decisionDeniedPolicy');
    case 'denied_quota':
      return t('a2a.federation.decisionDeniedQuota');
    default:
      return decision || '—';
  }
}

export function federationDecisionColor(decision: string): string {
  return decision === 'allowed' ? 'positive' : 'negative';
}

export function federationDecisionFilterOptions(t: ComposerTranslation) {
  return [
    { label: t('a2a.federation.auditFilterAll'), value: '' },
    { label: t('a2a.federation.decisionAllowed'), value: 'allowed' },
    { label: t('a2a.federation.decisionDeniedTrust'), value: 'denied_trust' },
    { label: t('a2a.federation.decisionDeniedPolicy'), value: 'denied_policy' },
    { label: t('a2a.federation.decisionDeniedQuota'), value: 'denied_quota' },
  ];
}

// ---------- 审计状态 ----------

export function federationAuditStatusLabel(t: ComposerTranslation, status: string): string {
  switch (status) {
    case 'success':
      return t('a2a.federation.auditStatusSuccess');
    case 'error':
      return t('a2a.federation.auditStatusError');
    case 'timeout':
      return t('a2a.federation.auditStatusTimeout');
    case 'pending':
      return t('a2a.federation.auditStatusPending');
    default:
      return status || '—';
  }
}

export function federationAuditStatusColor(status: string): string {
  if (status === 'success') return 'positive';
  if (status === 'error' || status === 'timeout') return 'negative';
  return 'grey';
}

export function federationAuditStatusFilterOptions(t: ComposerTranslation) {
  return [
    { label: t('a2a.federation.auditFilterAll'), value: '' },
    { label: t('a2a.federation.auditStatusSuccess'), value: 'success' },
    { label: t('a2a.federation.auditStatusError'), value: 'error' },
    { label: t('a2a.federation.auditStatusTimeout'), value: 'timeout' },
    { label: t('a2a.federation.auditStatusPending'), value: 'pending' },
  ];
}

/** 审计条目中的组织显示：'local' 映射为本组织。 */
export function federationOrgDisplay(t: ComposerTranslation, orgID: string): string {
  return orgID === 'local' ? t('a2a.federation.localOrg') : orgID || '—';
}

// ---------- 列定义工厂 ----------

/** 联邦组织列表 */
export function federationOrgColumns(t: ComposerTranslation): RegistryTableColumn<FederationOrg>[] {
  return [
    registryCol<FederationOrg>('name', t('a2a.federation.orgColName'), 'name', 'left', REGISTRY_COL_W.name),
    registryCol<FederationOrg>('domain', t('a2a.federation.orgColDomain'), 'domain', 'left', REGISTRY_COL_W.name),
    registryCol<FederationOrg>(
      'trust_level',
      t('a2a.federation.orgColTrust'),
      'trust_level',
      'left',
      REGISTRY_COL_W.metric,
    ),
    registryCol<FederationOrg>('auth_type', t('a2a.federation.orgColAuth'), 'auth_type', 'left', REGISTRY_COL_W.metric),
    registryCol<FederationOrg>('status', t('a2a.federation.orgColStatus'), 'status', 'left', REGISTRY_COL_W.status),
    registryCol<FederationOrg>(
      'joined_at',
      t('a2a.federation.orgColJoinedAt'),
      'joined_at',
      'left',
      REGISTRY_COL_W.nameWide,
    ),
    registryColActions<FederationOrg>(REGISTRY_COL_W.actionsWide, ''),
  ];
}

/** 联邦目录列表 */
export function federationDirectoryColumns(t: ComposerTranslation): RegistryTableColumn<FederationAgentEntry>[] {
  return [
    registryCol<FederationAgentEntry>(
      'agent',
      t('a2a.federation.dirColAgent'),
      (row) => row.card.display_name || row.card.agent_id,
      'left',
      REGISTRY_COL_W.nameWide,
    ),
    registryCol<FederationAgentEntry>(
      'org',
      t('a2a.federation.dirColOrg'),
      (row) => row.org.name || row.org.domain,
      'left',
      REGISTRY_COL_W.name,
    ),
    registryCol<FederationAgentEntry>(
      'capabilities',
      t('a2a.federation.dirColCapabilities'),
      (row) => row.card.capabilities,
      'left',
      REGISTRY_COL_W.content,
    ),
    registryCol<FederationAgentEntry>(
      'remote_url',
      t('a2a.federation.dirColRemoteUrl'),
      (row) => row.remote_agent.remote_url,
      'left',
      REGISTRY_COL_W.content,
    ),
  ];
}

/** 联邦审计列表 */
export function federationAuditColumns(t: ComposerTranslation): RegistryTableColumn<FederationAuditEntry>[] {
  return [
    registryCol<FederationAuditEntry>(
      'created_at',
      t('a2a.federation.auditColTime'),
      'created_at',
      'left',
      REGISTRY_COL_W.nameWide,
    ),
    registryCol<FederationAuditEntry>(
      'caller',
      t('a2a.federation.auditColCaller'),
      (row) => row.caller_org_id,
      'left',
      REGISTRY_COL_W.name,
    ),
    registryCol<FederationAuditEntry>(
      'callee',
      t('a2a.federation.auditColCallee'),
      (row) => row.callee_org_id,
      'left',
      REGISTRY_COL_W.name,
    ),
    registryCol<FederationAuditEntry>(
      'capability',
      t('a2a.federation.auditColCapability'),
      'capability',
      'left',
      REGISTRY_COL_W.category,
    ),
    registryCol<FederationAuditEntry>(
      'decision',
      t('a2a.federation.auditColDecision'),
      'decision',
      'left',
      REGISTRY_COL_W.metric,
    ),
    registryCol<FederationAuditEntry>(
      'status',
      t('a2a.federation.auditColStatus'),
      'status',
      'left',
      REGISTRY_COL_W.status,
    ),
    registryCol<FederationAuditEntry>(
      'latency_ms',
      t('a2a.federation.auditColLatency'),
      'latency_ms',
      'right',
      REGISTRY_COL_W.metric,
    ),
    registryCol<FederationAuditEntry>(
      'error_message',
      t('a2a.federation.auditColError'),
      'error_message',
      'left',
      REGISTRY_COL_W.content,
    ),
  ];
}
