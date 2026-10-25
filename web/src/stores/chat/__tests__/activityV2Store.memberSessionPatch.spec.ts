/**
 * patchMemberSessionStatusByChatSession 契约（D9-20）：
 *  ① running→paused 补丁生效且 Version 不变
 *  ② paused→running 补丁生效
 *  ③ 终态（completed/failed/skipped）为汇点，补丁拒绝
 *  ④ SessionID 不存在 / status 相同 → no-op
 *  ⑤ 补丁后更高 Version 的权威 upsert 仍能覆盖（仲裁复用既有 Version 守卫）
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../activityV2Store';
import type { MemberSession } from '../../../features/chat/v2Types';

function mkMemberSession(over: Partial<MemberSession> = {}): MemberSession {
  return {
    ID: 'ms-1',
    TeamRunID: 'tr-1',
    TeamStageID: 'ts-1',
    TaskID: 't-1',
    SessionID: 'cs-1',
    SpiritSessionID: 'sp-1',
    AgentKey: 'a-1',
    AgentName: 'Agent A',
    AvatarURL: '',
    Status: 'running',
    Seq: 1,
    Version: 1,
    StartedAt: '',
    FinishedAt: null,
    Error: '',
    ...over,
  };
}

describe('activityV2Store.patchMemberSessionStatusByChatSession (D9-20)', () => {
  let store: ReturnType<typeof useChatActivityStore>;

  beforeEach(() => {
    setActivePinia(createPinia());
    store = useChatActivityStore();
  });

  it('① running→paused 补丁生效且 Version 不变', () => {
    store.upsertMemberSession(mkMemberSession({ Status: 'running', Version: 3 }));
    const applied = store.patchMemberSessionStatusByChatSession('cs-1', 'paused');
    expect(applied).toBe(true);
    const ms = store.memberSessions.get('ms-1');
    expect(ms?.Status).toBe('paused');
    expect(ms?.Version).toBe(3);
  });

  it('② paused→running 补丁生效', () => {
    store.upsertMemberSession(mkMemberSession({ Status: 'paused', Version: 2 }));
    const applied = store.patchMemberSessionStatusByChatSession('cs-1', 'running');
    expect(applied).toBe(true);
    expect(store.memberSessions.get('ms-1')?.Status).toBe('running');
  });

  it.each(['completed', 'failed', 'skipped'] as const)('③ %s 为终态汇点，补丁拒绝', (terminal) => {
    store.upsertMemberSession(mkMemberSession({ Status: terminal, Version: 2 }));
    const applied = store.patchMemberSessionStatusByChatSession('cs-1', 'running');
    expect(applied).toBe(false);
    expect(store.memberSessions.get('ms-1')?.Status).toBe(terminal);
  });

  it('④ SessionID 不存在 / status 相同 / 非 paused|running 候选 → no-op', () => {
    store.upsertMemberSession(mkMemberSession({ Status: 'running', Version: 2 }));
    // SessionID 不存在
    expect(store.patchMemberSessionStatusByChatSession('cs-404', 'paused')).toBe(false);
    // status 相同
    expect(store.patchMemberSessionStatusByChatSession('cs-1', 'running')).toBe(false);
    // 非 paused/running 候选不被补丁旁路导入（权威终态只走 member_session.updated）
    expect(store.patchMemberSessionStatusByChatSession('cs-1', 'completed')).toBe(false);
    expect(store.memberSessions.get('ms-1')?.Status).toBe('running');
  });

  it('⑤ 补丁后再 upsertMemberSession 携带更高 Version 的 completed → 权威终态覆盖补丁', () => {
    store.upsertMemberSession(mkMemberSession({ Status: 'running', Version: 2 }));
    store.patchMemberSessionStatusByChatSession('cs-1', 'paused');
    expect(store.memberSessions.get('ms-1')?.Status).toBe('paused');

    store.upsertMemberSession(mkMemberSession({ Status: 'completed', Version: 3, FinishedAt: '2026-10-21T00:00:00Z' }));
    const ms = store.memberSessions.get('ms-1');
    expect(ms?.Status).toBe('completed');
    expect(ms?.Version).toBe(3);
  });
});
