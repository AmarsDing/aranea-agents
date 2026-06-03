import { computed, ref, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { ChatEntityKind, TeamRow } from '../../../components/chat/types';
import type { Agent } from '../../agents/types';
import type { useAppStore } from '../../../stores/app';

type Store = ReturnType<typeof useAppStore>;

export function useChatSettingsDialog(store: Store, displayAgents: Ref<Agent[]>, displayTeams: Ref<TeamRow[]>) {
  const { t } = useI18n();
  const $q = useQuasar();

  const settingsOpen = ref(false);
  const settingsMode = ref<ChatEntityKind | null>(null);
  const settingsId = ref<string | null>(null);
  const editName = ref('');
  const editKey = ref('');
  const editProvider = ref('');
  const editModel = ref('');
  const settingsSaving = ref(false);

  const settingsTitle = computed(() => {
    if (settingsMode.value === 'agent') return t('chat.settingsTitleAgent');
    if (settingsMode.value === 'team') return t('chat.settingsTitleTeam');
    return t('chat.settings');
  });

  async function onSaveSettings(updateTeam: (id: string, body: object) => Promise<TeamRow>) {
    settingsSaving.value = true;
    try {
      if (settingsMode.value === 'agent' && settingsId.value) {
        const agent =
          store.agents.find((item) => item.id === settingsId.value) ??
          displayAgents.value.find((item) => item.id === settingsId.value);
        if (agent) {
          const updated = await store.patchAgent(agent.id, {
            ...agent,
            display_name: editName.value,
            provider: editProvider.value,
            model: editModel.value,
          });
          if (updated) {
            displayAgents.value = displayAgents.value.map((item) =>
              item.id === updated.id ? { ...item, ...updated } : item,
            );
          }
        }
      } else if (settingsMode.value === 'team' && settingsId.value) {
        const team = displayTeams.value.find((item) => item.id === settingsId.value);
        if (team) {
          const updated = await updateTeam(team.id, {
            team_key: team.team_key,
            display_name: editName.value,
            status: team.status,
            definition_json: team.definition_json || '{}',
          });
          team.display_name = updated.display_name;
          team.definition_json = updated.definition_json;
        }
      }

      settingsOpen.value = false;
      $q.notify({ type: 'positive', message: t('chat.save') });
    } finally {
      settingsSaving.value = false;
    }
  }

  return {
    settingsOpen,
    settingsMode,
    settingsId,
    editName,
    editKey,
    editProvider,
    editModel,
    settingsSaving,
    settingsTitle,
    onSaveSettings,
  };
}
