// Self-improvement console API (73-self-iteration-v3, design §七).
// Wraps the proto-generated SelfImprovementService client and maps wire
// messages (all fields undefined-able) into clean domain types from ./types.
import { createSelfImprovementService } from '../../services/index';
import type { RiskRulesMsg, SelfImprovementRunMsg } from '../../services/kratos/self_improvement/v1/index';
import type {
  SIDiagnosis,
  SIGateResult,
  SIGovernanceDecision,
  SICriticReport,
  SIOutcomeStats,
  SIRiskRules,
  SIRiskRulesView,
  SIRun,
  SIRunDetail,
  SIRunFilter,
  SIStatus,
} from './types';

const si = createSelfImprovementService();

function str(v: string | undefined): string {
  return v ?? '';
}

function num(v: number | undefined): number {
  return Number(v ?? 0);
}

function mapRunBase(m: SelfImprovementRunMsg): SIRun {
  return {
    id: str(m.id),
    suggestionId: str(m.suggestionId),
    status: str(m.status) as SIRun['status'],
    triggerSource: str(m.triggerSource) as SIRun['triggerSource'],
    patchKind: str(m.patchKind) as SIRun['patchKind'],
    riskLevel: str(m.riskLevel) as SIRun['riskLevel'],
    baseRef: str(m.baseRef),
    branch: str(m.branch),
    diffStats: {
      files: num(m.diffStats?.files),
      additions: num(m.diffStats?.additions),
      deletions: num(m.diffStats?.deletions),
    },
    attempts: num(m.attempts),
    approvedBy: str(m.approvedBy),
    appliedCommit: str(m.appliedCommit),
    observeUntil: str(m.observeUntil),
    closedReason: str(m.closedReason),
    createdAt: str(m.createdAt),
    updatedAt: str(m.updatedAt),
  };
}

function mapDiagnosis(m: SelfImprovementRunMsg): SIDiagnosis | null {
  if (!m.diagnosis) return null;
  return {
    rootCause: str(m.diagnosis.rootCause),
    affectedFiles: m.diagnosis.affectedFiles ?? [],
    impactScope: str(m.diagnosis.impactScope),
    fixStrategy: str(m.diagnosis.fixStrategy),
    confidence: Number(m.diagnosis.confidence ?? 0),
  };
}

function mapGates(m: SelfImprovementRunMsg): SIGateResult[] {
  return (m.verificationReport ?? []).map((g) => ({
    gate: str(g.gate),
    passed: g.passed === true,
    output: str(g.output),
    durationMs: num(g.durationMs),
  }));
}

function mapCritic(m: SelfImprovementRunMsg): SICriticReport | null {
  if (!m.criticReport) return null;
  return {
    isSafe: m.criticReport.isSafe === true,
    riskLevel: str(m.criticReport.riskLevel),
    concerns: m.criticReport.concerns ?? [],
    suggestion: str(m.criticReport.suggestion),
  };
}

function mapGovernance(m: SelfImprovementRunMsg): SIGovernanceDecision | null {
  if (!m.governance) return null;
  return {
    riskLevel: str(m.governance.riskLevel),
    channel: str(m.governance.channel),
    ruleHits: m.governance.ruleHits ?? [],
  };
}

export async function listRuns(filter: SIRunFilter): Promise<{ items: SIRun[]; total: number }> {
  const resp = await si.ListRuns({
    status: filter.status || undefined,
    riskLevel: filter.riskLevel || undefined,
    triggerSource: filter.triggerSource || undefined,
    page: filter.page,
    pageSize: filter.pageSize,
  });
  return {
    items: (resp.items ?? []).map(mapRunBase),
    total: num(resp.total),
  };
}

export async function getRun(id: string): Promise<SIRunDetail> {
  const resp = await si.GetRun({ id });
  const m = resp.run;
  if (!m) throw new Error('run not found');
  return {
    ...mapRunBase(m),
    diff: str(m.diff),
    diagnosis: mapDiagnosis(m),
    verificationReport: mapGates(m),
    criticReport: mapCritic(m),
    governance: mapGovernance(m),
  };
}

export async function approveRun(id: string, reason?: string): Promise<void> {
  await si.ApproveRun({ id, reason: reason || undefined });
}

export async function rejectRun(id: string, reason: string): Promise<void> {
  await si.RejectRun({ id, reason });
}

export async function rollbackRun(id: string, reason?: string): Promise<void> {
  await si.RollbackRun({ id, reason: reason || undefined });
}

export async function closeRun(id: string, reason?: string): Promise<void> {
  await si.CloseRun({ id, reason: reason || undefined });
}

/** GetStatus — 功能可用性 + 前置条件自检（disabled 时也可调用）。 */
export async function getStatus(): Promise<SIStatus> {
  const r = await si.GetStatus({});
  return {
    enabled: !!r.enabled,
    refineLlmConfigured: !!r.refineLlmConfigured,
    refineLlmProvider: str(r.refineLlmProvider),
    refineLlmModel: str(r.refineLlmModel),
    repoRoot: str(r.repoRoot),
    repoRootValid: !!r.repoRootValid,
  };
}

function mapRiskRules(m: RiskRulesMsg | undefined): SIRiskRules {
  return {
    lowMaxLines: num(m?.lowMaxLines),
    mediumMaxLines: num(m?.mediumMaxLines),
    corePathGlobs: m?.corePathGlobs ?? [],
    dailyAutoQuota: num(m?.dailyAutoQuota),
  };
}

export async function getRiskRules(): Promise<SIRiskRulesView> {
  const resp = await si.GetRiskRules({});
  return { configured: mapRiskRules(resp.configured), effective: mapRiskRules(resp.effective) };
}

export async function updateRiskRules(rules: SIRiskRules): Promise<SIRiskRulesView> {
  const resp = await si.UpdateRiskRules({
    lowMaxLines: rules.lowMaxLines,
    mediumMaxLines: rules.mediumMaxLines,
    corePathGlobs: rules.corePathGlobs,
    dailyAutoQuota: rules.dailyAutoQuota,
  });
  return { configured: mapRiskRules(resp.configured), effective: mapRiskRules(resp.effective) };
}

export async function getOutcomeStats(): Promise<SIOutcomeStats> {
  const resp = await si.GetOutcomeStats({});
  return {
    total: num(resp.total),
    effective: num(resp.effective),
    neutral: num(resp.neutral),
    regressed: num(resp.regressed),
    effectiveRate: Number(resp.effectiveRate ?? 0),
    rollbackRate: Number(resp.rollbackRate ?? 0),
    byTrigger: (resp.byTrigger ?? []).map((t) => ({
      triggerSource: str(t.triggerSource),
      total: num(t.total),
      effective: num(t.effective),
      neutral: num(t.neutral),
      regressed: num(t.regressed),
    })),
  };
}
