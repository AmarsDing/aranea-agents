// web/src/features/chat/__tests__/v2Types.teamRunStatus.spec.ts
/**
 * D9-T：TeamRunStatus 联合补 'partial_failure'（对齐
 * internal/biz/team_run.go TeamRunV2StatusPartialFailure）。
 *  ① 类型断言：const s: TeamRunStatus = 'partial_failure' 编译通过
 *  ② listTeamRunsV2 映射回归：DTO status='partial_failure' 透传，不经 'running' 兜底
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockGet = vi.fn();
vi.mock('../../../services', () => ({
  kratosApi: { get: (...args: unknown[]) => mockGet(...args) },
}));

import type { TeamRunStatus } from '../v2Types';
import { listTeamRunsV2 } from '../../session/v2Api';

describe('TeamRunStatus partial_failure (D9-T)', () => {
  beforeEach(() => {
    mockGet.mockReset();
  });

  it('accepts partial_failure at the type level', () => {
    const s: TeamRunStatus = 'partial_failure';
    expect(s).toBe('partial_failure');
    // 既有取值不被收窄
    const legacy: TeamRunStatus = 'failed';
    expect(legacy).toBe('failed');
  });

  it('listTeamRunsV2 passes partial_failure through without the running fallback', async () => {
    mockGet.mockResolvedValue({
      data: { teamRuns: [{ id: 'tr-1', status: 'partial_failure', teamStageId: 'ts-1', taskId: 't-1' }] },
    });
    const runs = await listTeamRunsV2('ts-1');
    expect(runs[0]?.Status).toBe('partial_failure');
  });
});
