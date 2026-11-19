import { onMounted, ref } from 'vue';
import { getUsage, type AgentUsage } from '../fleet/api';

/**
 * 各编码终端的用量（运行台模块的 /v1/fleet/usage）。
 * undefined = 还没取回；null = 取不到（模块没部署或没有权限），界面如实写明，不显示成 0。
 */
export function useTerminalUsage(allowed: () => boolean) {
  const terminals = ref<AgentUsage[] | null>();

  async function refresh() {
    if (!allowed()) {
      terminals.value = null;
      return;
    }
    try {
      terminals.value = await getUsage();
    } catch {
      terminals.value = null;
    }
  }

  onMounted(refresh);
  return { terminals, refresh };
}
