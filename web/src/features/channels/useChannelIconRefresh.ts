// FB4 fix: extract avatar icon refresh + $q.notify from ChannelEditorDialog.vue
// into composable so the .vue file does not call Store or $q.notify directly.
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useAvatarCatalogStore } from '../../stores/avatar';

export function useChannelIconRefresh() {
  const { t } = useI18n();
  const $q = useQuasar();
  const avatarStore = useAvatarCatalogStore();
  const refreshingIcons = ref(false);

  async function refreshPlatformIcons() {
    refreshingIcons.value = true;
    try {
      const result = await avatarStore.refreshChannelIcons();
      $q.notify({
        type: result.failed > 0 ? 'warning' : 'positive',
        message: t('channelEditor.refreshIconsResult', { updated: result.updated, failed: result.failed }),
        position: 'top',
      });
    } catch {
      $q.notify({ type: 'negative', message: t('channelEditor.refreshIconsFailed'), position: 'top' });
    } finally {
      refreshingIcons.value = false;
    }
  }

  return { refreshingIcons, refreshPlatformIcons };
}
