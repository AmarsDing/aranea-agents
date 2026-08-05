import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSelfImprovementStore } from '../selfImprovement';
import {
  approveRun,
  closeRun,
  getOutcomeStats,
  getRiskRules,
  getRun,
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
});
