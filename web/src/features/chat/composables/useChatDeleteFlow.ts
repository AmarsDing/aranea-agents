import { computed, ref, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useTeamsStore } from "../../../stores/teams";
import type { DeleteKind, SessionView, TeamRow } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";
import { isAgentWorking, LS_AG_ORDER, LS_TM_ORDER } from "./chatWorkspaceUtils";
import type { useAppStore } from "../../../stores/app";
import type { useChatStore } from "../../../stores/chat";

type AppStore = ReturnType<typeof useAppStore>;
type ChatStore = ReturnType<typeof useChatStore>;

export type DeleteFlowDeps = {
  appStore: AppStore;
  chatStore: ChatStore;
  displayAgents: ReturnType<typeof ref<Agent[]>>;
  displayTeams: ReturnType<typeof ref<TeamRow[]>>;
  displaySessions: ComputedRef<SessionView[]>;
  defaultAgentId: ReturnType<typeof ref<string | null>>;
  selectTeam: (team: TeamRow) => Promise<void>;
};

export function useChatDeleteFlow(deps: DeleteFlowDeps) {
  const { t } = useI18n();
  const $q = useQuasar();

  const deleteOpen = ref(false);
  const deleteKind = ref<DeleteKind>("agent");
  const deleteTargetId = ref<string | null>(null);
  const deleteNameInput = ref("");
  const deleteBlockBusy = ref(false);
  const deleteBlockDefault = ref(false);
  const deleting = ref(false);

  const expectedDeleteName = computed(() => {
    if (deleteKind.value === "agent" && deleteTargetId.value) {
      return (
        deps.appStore.agents.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
        deps.displayAgents.value.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
        ""
      );
    }
    if (deleteKind.value === "team") {
      return deps.displayTeams.value.find((team) => team.id === deleteTargetId.value)?.display_name ?? "";
    }
    if (deleteKind.value === "session") {
      return deps.displaySessions.value.find((session) => session.id === deleteTargetId.value)?.title ?? "";
    }
    return "";
  });

  const deleteNameError = computed(
    () => deleteNameInput.value && deleteNameInput.value !== expectedDeleteName.value
  );

  const canConfirmDelete = computed(() => {
    if (deleteBlockBusy.value || deleteBlockDefault.value) return false;
    if (deleteKind.value === "all" || deleteKind.value === "session") return true;
    return deleteNameInput.value === expectedDeleteName.value;
  });

  const deleteTitleText = computed(() => {
    if (deleteKind.value === "agent") return t("chat.deleteTitleAgent");
    if (deleteKind.value === "team") return t("chat.deleteTitleTeam");
    if (deleteKind.value === "session") return t("chat.deleteTitleSession");
    return t("chat.deleteAllTitle");
  });

  function openDelete(kind: DeleteKind, id: string) {
    deleteKind.value = kind;
    deleteTargetId.value = id;
    deleteNameInput.value = "";
    deleteBlockBusy.value = false;
    deleteBlockDefault.value = false;

    if (kind === "agent" && id) {
      const agent =
        deps.appStore.agents.find((item) => item.id === id) ??
        deps.displayAgents.value.find((item) => item.id === id);
      deleteBlockBusy.value = agent ? isAgentWorking(agent) : false;
    }

    if (kind === "team" && id) {
      const team = deps.displayTeams.value.find((item) => item.id === id);
      if (team?.isDefault) {
        deleteBlockDefault.value = true;
        $q.notify({ type: "warning", message: t("chat.deleteBlockedDefault") });
        return;
      }
      deleteBlockBusy.value = team?.isWorking ?? false;
    }

    deleteOpen.value = true;
  }

  async function deleteSession(id: string) {
    if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
      await deps.chatStore.removeTeamSessionLocal(deps.chatStore.selectedTeamId, id);
      return;
    }
    await deps.chatStore.removeSessionLocal(id);
    if (deps.chatStore.selectedSession) {
      await deps.chatStore.loadMessages({ sessionId: deps.chatStore.selectedSession.id });
    }
  }

  async function clearSessions() {
    if (deps.chatStore.entityKind === "agent") {
      const agentId = deps.appStore.selectedAgent?.id;
      if (agentId) await deps.chatStore.clearAllAgentSessions(agentId);
      return;
    }
    if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
      deps.chatStore.clearTeamSessions(deps.chatStore.selectedTeamId);
    }
  }

  async function onConfirmDelete() {
    const id = deleteTargetId.value;
    if (deleteBlockBusy.value || deleteBlockDefault.value) return;

    if (deleteKind.value === "agent" && id) {
      deleting.value = true;
      try {
        localStorage.removeItem(LS_AG_ORDER);
        await deps.appStore.removeAgentFromList(id);
        deps.displayAgents.value = deps.displayAgents.value.filter((agent) => agent.id !== id);
        deps.defaultAgentId.value = deps.appStore.agents[0]?.id ?? null;
        if (deps.chatStore.entityKind === "agent") {
          if (deps.appStore.selectedAgent) {
            await deps.chatStore.loadAgentSessions(deps.appStore.selectedAgent.id);
            deps.chatStore.selectedSession = deps.chatStore.sessions[0] ?? null;
            if (deps.chatStore.selectedSession) {
              deps.chatStore.clearSessionMessages(deps.chatStore.selectedSession.id);
              await deps.chatStore.loadMessages({ sessionId: deps.chatStore.selectedSession.id });
            }
          } else if (deps.displayTeams.value[0]) {
            await deps.selectTeam(deps.displayTeams.value[0]);
          }
        }
      } finally {
        deleting.value = false;
      }
    } else if (deleteKind.value === "team" && id) {
      const teamsStore = useTeamsStore();
      await teamsStore.remove(id);
      localStorage.removeItem(LS_TM_ORDER);
      deps.displayTeams.value = deps.displayTeams.value.filter((team) => team.id !== id);
      if (deps.chatStore.selectedTeamId === id) deps.chatStore.selectedTeamId = null;
    } else if (deleteKind.value === "session" && id) {
      await deleteSession(id);
    } else if (deleteKind.value === "all") {
      await clearSessions();
    }

    deleteOpen.value = false;
    $q.notify({ type: "info", message: t("chat.deleteSuccess") });
  }

  return {
    deleteOpen,
    deleteKind,
    deleteTargetId,
    deleteNameInput,
    deleteBlockBusy,
    deleteBlockDefault,
    deleting,
    expectedDeleteName,
    deleteNameError,
    canConfirmDelete,
    deleteTitleText,
    openDelete,
    onConfirmDelete,
  };
}
