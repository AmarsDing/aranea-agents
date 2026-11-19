import { ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { Tool } from './types';
import { useToolsStore } from '../../stores/tools';
import { parseKratosApiError } from '../../utils/kratosError';
import { confirm } from '../../composables/useAppConfirm';

export function useToolToggle(onChanged: () => void | Promise<void>) {
  const $q = useQuasar();
  const { t } = useI18n();
  const toolsStore = useToolsStore();
  const busyId = ref('');

  async function toggleEnabled(tool: Tool, value: boolean) {
    if (value && (tool.risk_level === 'high' || tool.risk_level === 'critical')) {
      const ok = await confirm({
        title: t('toolsPage.toggle.highRiskTitle'),
        message: t('toolsPage.toggle.highRiskMessage', { name: tool.display_name, level: tool.risk_level }),
        tone: 'danger',
      });
      if (!ok) return;
      busyId.value = tool.id;
      try {
        await toolsStore.toggle(tool.id || tool.key, value, 'I_UNDERSTAND_RISK');
        await onChanged();
      } catch (err) {
        $q.notify({ type: 'negative', message: parseKratosApiError(err).message || t('toolsPage.toggle.actionFailed') });
      } finally {
        busyId.value = '';
      }
      return;
    }
    busyId.value = tool.id;
    try {
      await toolsStore.toggle(tool.id || tool.key, value);
      await onChanged();
    } catch (err) {
      $q.notify({ type: 'negative', message: parseKratosApiError(err).message || t('toolsPage.toggle.actionFailed') });
    } finally {
      busyId.value = '';
    }
  }

  async function removeTool(tool: Tool) {
    if (tool.readonly) {
      $q.notify({ type: 'warning', message: t('toolsPage.toggle.readonlyNoRemove', { name: tool.display_name }) });
      return;
    }
    const ok = await confirm({
      title: t('toolsPage.toggle.removeTitle'),
      message: t('toolsPage.toggle.removeMessage', { name: tool.display_name, key: tool.key }),
      tone: 'danger',
    });
    if (!ok) return;
    busyId.value = tool.id;
    try {
      await toolsStore.remove(tool.id || tool.key);
      await onChanged();
    } catch (err) {
      $q.notify({ type: 'negative', message: parseKratosApiError(err).message || t('toolsPage.toggle.removeFailed') });
    } finally {
      busyId.value = '';
    }
  }

  return { busyId, toggleEnabled, removeTool };
}
