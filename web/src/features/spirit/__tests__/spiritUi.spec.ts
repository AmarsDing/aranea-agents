import { describe, it, expect } from 'vitest';
import {
  mapSpiritStatusToSession,
  spiritTeamStatusToLabel,
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
