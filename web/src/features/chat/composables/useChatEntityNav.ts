import { nextTick, ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { archiveSession, getSession, restoreSession } from "../../session/api";
import { listTeams, updateTeam } from "../../teams/api";
import { listPlatformResourceTree } from "../../platform/api";
import type { PlatformResourceTreeNode } from "../../platform/types";
import type { Agent, ChatEntityKind, Session, TeamRow } from "../../../components/chat/types";
import { hydrateAgentSettings } from "../agentPlannerSettings";
import { applyStreamingSnapshotToSession } from "../../../stores/chatStreamingSnapshots";
import type { useAppStore } from "../../../stores/app";
import type { useChatStore } from "../../../stores/chat";
import type { useChatStreamManager } from "./useChatStreamManager";

type AppStore = ReturnType<typeof useAppStore>;
type ChatStore = ReturnType<typeof useChatStore>;
type StreamManager = ReturnType<typeof useChatStreamManager>;

export type EntityNavDeps = {
  appStore: AppStore;
  chatStore: ChatStore;
  streamManager: StreamManager;
  displayAgents: ReturnType<typeof ref<Agent[]>>;
  displayTeams: ReturnType<typeof ref<TeamRow[]>>;
  dialogMode: ReturnType<typeof ref<string>>;
  selectedProviderModel: ReturnType<typeof ref<{ provider?: string; model?: string } | undefined>>;
  makeSessionTitle: (content: string) => string;
  t: (key: string, ...args: unknown[]) => string;
};

export function useChatEntityNav(deps: EntityNavDeps) {
  const $q = useQuasar();
  const router = useRouter();
  const categoryTree = ref<PlatformResourceTreeNode[]>([]);

  function sessionOwnerIsTeam(session: Session) {
    return session.owner_type === "team" || Boolean(session.team_id?.trim());
  }

  async function resolveSessionById(sessionId: string): Promise<Session | null> {
    const hit = deps.chatStore.findSessionById(sessionId);
    if (hit) return hit;
    try {
      return await getSession(sessionId);
    } catch {
      return null;
    }
  }

  async function loadTeamSessions(teamID: string) {
    await deps.chatStore.loadTeamSessions(teamID);
  }

  async function loadTeams() {
    try {
      const rows = await listTeams();
      deps.displayTeams.value = rows.map((team) => ({
        id: team.id,
        team_key: team.team_key,
        display_name: team.display_name,
        status: team.status,
        isDefault: team.is_default,
        isWorking: /work|run|busy|ing/i.test(team.status || ""),
        definition_json: team.definition_json,
      }));
    } catch {
      deps.displayTeams.value = [];
    }
  }

  async function loadCategoryTree() {
    categoryTree.value = await listPlatformResourceTree("agent-categories");
  }

  async function selectAgent(agent: Agent, options?: { sessionId?: string }) {
    if (deps.chatStore.entityKind === "team") {
      deps.streamManager.disconnectTeamStream();
      deps.chatStore.clearTeamMessageCache();
    }
    deps.chatStore.resetForAgentSwitch();
    const resolved = await hydrateAgentSettings(agent);
    deps.appStore.selectedAgent = resolved;
    deps.appStore.upsertAgent(resolved);
    await deps.chatStore.loadAgentSessions(resolved.id);
    await nextTick();
    const preferredId = options?.sessionId?.trim();
    deps.chatStore.selectedSession = preferredId
      ? (deps.chatStore.sessions.find((item) => item.id === preferredId) ?? deps.chatStore.sessions[0] ?? null)
      : (deps.chatStore.sessions[0] ?? null);
    if (deps.chatStore.selectedSession) {
      await deps.chatStore.loadMessages({ sessionId: deps.chatStore.selectedSession.id });
      applyStreamingSnapshotToSession(
        (sid) => deps.chatStore.getMessages(sid),
        (sid, rows) => deps.chatStore.setMessages(sid, rows),
        deps.chatStore.selectedSession.id
      );
      deps.streamManager.ensureChatStream(deps.chatStore.selectedSession.id);
    }
  }

  async function selectTeam(team: TeamRow, options?: { sessionId?: string }) {
    if (deps.chatStore.selectedTeamId && deps.chatStore.selectedTeamId !== team.id) {
      deps.streamManager.disconnectTeamStream();
      deps.chatStore.clearTeamMessageCache();
    }
    deps.chatStore.resetForTeamSwitch(team.id);
    await loadTeamSessions(team.id);
    const preferredId = options?.sessionId?.trim();
    const teamList = deps.chatStore.teamSessions[team.id] ?? [];
    deps.chatStore.teamSelectedSessionId = preferredId
      ? (teamList.find((item) => item.id === preferredId)?.id ?? preferredId)
      : (teamList[0]?.id ?? null);
    if (deps.chatStore.teamSelectedSessionId) {
      await deps.chatStore.loadMessages({ sessionId: deps.chatStore.teamSelectedSessionId, replace: true });
      deps.streamManager.ensureTeamStream(deps.chatStore.teamSelectedSessionId);
    }
  }

  async function focusAgentSessionView(sessionId: string, agentId: string) {
    const route = router.currentRoute.value;
    const routeSession = typeof route.query.session === "string" ? route.query.session.trim() : "";
    const routeAgent = typeof route.query.agent === "string" ? route.query.agent.trim() : "";
    const alreadyFocused =
      route.name === "chat" &&
      routeSession === sessionId &&
      routeAgent === agentId &&
      deps.chatStore.entityKind === "agent" &&
      deps.appStore.selectedAgent?.id === agentId &&
      deps.chatStore.selectedSession?.id === sessionId;

    if (alreadyFocused) {
      const session = deps.chatStore.sessions.find((item) => item.id === sessionId);
      if (session) deps.chatStore.selectedSession = session;
      deps.chatStore.clearSessionMessages(sessionId);
      await deps.chatStore.loadMessages({ sessionId });
      applyStreamingSnapshotToSession(
        (sid) => deps.chatStore.getMessages(sid),
        (sid, rows) => deps.chatStore.setMessages(sid, rows),
        sessionId
      );
      deps.streamManager.ensureChatStream(sessionId);
      return;
    }

    const query = { session: sessionId, agent: agentId };
    if (route.name === "chat") {
      await router.replace({ name: "chat", query });
    } else {
      await router.push({ name: "chat", query });
    }
  }

  async function onSelectSession(sessionId: string) {
    const resolved = await resolveSessionById(sessionId);
    if (!resolved) return;

    if (sessionOwnerIsTeam(resolved)) {
      const teamId = resolved.team_id?.trim();
      if (!teamId) return;
      const team = deps.displayTeams.value.find((item) => item.id === teamId);
      if (!team) {
        $q.notify({ type: "warning", message: "找不到该会话所属的 Team" });
        return;
      }
      if (deps.chatStore.entityKind !== "team" || deps.chatStore.selectedTeamId !== teamId) {
        await selectTeam(team, { sessionId });
        deps.streamManager.ensureTeamStream(sessionId);
        return;
      }
      deps.chatStore.teamSelectedSessionId = sessionId;
      await deps.chatStore.loadMessages({ sessionId, replace: true });
      deps.streamManager.ensureTeamStream(sessionId);
      return;
    }

    const agentId = resolved.agent_id?.trim();
    if (!agentId) return;
    const agent =
      deps.appStore.agents.find((item) => item.id === agentId) ??
      deps.displayAgents.value.find((item) => item.id === agentId);
    if (!agent) {
      $q.notify({ type: "warning", message: "找不到该会话所属的 Agent" });
      return;
    }
    await focusAgentSessionView(sessionId, agentId);
  }

  async function onRenameSession(payload: { id: string; title: string }) {
    const title = payload.title.trim();
    if (!title) return;
    try {
      if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
        await deps.chatStore.renameTeamSessionLocal(deps.chatStore.selectedTeamId, payload.id, title);
        return;
      }
      await deps.chatStore.renameSessionLocal(payload.id, title);
    } catch {
      $q.notify({ type: "negative", message: "重命名失败，请重试" });
    }
  }

  async function onRestoreSession(sessionId: string) {
    try {
      await restoreSession(sessionId);
      if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
        const teamId = deps.chatStore.selectedTeamId;
        const sessions = deps.chatStore.teamSessions[teamId] ?? [];
        deps.chatStore.teamSessions[teamId] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "active" } : s
        );
      }
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "恢复会话失败" });
    }
  }

  async function onArchiveSession(sessionId: string) {
    try {
      await archiveSession(sessionId);
      if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
        const teamId = deps.chatStore.selectedTeamId;
        const sessions = deps.chatStore.teamSessions[teamId] ?? [];
        deps.chatStore.teamSessions[teamId] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "archived" } : s
        );
      }
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "归档会话失败" });
    }
  }

  function onSessionDetail(sessionId: string) {
    router.push({ name: "session-detail", params: { sessionId } });
  }

  async function onNewSession(title?: string) {
    if (deps.chatStore.entityKind === "agent" && deps.appStore.selectedAgent) {
      const selectedModel = deps.selectedProviderModel.value;
      await deps.chatStore.addAgentSession(deps.appStore.selectedAgent.id, title || deps.t("chat.untitledSession"), {
        dialog_mode: deps.dialogMode.value,
        default_provider: selectedModel?.provider || deps.appStore.selectedAgent.provider,
        default_model: selectedModel?.model || deps.appStore.selectedAgent.model,
      });
      if (deps.chatStore.selectedSession) {
        await deps.chatStore.loadMessages({ sessionId: deps.chatStore.selectedSession.id });
        deps.streamManager.ensureChatStream(deps.chatStore.selectedSession.id);
      }
      return;
    }

    if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
      const selectedModel = deps.selectedProviderModel.value;
      const created = await deps.chatStore.addTeamSession(
        deps.chatStore.selectedTeamId,
        title || deps.t("chat.untitledSession"),
        {
          dialog_mode: deps.dialogMode.value,
          default_provider: selectedModel?.provider || "",
          default_model: selectedModel?.model || "",
        }
      );
      if (created) {
        deps.streamManager.ensureTeamStream(created.id);
      }
    }
  }

  async function openSettings(kind: ChatEntityKind, id: string) {
    if (kind === "agent") {
      await router.push(`/agents/${id}/settings`);
      return;
    }
    if (kind === "team") {
      await router.push({ name: "team", query: { edit: id } });
    }
  }

  async function focusSessionById(sessionId: string, agentId?: string) {
    const session = await resolveSessionById(sessionId);
    if (!session) return;
    const aid = agentId?.trim() || session.agent_id?.trim();
    if (!aid) return;
    const agent =
      deps.appStore.agents.find((a) => a.id === aid) ??
      deps.displayAgents.value.find((a) => a.id === aid);
    if (!agent) return;
    await selectAgent(agent, { sessionId });
  }

  return {
    categoryTree,
    loadCategoryTree,
    loadTeams,
    loadTeamSessions,
    selectAgent,
    selectTeam,
    focusSessionById,
    onSelectSession,
    onRenameSession,
    onRestoreSession,
    onArchiveSession,
    onSessionDetail,
    onNewSession,
    openSettings,
    updateTeam,
  };
}
