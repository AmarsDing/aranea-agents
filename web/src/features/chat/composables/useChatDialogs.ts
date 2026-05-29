import { reactive } from "vue";
import type { ComputedRef, Ref } from "vue";
import type { useChatDeleteFlow } from "./useChatDeleteFlow";
import type { useChatSettingsDialog } from "./useChatSettingsDialog";
import type { ChatEventInspectorStreamDeps } from "./useChatEventInspector";
import type { SessionInspectorTab } from "../../../components/chat/SessionTimelineDialog.vue";
import type { PlatformResource } from "../../platform/types";

export interface ChatDialogsDeps {
  deleteFlow: ReturnType<typeof useChatDeleteFlow>;
  settingsDialog: ReturnType<typeof useChatSettingsDialog>;
  traceOpen: Ref<boolean>;
  traceSessionId: Ref<string | null>;
  traceSessionTitle: Ref<string>;
  traceInitialTab: Ref<SessionInspectorTab>;
  traceStreamDeps: ComputedRef<ChatEventInspectorStreamDeps>;
  selectedProviderModel: ComputedRef<PlatformResource | undefined>;
  fileSupported: ComputedRef<boolean>;
  onSaveSettings: () => Promise<void>;
}

export function useChatDialogs(deps: ChatDialogsDeps) {
  const { deleteFlow, settingsDialog, onSaveSettings } = deps;

  return reactive({
    settingsOpen: settingsDialog.settingsOpen,
    settingsMode: settingsDialog.settingsMode,
    settingsId: settingsDialog.settingsId,
    editName: settingsDialog.editName,
    editKey: settingsDialog.editKey,
    editProvider: settingsDialog.editProvider,
    editModel: settingsDialog.editModel,
    settingsSaving: settingsDialog.settingsSaving,
    settingsTitle: settingsDialog.settingsTitle,
    onSaveSettings,
    deleteOpen: deleteFlow.deleteOpen,
    deleteKind: deleteFlow.deleteKind,
    deleteTargetId: deleteFlow.deleteTargetId,
    deleteNameInput: deleteFlow.deleteNameInput,
    deleteBlockBusy: deleteFlow.deleteBlockBusy,
    deleteBlockDefault: deleteFlow.deleteBlockDefault,
    deleting: deleteFlow.deleting,
    expectedDeleteName: deleteFlow.expectedDeleteName,
    deleteNameError: deleteFlow.deleteNameError,
    canConfirmDelete: deleteFlow.canConfirmDelete,
    deleteTitleText: deleteFlow.deleteTitleText,
    onConfirmDelete: deleteFlow.onConfirmDelete,
    traceOpen: deps.traceOpen,
    traceSessionId: deps.traceSessionId,
    traceSessionTitle: deps.traceSessionTitle,
    traceInitialTab: deps.traceInitialTab,
    traceStreamDeps: deps.traceStreamDeps,
    selectedProviderModel: deps.selectedProviderModel,
    fileSupported: deps.fileSupported,
  });
}
