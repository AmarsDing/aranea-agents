import { describe, expect, it } from 'vitest';
import {
  buildMonitorTracesQuery,
  isRunLifecycleEventType,
  RUN_LIVE_REFRESH_EVENT_TYPES,
  TRACE_DOMAIN_FILTERS,
  TRACE_STATUS_FILTERS,
} from '../tracesQuery';

describe('buildMonitorTracesQuery', () => {
  it('默认视图：exclude_internal=true，无 domain', () => {
    const q = buildMonitorTracesQuery({ keyword: '', status: '', domain: '', page: 1, pageSize: 12 });
    expect(q.exclude_internal).toBe(true);
    expect(q.domain).toBeUndefined();
    expect(q.limit).toBe(12);
    expect(q.offset).toBe(0);
  });

  it('显式 domain 优先于 exclude_internal', () => {
    const q = buildMonitorTracesQuery({ keyword: '', status: '', domain: 'system', page: 1, pageSize: 12 });
    expect(q.domain).toBe('system');
    expect(q.exclude_internal).toBeUndefined();
  });

  it('keyword/status 透传并 trim', () => {
    const q = buildMonitorTracesQuery({ keyword: '  助手 ', status: 'error', domain: 'chat', page: 1, pageSize: 12 });
    expect(q.keyword).toBe('助手');
    expect(q.status).toBe('error');
  });

  it('分页 offset 计算与负数防御', () => {
    expect(buildMonitorTracesQuery({ keyword: '', status: '', domain: '', page: 3, pageSize: 24 }).offset).toBe(48);
    expect(buildMonitorTracesQuery({ keyword: '', status: '', domain: '', page: 0, pageSize: 24 }).offset).toBe(0);
  });
});

describe('isRunLifecycleEventType', () => {
  it('运行生命周期事件命中', () => {
    for (const t of RUN_LIVE_REFRESH_EVENT_TYPES) {
      expect(isRunLifecycleEventType(t)).toBe(true);
    }
  });

  it('无关事件不命中', () => {
    expect(isRunLifecycleEventType('team_step_finished')).toBe(false);
    expect(isRunLifecycleEventType('intent_pass')).toBe(false);
    expect(isRunLifecycleEventType('')).toBe(false);
  });
});

describe('筛选常量', () => {
  it('domain 筛选含默认项与内部域', () => {
    expect(TRACE_DOMAIN_FILTERS).toContain('');
    expect(TRACE_DOMAIN_FILTERS).toContain('chat');
    expect(TRACE_DOMAIN_FILTERS).toContain('system');
  });

  it('status 筛选对齐后端枚举', () => {
    expect(TRACE_STATUS_FILTERS).toEqual(['', 'running', 'ok', 'error', 'timeout', 'interrupted', 'cancelled']);
  });
});
