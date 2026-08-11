import { ref } from 'vue';
import { useQuasar } from 'quasar';
import type { Tool } from './types';
import { useToolsStore } from '../../stores/tools';
import { parseKratosApiError } from '../../utils/kratosError';

export function useToolToggle(onChanged: () => void | Promise<void>) {
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const busyId = ref('');

  async function toggleEnabled(tool: Tool, value: boolean) {
    if (value && (tool.risk_level === 'high' || tool.risk_level === 'critical')) {
      $q.dialog({
        title: '高风险工具确认',
        message: `即将启用高风险工具「${tool.display_name}」（风险等级：${tool.risk_level}）。此操作可能带来安全风险，请确认您已了解相关风险。`,
        cancel: true,
        persistent: true,
      }).onOk(async () => {
        busyId.value = tool.id;
        try {
          await toolsStore.toggle(tool.id || tool.key, value, 'I_UNDERSTAND_RISK');
          await onChanged();
        } catch (err) {
          $q.notify({ type: 'negative', message: parseKratosApiError(err).message || '操作失败' });
        } finally {
          busyId.value = '';
        }
      });
      return;
    }
    busyId.value = tool.id;
    try {
      await toolsStore.toggle(tool.id || tool.key, value);
      await onChanged();
    } catch (err) {
      $q.notify({ type: 'negative', message: parseKratosApiError(err).message || '操作失败' });
    } finally {
      busyId.value = '';
    }
  }

  function removeTool(tool: Tool) {
    $q.dialog({
      title: '删除 Tool',
      message: `确认删除 ${tool.display_name}（${tool.key}）？`,
      cancel: true,
      persistent: true,
    }).onOk(async () => {
      busyId.value = tool.id;
      try {
        await toolsStore.remove(tool.id || tool.key);
        await onChanged();
      } catch (err) {
        $q.notify({ type: 'negative', message: parseKratosApiError(err).message || '删除失败' });
      } finally {
        busyId.value = '';
      }
    });
  }

  return { busyId, toggleEnabled, removeTool };
}
