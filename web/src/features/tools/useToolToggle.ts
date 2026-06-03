import { ref } from 'vue';
import { useQuasar } from 'quasar';
import type { Tool } from './types';
import { useToolsStore } from '../../stores/tools';

export function useToolToggle(onChanged: () => void | Promise<void>) {
  const $q = useQuasar();
  const toolsStore = useToolsStore();
  const busyId = ref('');

  async function toggleEnabled(tool: Tool, value: boolean) {
    if (value && (tool.risk_level === 'high' || tool.risk_level === 'critical')) {
      $q.dialog({
        title: '高风险工具确认',
        message: `即将启用高风险工具「${tool.display_name}」（风险等级：${tool.risk_level}）。请输入工具 Key 以确认：${tool.key}`,
        prompt: { model: '', type: 'text', label: '请输入 Tool Key' },
        cancel: true,
        persistent: true,
      }).onOk(async (inputKey: string) => {
        if (inputKey !== tool.key) {
          $q.notify({ type: 'negative', message: '输入的 Key 不匹配，操作已取消' });
          return;
        }
        busyId.value = tool.id;
        try {
          await toolsStore.toggle(tool.id || tool.key, value, tool.key);
          await onChanged();
        } catch (err) {
          $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '操作失败' });
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
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '操作失败' });
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
        $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
      } finally {
        busyId.value = '';
      }
    });
  }

  return { busyId, toggleEnabled, removeTool };
}
