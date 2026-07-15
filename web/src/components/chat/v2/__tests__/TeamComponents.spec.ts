// web/src/components/chat/v2/__tests__/TeamComponents.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import MemberSessionPanel from '../MemberSessionPanel.vue';
import TeamRunCard from '../TeamRunCard.vue';
import type { MemberSession, TeamRun } from '../../../../features/chat/v2Types';

describe('v2 Team Components', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('MemberSessionPanel renders agent name', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: {
        memberSession: {
          ID: 'ms1',
          TeamRunID: 'tr1',
          TeamStageID: 'ts1',
          TaskID: 'tk1',
          SessionID: 'ms-sess',
          SpiritSessionID: 's1',
          AgentKey: 'coder',
          AgentName: 'Coder',
          AvatarURL: '',
          Status: 'running',
          Seq: 1,
          Version: 1,
          StartedAt: '',
          FinishedAt: null,
          Error: '',
        } as MemberSession,
      },
    });
    expect(wrapper.text()).toContain('Coder');
  });

  it('TeamRunCard renders member sessions', async () => {
    const store = useChatActivityStore();
    store.upsertMemberSession({
      ID: 'ms1',
      TeamRunID: 'tr1',
      TeamStageID: 'ts1',
      TaskID: 'tk1',
      SessionID: 'ms-sess',
      SpiritSessionID: 's1',
      AgentKey: 'coder',
      AgentName: 'Coder',
      AvatarURL: '',
      Status: 'completed',
      Seq: 1,
      Version: 1,
      StartedAt: '',
      FinishedAt: '',
      Error: '',
    } as MemberSession);
    const wrapper = mount(TeamRunCard, {
      props: {
        teamRun: {
          ID: 'tr1',
          TeamStageID: 'ts1',
          TaskID: 'tk1',
          SessionID: 's1',
          SpiritSessionID: 's1',
          DagNodeID: '',
          DependsOn: [],
          Status: 'completed',
          StartedAt: '',
          CompletedAt: '',
          Seq: 1,
          Version: 1,
          Error: '',
        } as TeamRun,
      },
    });
    expect(wrapper.text()).toContain('Coder');
  });
});
