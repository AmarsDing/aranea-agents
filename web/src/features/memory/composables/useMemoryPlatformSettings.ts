// FD4+FB3 fix: extract platform settings data fetching/saving + error handling
// from MemoryPlatformSettingsPanel.vue into composable so the .vue file only handles template.
import { onMounted, reactive, ref } from 'vue';
import { useMemoryApi } from './useMemoryApi';

export function useMemoryPlatformSettings() {
  const { getMemoryPlatformSettings, updateMemoryPlatformSettings } = useMemoryApi();

  const loading = ref(false);
  const saving = ref(false);
  const loaded = ref(false);
  const envPolicyStrict = ref(false);
  const envBackfillDisabled = ref(false);
  const message = ref('');
  const messageOk = ref(true);

  const form = reactive({
    policy_strict: false,
    episode_backfill_disabled: false,
  });

  async function load() {
    loading.value = true;
    message.value = '';
    try {
      const row = await getMemoryPlatformSettings();
      form.policy_strict = row.policy_strict;
      form.episode_backfill_disabled = row.episode_backfill_disabled;
      envPolicyStrict.value = row.env_policy_strict_override;
      envBackfillDisabled.value = row.env_episode_backfill_disabled_override;
      loaded.value = true;
    } catch (err) {
      loaded.value = false;
      message.value = err instanceof Error ? err.message : '加载失败';
      messageOk.value = false;
    } finally {
      loading.value = false;
    }
  }

  async function save() {
    saving.value = true;
    message.value = '';
    try {
      const row = await updateMemoryPlatformSettings({
        policy_strict: form.policy_strict,
        episode_backfill_disabled: form.episode_backfill_disabled,
      });
      form.policy_strict = row.policy_strict;
      form.episode_backfill_disabled = row.episode_backfill_disabled;
      envPolicyStrict.value = row.env_policy_strict_override;
      envBackfillDisabled.value = row.env_episode_backfill_disabled_override;
      message.value = '已保存';
      messageOk.value = true;
    } catch (err) {
      message.value = err instanceof Error ? err.message : '保存失败';
      messageOk.value = false;
    } finally {
      saving.value = false;
    }
  }

  onMounted(load);

  return { loading, saving, loaded, envPolicyStrict, envBackfillDisabled, message, messageOk, form, load, save };
}
