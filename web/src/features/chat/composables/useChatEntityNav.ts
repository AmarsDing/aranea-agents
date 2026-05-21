import { nextTick, ref, type Ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import {
  archiveSession,
  createSession,
  getSession,
  listSessionChatMessages as listMessages,
  listTeamSessions,
  restoreSession,
  updateSessionTitle,
} from "../../session/api";
import { listTeams, updateTeam } from "../../teams/api";
import { listPlatformResourceTree, type PlatformResourceTreeNode } from "../../platform/api";
import type { Agent, ChatEntityKind, Message, Session, TeamRow } from "../../../components/chat/types";
import { hydrateAgentSettings } from "../agentPlannerSettings";
import { formatSessionTime } from "./chatWorkspaceUtils";
import type { useAppStore } from "../../../stores/app";
import type { useChatStreamManager } from "./useChatStreamManager";

type Store = ReturnType<typeof useAppStore>;
type StreamManager = ReturnType<typeof useChatStreamManager>;

export type EntityNavDeps = {
  store: Store;
  streamManager: StreamManager;
  selectedEntityKind: Ref<ChatEntityKind>;
  selectedTeamId: Ref<string | null>;
  teamSelectedSessionId: Ref<string | null>;
  displayAgents: Ref<Agent[]>;
  displayTeams: Ref<TeamRow[]>;
  teamSessions: Ref<Record<string, Array<Session & { at: string }>>>;
  teamMessages: Ref<Record<string, Message[]>>;
  dialogMode: Ref<string>;
  selectedProviderModel: Ref<{ provider?: string; model?: string } | undefined>;
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
    const fromAgentList = deps.store.sessions.find((item) => item.id === sessionId);
    if (fromAgentList) return fromAgentList;
    if (deps.selectedTeamId.value) {
      const fromCurrentTeam = deps.teamSessions.value[deps.selectedTeamId.value]?.find(
        (item) => item.id === sessionId
      );
      if (fromCurrentTeam) return fromCurrentTeam;
    }
    for (const sessions of Object.values(deps.teamSessions.value)) {
      const hit = sessions.find((item) => item.id === sessionId);
      if (hit) return hit;
    }
    try {
      return await getSession(sessionId);
    } catch {
      return null;
    }
  }

  async function loadTeamSessions(teamID: string) {
    const rows = await listTeamSessions(teamID);
    deps.teamSessions.value[teamID] = rows.map((session) => ({
      ...session,
      at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
    }));
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
    if (deps.selectedEntityKind.value === "team") {
      deps.streamManager.disconnectTeamStream();
      for (const key of Object.keys(deps.teamMessages.value)) {
        delete deps.teamMessages.value[key];
      }
    }
    deps.selectedEntityKind.value = "agent";
    deps.selectedTeamId.value = null;
    deps.teamSelectedSessionId.value = null;
    const resolved = await hydrateAgentSettings(agent);
    deps.store.selectedAgent = resolved;
    deps.store.upsertAgent(resolved);
    await deps.store.loadSessions();
    await nextTick();
    const preferredId = options?.sessionId?.trim();
    deps.store.selectedSession = preferredId
      ? (deps.store.sessions.find((item) => item.id === preferredId) ?? deps.store.sessions[0] ?? null)
      : (deps.store.sessions[0] ?? null);
    deps.store.messages = [];
    if (deps.store.selectedSession) await deps.store.loadMessages();
  }

  async function selectTeam(team: TeamRow, options?: { sessionId?: string }) {
    if (deps.selectedTeamId.value && deps.selectedTeamId.value !== team.id) {
      deps.streamManager.disconnectTeamStream();
      for (const key of Object.keys(deps.teamMessages.value)) {
        delete deps.teamMessages.value[key];
      }
    }
    deps.selectedEntityKind.value = "team";
    deps.selectedTeamId.value = team.id;
    deps.store.selectedSession = null;
    deps.store.messages = [];
    await loadTeamSessions(team.id);
    const preferredId = options?.sessionId?.trim();
    const teamList = deps.teamSessions.value[team.id] ?? [];
    deps.teamSelectedSessionId.value = preferredId
      ? (teamList.find((item) => item.id === preferredId)?.id ?? preferredId)
      : (teamList[0]?.id ?? null);
    if (deps.teamSelectedSessionId.value) {
      deps.teamMessages.value[deps.teamSelectedSessionId.value] = await listMessages(
        deps.teamSelectedSessionId.value
      );
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
      if (deps.selectedEntityKind.value !== "team" || deps.selectedTeamId.value !== teamId) {
        await selectTeam(team, { sessionId });
        deps.streamManager.ensureTeamStream(sessionId);
        return;
      }
      deps.teamSelectedSessionId.value = sessionId;
      deps.teamMessages.value[sessionId] = await listMessages(sessionId);
      deps.streamManager.ensureTeamStream(sessionId);
      return;
    }

    const agentId = resolved.agent_id?.trim();
    if (!agentId) return;
    const agent =
      deps.store.agents.find((item) => item.id === agentId) ??
      deps.displayAgents.value.find((item) => item.id === agentId);
    if (!agent) {
      $q.notify({ type: "warning", message: "找不到该会话所属的 Agent" });
      return;
    }
    if (deps.selectedEntityKind.value !== "agent" || deps.store.selectedAgent?.id !== agentId) {
      await selectAgent(agent, { sessionId });
      deps.streamManager.ensureChatStream(sessionId);
      return;
    }

    const session = deps.store.sessions.find((item) => item.id === sessionId) ?? resolved;
    deps.store.selectedSession = session;
    deps.store.messages = [];
    if (session) {
      await deps.store.loadMessages();
      deps.streamManager.ensureChatStream(session.id);
    }
  }

  async function onRenameSession(payload: { id: string; title: string }) {
    const title = payload.title.trim();
    if (!title) return;
    try {
      if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
        const updated = await updateSessionTitle(payload.id, title);
        deps.teamSessions.value[deps.selectedTeamId.value] = (
          deps.teamSessions.value[deps.selectedTeamId.value] ?? []
        ).map((session) =>
          session.id === payload.id
            ? {
                ...updated,
                at: formatSessionTime(updated.last_message_at || updated.updated_at || updated.created_at),
              }
            : session
        );
        return;
      }
      await deps.store.renameSessionLocal(payload.id, title);
    } catch {
      $q.notify({ type: "negative", message: "重命名失败，请重试" });
    }
  }

  async function onRestoreSession(sessionId: string) {
    try {
      await restoreSession(sessionId);
      if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
        const sessions = deps.teamSessions.value[deps.selectedTeamId.value] ?? [];
        deps.teamSessions.value[deps.selectedTeamId.value] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "active" } : s
        ) as typeof sessions;
      }
    } catch (err) {
      console.error("Restore session failed", err);
    }
  }

  async function onArchiveSession(sessionId: string) {
    try {
      await archiveSession(sessionId);
      if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
        const sessions = deps.teamSessions.value[deps.selectedTeamId.value] ?? [];
        deps.teamSessions.value[deps.selectedTeamId.value] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "archived" } : s
        ) as typeof sessions;
      }
    } catch (err) {
      console.error("Archive session failed", err);
    }
  }

  function onSessionDetail(sessionId: string) {
    router.push({ name: "session-detail", params: { sessionId } });
  }

  async function onNewSession(title?: string) {
    if (deps.selectedEntityKind.value === "agent" && deps.store.selectedAgent) {
      const selectedModel = deps.selectedProviderModel.value;
      await deps.store.addSession(title || deps.t("chat.untitledSession"), {
        dialog_mode: deps.dialogMode.value,
        default_provider: selectedModel?.provider || deps.store.selectedAgent.provider,
        default_model: selectedModel?.model || deps.store.selectedAgent.model,
      });
      if (deps.store.selectedSession) {
        await deps.store.loadMessages();
        deps.streamManager.ensureChatStream(deps.store.selectedSession.id);
      }
      return;
    }

    if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
      const selectedModel = deps.selectedProviderModel.value;
      const created = await createSession({
        owner_type: "team",
        team_id: deps.selectedTeamId.value,
        title: title || deps.t("chat.untitledSession"),
        dialog_mode: deps.dialogMode.value,
        default_provider: selectedModel?.provider || "",
        default_model: selectedModel?.model || "",
      });
      deps.teamSessions.value[deps.selectedTeamId.value] = [
        {
          ...created,
          at: formatSessionTime(created.last_message_at || created.updated_at || created.created_at),
        },
        ...(deps.teamSessions.value[deps.selectedTeamId.value] ?? []),
      ];
      deps.teamSelectedSessionId.value = created.id;
      deps.teamMessages.value[created.id] = [];
      deps.streamManager.ensureTeamStream(created.id);
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

  return {
    categoryTree,
    loadCategoryTree,
    loadTeams,
    loadTeamSessions,
    selectAgent,
    selectTeam,
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
