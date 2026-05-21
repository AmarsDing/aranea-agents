import { computed, ref, type ComputedRef, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { deleteTeam } from "../../teams/api";
import type { Agent, DeleteKind, SessionView, TeamRow } from "../../../components/chat/types";
import { isAgentWorking, LS_AG_ORDER, LS_TM_ORDER } from "./chatWorkspaceUtils";

type Store = ReturnType<typeof useAppStore>;

export type DeleteFlowDeps = {
  store: Store;
  displayAgents: Ref<Agent[]>;
  displayTeams: Ref<TeamRow[]>;
  displaySessions: ComputedRef<SessionView[]>;
  selectedEntityKind: Ref<"agent" | "team">;
  selectedTeamId: Ref<string | null>;
  teamSessions: Ref<Record<string, Array<{ id: string }>>>;
  teamSelectedSessionId: Ref<string | null>;
  teamMessages: Ref<Record<string, unknown[]>>;
  defaultAgentId: Ref<string | null>;
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
        deps.store.agents.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
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
        deps.store.agents.find((item) => item.id === id) ??
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
    if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
      deps.teamSessions.value[deps.selectedTeamId.value] = (
        deps.teamSessions.value[deps.selectedTeamId.value] ?? []
      ).filter((session) => session.id !== id);
      delete deps.teamMessages.value[id];
      if (deps.teamSelectedSessionId.value === id) {
        deps.teamSelectedSessionId.value =
          deps.teamSessions.value[deps.selectedTeamId.value]?.[0]?.id ?? null;
      }
      return;
    }
    await deps.store.removeSessionLocal(id);
    if (deps.store.selectedSession) await deps.store.loadMessages();
  }

  async function clearSessions() {
    if (deps.selectedEntityKind.value === "agent") {
      await deps.store.clearAllSessions();
      return;
    }
    if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
      deps.teamSessions.value[deps.selectedTeamId.value] = [];
      deps.teamSelectedSessionId.value = null;
      for (const sid of Object.keys(deps.teamMessages.value)) delete deps.teamMessages.value[sid];
    }
  }

  async function onConfirmDelete() {
    const id = deleteTargetId.value;
    if (deleteBlockBusy.value || deleteBlockDefault.value) return;

    if (deleteKind.value === "agent" && id) {
      deleting.value = true;
      try {
        localStorage.removeItem(LS_AG_ORDER);
        await deps.store.removeAgentFromList(id);
        deps.displayAgents.value = deps.displayAgents.value.filter((agent) => agent.id !== id);
        deps.defaultAgentId.value = deps.store.agents[0]?.id ?? null;
        if (deps.selectedEntityKind.value === "agent") {
          if (deps.store.selectedAgent) {
            await deps.store.loadSessions();
            deps.store.selectedSession = deps.store.sessions[0] ?? null;
            deps.store.messages = [];
            if (deps.store.selectedSession) await deps.store.loadMessages();
          } else if (deps.displayTeams.value[0]) {
            await deps.selectTeam(deps.displayTeams.value[0]);
          }
        }
      } finally {
        deleting.value = false;
      }
    } else if (deleteKind.value === "team" && id) {
      await deleteTeam(id);
      localStorage.removeItem(LS_TM_ORDER);
      deps.displayTeams.value = deps.displayTeams.value.filter((team) => team.id !== id);
      if (deps.selectedTeamId.value === id) deps.selectedTeamId.value = null;
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
