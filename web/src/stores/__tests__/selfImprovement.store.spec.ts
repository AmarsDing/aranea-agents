import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSelfImprovementStore } from '../selfImprovement';
import {
  approveRun,
  closeRun,
  getOutcomeStats,
  getRiskRules,
  getRun,
  getStatus,
  listRuns,
  rejectRun,
  rollbackRun,
  updateRiskRules,
} from '../../features/self-improvement/api';
import type { SIRiskRules, SIRun, SIRunDetail } from '../../features/self-improvement/types';

vi.mock('../../features/self-improvement/api', () => ({
  listRuns: vi.fn(),
  getRun: vi.fn(),
  approveRun: vi.fn(),
  rejectRun: vi.fn(),
  rollbackRun: vi.fn(),
  closeRun: vi.fn(),
  getOutcomeStats: vi.fn(),
  getRiskRules: vi.fn(),
  updateRiskRules: vi.fn(),
  getStatus: vi.fn(),
}));

const listRunsMock = vi.mocked(listRuns);
const getRunMock = vi.mocked(getRun);
const approveRunMock = vi.mocked(approveRun);
const rejectRunMock = vi.mocked(rejectRun);
const rollbackRunMock = vi.mocked(rollbackRun);
const closeRunMock = vi.mocked(closeRun);
const getOutcomeStatsMock = vi.mocked(getOutcomeStats);
const getRiskRulesMock = vi.mocked(getRiskRules);
const updateRiskRulesMock = vi.mocked(updateRiskRules);
const getStatusMock = vi.mocked(getStatus);

/** 模拟 Kratos HTTP 错误体（apierror.ToKratos：reason = Domain_Code）。 */
function kratosErr(status: number, reason: string | undefined, message: string): Error {
  const e = new Error(message) as Error & { response?: { status: number; data?: { reason?: string } } };
  e.response = { status, data: reason ? { reason } : undefined };
  return e;
}

function makeRun(over: Partial<SIRun> = {}): SIRun {
  return {
    id: 'run-1',
    suggestionId: 'sug-1',
    status: 'awaiting_governance',
    triggerSource: 'error_cluster',
    patchKind: 'code',
    riskLevel: 'high',
    baseRef: 'main',
    branch: 'si/run-1',
    diffStats: { files: 2, additions: 10, deletions: 3 },
    attempts: 1,
    approvedBy: '',
    appliedCommit: '',
    observeUntil: '',
    closedReason: '',
    createdAt: '2026-07-31T00:00:00Z',
    updatedAt: '2026-07-31T00:00:00Z',
    ...over,
  };
}

function makeDetail(over: Partial<SIRunDetail> = {}): SIRunDetail {
  return {
    ...makeRun(),
    diff: 'diff --git a/a.go b/a.go\n+added',
    diagnosis: {
      rootCause: 'nil deref',
      affectedFiles: ['a.go'],
      impactScope: 'biz',
      fixStrategy: 'guard',
      confidence: 0.9,
    },
    verificationReport: [{ gate: 'G1 build', passed: true, output: 'ok', durationMs: 1200 }],
    criticReport: { isSafe: true, riskLevel: 'low', concerns: [], suggestion: '' },
    governance: { riskLevel: 'high', channel: 'approval', ruleHits: ['R3'] },
    ...over,
  };
}

