/**
 * 语音助手会话绑定（M74 V9-T4，设计 74 §15.4-E）。
 *
 * 进入语音模式 = 与语音助手（__voice_butler__）对话。Turn 执行按会话
 * AgentID hydrate agent（评审 R5，TurnInput.AgentKey 仅透传），voice WS
 * 必须绑定 agent_id 属于语音助手的会话——仅改 voice.start 的 agent_key 无效。
 *
 * 本 composable 负责：
 * - 进入：选中语音助手的最近会话（复用现有会话选择体系）；无则纯 API 预建
 *   （不钉 provider/model——语音助手 turn 用 agent 配置的快速模型，
 *   onNewSession 路径会钉用户全局选型）后选中；
 * - 退出：恢复进入前选中的 agent/team 会话（语音是辅助模式，不吞掉用户
 *   原来的工作上下文）。
 */
import { useAppStore } from '../../../stores/app';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { Agent } from '../../agents/types';
import type { TeamRow } from '../../../components/chat/types';
import { createSession, listSessions } from '../../session/api';

export const VOICE_BUTLER_AGENT_KEY = '__voice_butler__';

type SelectionSnapshot =
  | { kind: 'agent'; agentId: string; sessionId?: string }
  | { kind: 'team'; teamId: string; sessionId?: string };

export type VoiceButlerBindingDeps = {
  selectAgent: (agent: Agent, options?: { sessionId?: string }) => Promise<void>;
  selectTeam: (team: TeamRow, options?: { sessionId?: string }) => Promise<void>;
  displayTeams: () => TeamRow[];
  /** 新建语音助手会话标题（i18n）。 */
  sessionTitle: () => string;
};

export type VoiceButlerBinding = {
  /** 进入语音模式：选中/创建语音助手会话，返回其 id；不可用/失败返回 null。 */
  bindVoiceButlerSession(): Promise<string | null>;
  /** 退出语音模式：恢复进入前的会话选择（无快照/已漂移则空操作）。 */
  restorePreviousSelection(): Promise<void>;
};

export function useVoiceButlerBinding(deps: VoiceButlerBindingDeps): VoiceButlerBinding {
  const appStore = useAppStore();
  const sessionStore = useChatSessionStore();
  let snapshot: SelectionSnapshot | null = null;

  function takeSnapshot(): void {
    snapshot =
      sessionStore.entityKind === 'team' && sessionStore.selectedTeamId
        ? {
            kind: 'team',
            teamId: sessionStore.selectedTeamId,
            sessionId: sessionStore.teamSelectedSessionId ?? undefined,
          }
        : appStore.selectedAgent
          ? {
              kind: 'agent',
              agentId: appStore.selectedAgent.id,
              sessionId: sessionStore.selectedSession?.id ?? undefined,
            }
          : null;
  }

  async function bindVoiceButlerSession(): Promise<string | null> {
    const butler = appStore.agents.find((a) => a.agent_key === VOICE_BUTLER_AGENT_KEY) ?? null;
    if (!butler) return null;
    // 幂等：已在语音助手会话（重入/手动选中）→ 直接复用，不打断既有快照
    if (
      sessionStore.entityKind === 'agent' &&
      appStore.selectedAgent?.agent_key === VOICE_BUTLER_AGENT_KEY &&
      sessionStore.selectedSession
    ) {
      return sessionStore.selectedSession.id;
    }
    try {
      const rows = await listSessions(butler.id, 1);
      // 新建会话未必排在展示排序首位，显式指定选中
      let preferredId: string | undefined;
      if (rows.length === 0) {
        const created = await createSession({ agent_id: butler.id, title: deps.sessionTitle() });
        preferredId = created.id;
      }
      takeSnapshot();
      await deps.selectAgent(butler, preferredId ? { sessionId: preferredId } : undefined);
      return sessionStore.selectedSession?.id ?? null;
    } catch (e) {
      console.warn('[voice] bind voice butler session failed', e);
      return null;
    }
  }

  async function restorePreviousSelection(): Promise<void> {
    const snap = snapshot;
    snapshot = null;
    if (!snap) return;
    // 语音模式中用户已手动切走 → 不打扰
    const stillOnButler =
      sessionStore.entityKind === 'agent' && appStore.selectedAgent?.agent_key === VOICE_BUTLER_AGENT_KEY;
    if (!stillOnButler) return;
    try {
      if (snap.kind === 'team') {
        const team = deps.displayTeams().find((item) => item.id === snap.teamId);
        if (team) await deps.selectTeam(team, snap.sessionId ? { sessionId: snap.sessionId } : undefined);
        return;
      }
      const agent = appStore.agents.find((item) => item.id === snap.agentId);
      if (agent) await deps.selectAgent(agent, snap.sessionId ? { sessionId: snap.sessionId } : undefined);
    } catch (e) {
      // 恢复失败静默降级：用户可手动切回，不阻断退出流程
      console.warn('[voice] restore previous selection failed', e);
    }
  }

  return { bindVoiceButlerSession, restorePreviousSelection };
}
