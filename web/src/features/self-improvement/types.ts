// Self-improvement console domain types (73-self-iteration-v3, design §七/§八).
// Clean view models mapped from the proto-generated wire messages in api.ts —
// components must import types from here, never from the generated client.

export type SIRunStatus =
  | 'detected'
  | 'diagnosing'
  | 'patching'
  | 'verifying'
  | 'awaiting_governance'
  | 'applying'
  | 'applied'
  | 'observing'
  | 'closed'
  | 'rolled_back'
  | 'verify_failed'
  | 'rejected'
  | 'failed'
  | '';

export type SIRiskLevel = 'low' | 'medium' | 'high' | '';

export type SITriggerSource = 'error_cluster' | 'perf_bottleneck' | 'eval_regression' | 'test_failure' | '';

export type SIPatchKind = 'code' | 'config' | 'prompt' | 'docs' | 'i18n' | 'test' | '';

export interface SIDiffStats {
  files: number;
  additions: number;
  deletions: number;
}

export interface SIDiagnosis {
  rootCause: string;
  affectedFiles: string[];
  impactScope: string;
  fixStrategy: string;
  confidence: number;
}

export interface SIGateResult {
  gate: string;
  passed: boolean;
  output: string;
  durationMs: number;
}

export interface SICriticReport {
  isSafe: boolean;
  riskLevel: string;
  concerns: string[];
  suggestion: string;
}

export interface SIGovernanceDecision {
  riskLevel: string;
  channel: string; // auto / notify / approval / reject
  ruleHits: string[];
}

/** List view of one run (heavy fields omitted by the backend). */
export interface SIRun {
  id: string;
  suggestionId: string;
  status: SIRunStatus;
  triggerSource: SITriggerSource;
  patchKind: SIPatchKind;
  riskLevel: SIRiskLevel;
  baseRef: string;
  branch: string;
  diffStats: SIDiffStats;
  attempts: number;
  approvedBy: string;
  appliedCommit: string;
  observeUntil: string;
  closedReason: string;
  createdAt: string;
  updatedAt: string;
}

/** Full detail (GetRun) — adds diff/diagnosis/reports. */
export interface SIRunDetail extends SIRun {
  diff: string;
  diagnosis: SIDiagnosis | null;
  verificationReport: SIGateResult[];
  criticReport: SICriticReport | null;
  governance: SIGovernanceDecision | null;
}

export interface SIRunFilter {
  status?: string;
  riskLevel?: string;
  triggerSource?: string;
  page: number;
  pageSize: number;
}

export interface SITriggerOutcomeStats {
  triggerSource: string;
  total: number;
  effective: number;
  neutral: number;
  regressed: number;
}

export interface SIOutcomeStats {
  total: number;
  effective: number;
  neutral: number;
  regressed: number;
  effectiveRate: number;
  rollbackRate: number;
  byTrigger: SITriggerOutcomeStats[];
}

/**
 * Admin-configurable risk-classification rule set (design §六 D6).
 * Numeric 0 / empty globs mean "inherit the code default".
 */
export interface SIRiskRules {
  lowMaxLines: number;
  mediumMaxLines: number;
  corePathGlobs: string[];
  dailyAutoQuota: number;
}

/** Dual view returned by Get/UpdateRiskRules: raw stored vs normalized. */
export interface SIRiskRulesView {
  configured: SIRiskRules;
  effective: SIRiskRules;
}

/**
 * Feature availability + prerequisite preflight (GetStatus RPC, P5.5).
 * Answers even when the feature is disabled — the console renders the
 * disabled empty state / missing-prerequisite guidance from it.
 */
export interface SIStatus {
  /** self_improvement.enabled master switch. */
  enabled: boolean;
  /** DefaultRefineLLM configured (Analyst/Patcher/Critic hard dependency). */
  refineLlmConfigured: boolean;
  refineLlmProvider: string;
  refineLlmModel: string;
  /** Resolved sandbox repo root (config sandbox.repo_root, else process cwd). */
  repoRoot: string;
  /** Resolved root exists and contains .git. */
  repoRootValid: boolean;
}