describe('useSelfImprovementStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('loads runs with total into state', async () => {
    listRunsMock.mockResolvedValue({ items: [makeRun()], total: 42 });
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 2, pageSize: 10, status: 'observing' });
    expect(listRunsMock).toHaveBeenCalledWith({ page: 2, pageSize: 10, status: 'observing' });
    expect(store.runs).toHaveLength(1);
    expect(store.runs[0].id).toBe('run-1');
    expect(store.total).toBe(42);
    expect(store.loading).toBe(false);
  });

  it('captures list errors into store.error', async () => {
    listRunsMock.mockRejectedValue(new Error('boom'));
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    expect(store.error).toBe('boom');
    expect(store.runs).toHaveLength(0);
  });

  it('loads run detail', async () => {
    getRunMock.mockResolvedValue(makeDetail());
    const store = useSelfImprovementStore();
    const detail = await store.loadRun('run-1');
    expect(detail?.diagnosis?.rootCause).toBe('nil deref');
    expect(store.detail?.verificationReport).toHaveLength(1);
  });

  it('loads outcome stats', async () => {
    getOutcomeStatsMock.mockResolvedValue({
      total: 10,
      effective: 6,
      neutral: 3,
      regressed: 1,
      effectiveRate: 0.6,
      rollbackRate: 0.1,
      byTrigger: [{ triggerSource: 'error_cluster', total: 10, effective: 6, neutral: 3, regressed: 1 }],
    });
    const store = useSelfImprovementStore();
    await store.loadOutcomeStats();
    expect(store.outcomeStats?.effectiveRate).toBe(0.6);
    expect(store.outcomeStats?.byTrigger).toHaveLength(1);
  });

  it('approve patches list row + open detail status', async () => {
    listRunsMock.mockResolvedValue({ items: [makeRun()], total: 1 });
    approveRunMock.mockResolvedValue(undefined);
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    store.detail = makeDetail();
    await store.approve('run-1', 'lgtm');
    expect(approveRunMock).toHaveBeenCalledWith('run-1', 'lgtm');
    expect(store.runs[0].status).toBe('applying');
    expect(store.detail?.status).toBe('applying');
  });

  it('reject marks row rejected', async () => {
    listRunsMock.mockResolvedValue({ items: [makeRun()], total: 1 });
    rejectRunMock.mockResolvedValue(undefined);
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    await store.reject('run-1', 'too risky');
    expect(rejectRunMock).toHaveBeenCalledWith('run-1', 'too risky');
    expect(store.runs[0].status).toBe('rejected');
  });

  it('rollback marks row rolled_back', async () => {
    listRunsMock.mockResolvedValue({ items: [makeRun({ status: 'observing' })], total: 1 });
    rollbackRunMock.mockResolvedValue(undefined);
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    await store.rollback('run-1');
    expect(store.runs[0].status).toBe('rolled_back');
  });

  it('close marks row closed', async () => {
    listRunsMock.mockResolvedValue({ items: [makeRun({ status: 'observing' })], total: 1 });
    closeRunMock.mockResolvedValue(undefined);
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    await store.close('run-1');
    expect(store.runs[0].status).toBe('closed');
  });

  it('mutation errors propagate to caller', async () => {
    approveRunMock.mockRejectedValue(new Error('conflict'));
    const store = useSelfImprovementStore();
    await expect(store.approve('run-x')).rejects.toThrow('conflict');
  });

  it('loads risk rules dual view', async () => {
    getRiskRulesMock.mockResolvedValue({
      configured: { lowMaxLines: 0, mediumMaxLines: 0, corePathGlobs: [], dailyAutoQuota: 0 },
      effective: { lowMaxLines: 100, mediumMaxLines: 300, corePathGlobs: ['internal/**'], dailyAutoQuota: 5 },
    });
    const store = useSelfImprovementStore();
    await store.loadRiskRules();
    expect(store.riskRules?.effective.lowMaxLines).toBe(100);
    expect(store.riskRules?.configured.corePathGlobs).toHaveLength(0);
    expect(store.rulesLoading).toBe(false);
  });

  it('saveRiskRules stores the returned view and propagates errors', async () => {
    const payload: SIRiskRules = { lowMaxLines: 80, mediumMaxLines: 250, corePathGlobs: ['cmd/**'], dailyAutoQuota: 3 };
    updateRiskRulesMock.mockResolvedValue({ configured: payload, effective: payload });
    const store = useSelfImprovementStore();
    await store.saveRiskRules(payload);
    expect(updateRiskRulesMock).toHaveBeenCalledWith(payload);
    expect(store.riskRules?.configured.mediumMaxLines).toBe(250);

    updateRiskRulesMock.mockRejectedValue(new Error('low > medium'));
    await expect(store.saveRiskRules(payload)).rejects.toThrow('low > medium');
  });

  // ── P5.5：feature availability / 错误分类 ────────────────────────────────

  it('503 SELF_IMPROVEMENT_UNAVAILABLE sets featureDisabled and clears error', async () => {
    listRunsMock.mockRejectedValue(kratosErr(503, 'SELF_IMPROVEMENT_UNAVAILABLE', 'not enabled'));
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    expect(store.featureDisabled).toBe(true);
    expect(store.error).toBeNull();
    expect(store.errorKind).toBe('');
  });

  it('loadStatus is authoritative: disabled probe sets featureDisabled=true', async () => {
    getStatusMock.mockResolvedValue({
      enabled: false,
      refineLlmConfigured: false,
      refineLlmProvider: '',
      refineLlmModel: '',
      repoRoot: '/repo',
      repoRootValid: true,
    });
    const store = useSelfImprovementStore();
    await store.loadStatus();
    expect(store.featureDisabled).toBe(true);
  });

  it('recheck on a still-disabled backend keeps the guided empty state', async () => {
    getStatusMock.mockResolvedValue({
      enabled: false,
      refineLlmConfigured: false,
      refineLlmProvider: '',
      refineLlmModel: '',
      repoRoot: '',
      repoRootValid: false,
    });
    const store = useSelfImprovementStore();
    await store.recheck({ page: 1, pageSize: 20 });
    expect(store.featureDisabled).toBe(true);
    // 仍 disabled 时不再调业务端点（避免无意义 503）。
    expect(listRunsMock).not.toHaveBeenCalled();
    expect(getOutcomeStatsMock).not.toHaveBeenCalled();
  });

  it('recheck on a now-enabled backend clears flag and loads data', async () => {
    getStatusMock.mockResolvedValue({
      enabled: true,
      refineLlmConfigured: true,
      refineLlmProvider: 'openai',
      refineLlmModel: 'gpt-x',
      repoRoot: '/repo',
      repoRootValid: true,
    });
    listRunsMock.mockResolvedValue({ items: [makeRun()], total: 1 });
    getOutcomeStatsMock.mockResolvedValue({
      total: 1,
      effective: 1,
      neutral: 0,
      regressed: 0,
      effectiveRate: 1,
      rollbackRate: 0,
      byTrigger: [],
    });
    const store = useSelfImprovementStore();
    store.featureDisabled = true;
    await store.recheck({ page: 1, pageSize: 20 });
    expect(store.featureDisabled).toBe(false);
    expect(store.runs).toHaveLength(1);
    expect(store.outcomeStats?.total).toBe(1);
  });

  it('404 without domain reason maps to legacy (old backend, route missing)', async () => {
    listRunsMock.mockRejectedValue(kratosErr(404, undefined, '404 page not found'));
    const store = useSelfImprovementStore();
    await store.loadRuns({ page: 1, pageSize: 20 });
    expect(store.errorKind).toBe('legacy');
  });

  it('404 with SELF_IMPROVEMENT_NOT_FOUND is a real not-found, not legacy', async () => {
    getRunMock.mockRejectedValue(kratosErr(404, 'SELF_IMPROVEMENT_NOT_FOUND', 'run run-x not found'));
    const store = useSelfImprovementStore();
    const detail = await store.loadRun('run-x');
    expect(detail).toBeNull();
    expect(store.errorKind).toBe('unknown'); // 回退原始 message，而非误报「后端版本过旧」
    expect(store.error).toBe('run run-x not found');
  });

  it('outcome stats failure degrades to statsFailed without touching main error', async () => {
    getOutcomeStatsMock.mockRejectedValue(kratosErr(500, 'SELF_IMPROVEMENT_INTERNAL', 'internal error'));
    const store = useSelfImprovementStore();
    await store.loadOutcomeStats();
    expect(store.statsFailed).toBe(true);
    expect(store.outcomeStats).toBeNull();
    expect(store.error).toBeNull(); // 不抢主错误条
  });

  it('outcome stats 503 SELF_IMPROVEMENT sets featureDisabled instead of statsFailed', async () => {
    getOutcomeStatsMock.mockRejectedValue(kratosErr(503, 'SELF_IMPROVEMENT_UNAVAILABLE', 'not enabled'));
    const store = useSelfImprovementStore();
    await store.loadOutcomeStats();
    expect(store.featureDisabled).toBe(true);
    expect(store.statsFailed).toBe(false);
  });

  it('loadStatus probe failure keeps current flag and nulls statusInfo', async () => {
    getStatusMock.mockRejectedValue(new Error('network down'));
    const store = useSelfImprovementStore();
    store.featureDisabled = true;
    await store.loadStatus();
    expect(store.statusInfo).toBeNull();
    expect(store.featureDisabled).toBe(true); // 探测失败不擅自改状态
  });
});
