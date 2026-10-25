import { describe, it, expect } from 'vitest';
import {
  mapSpiritStatusToSession,
  spiritTeamStatusToLabel,
  spiritMemberStatusToLabel,
  STATUS_LABEL_CONFIG,
  type AgentNodeStatusLabel,
} from '../spiritUi';
import type { SpiritTeamStatus } from '../types';

const ALL_TEAM_STATUSES: SpiritTeamStatus[] = [
  'pending',
  'running',
  'paused',
  'completed',
  'partial_failure',
  'failed',
  'cancelled',
  'interrupted',
  'archived',
];

describe('spiritUi partial_failure 映射', () => {
  it('mapSpiritStatusToSession: partial_failure 会话徽标等同 completed（调度语义）', () => {
    expect(mapSpiritStatusToSession('partial_failure')).toBe('completed');
  });

  it('spiritTeamStatusToLabel: partial_failure 独立展示态', () => {
    expect(spiritTeamStatusToLabel('partial_failure')).toBe('partial_failure');
    // 展示语义必须与 completed 区分（部分失败 ≠ 完成）
    expect(spiritTeamStatusToLabel('partial_failure')).not.toBe(spiritTeamStatusToLabel('completed'));
  });

  it('STATUS_LABEL_CONFIG 含 partial_failure 展示配置', () => {
    const cfg = STATUS_LABEL_CONFIG.partial_failure;
    expect(cfg).toBeDefined();
    expect(cfg.text).toBe('部分失败');
    expect(cfg.animated).toBe(false);
  });

  it('全部 SpiritTeamStatus 在两个映射中均有定义（防新增状态漏配）', () => {
    for (const s of ALL_TEAM_STATUSES) {
      expect(mapSpiritStatusToSession(s), `mapSpiritStatusToSession(${s})`).toBeTruthy();
      const label: AgentNodeStatusLabel = spiritTeamStatusToLabel(s);
      expect(STATUS_LABEL_CONFIG[label], `STATUS_LABEL_CONFIG[${label}]`).toBeDefined();
    }
  });
});

// D10-F1（发现#23）：spiritMemberStatusToLabel 以 v2 词表
// （internal/biz/member_session.go MemberSessionStatus：pending/running/paused/
// completed/failed/skipped）为主键，legacy 词表（idle/error/waiting/blocked）
// 作为别名层保留。
describe('spiritMemberStatusToLabel（D10-F1：v2 词表权威）', () => {
  const V2_CASES: Array<[string, AgentNodeStatusLabel]> = [
    ['pending', 'queued'],
    ['running', 'active'],
    ['paused', 'suspended'],
    ['completed', 'done'],
    ['failed', 'failed'],
    ['skipped', 'skipped'],
  ];

  it.each(V2_CASES)('v2 词表 %s → %s', (status, expected) => {
    expect(spiritMemberStatusToLabel(status)).toBe(expected);
  });

  it('v2 终态/暂停值不再兜底为 queued（#23 失败先行锚点）', () => {
    for (const s of ['failed', 'skipped', 'paused'] as const) {
      expect(spiritMemberStatusToLabel(s), `${s} 不应回落到 queued`).not.toBe('queued');
    }
  });

  const LEGACY_CASES: Array<[string, AgentNodeStatusLabel]> = [
    ['idle', 'queued'],
    ['error', 'failed'],
    ['waiting', 'suspended'],
    ['blocked', 'tool_blocked'],
  ];

  it.each(LEGACY_CASES)('legacy 键 %s → %s 回归不变', (status, expected) => {
    expect(spiritMemberStatusToLabel(status)).toBe(expected);
  });

  it('未知字符串（含空串）→ queued 兜底不变', () => {
    expect(spiritMemberStatusToLabel('')).toBe('queued');
    expect(spiritMemberStatusToLabel('some_future_status')).toBe('queued');
  });
});
