/**
 * 点击 Agent 卡片定位到中间面板会话的 composable。
 *
 * 使用模块级 ref（单例模式），让 ChatEntitySidebar 和 ActivityStream 共享定位命令。
 * ChatEntitySidebar → locate() → ActivityStream watch locateCommand → scrollIntoView + 高亮。
 */
import { ref, type Ref } from 'vue';

export interface LocateCommand {
  agentKey: string;
  teamSessionId: string;
  teamId: string;
}

const locateCommand = ref<LocateCommand | null>(null);

export function useScrollToActivity(): {
  locateCommand: Ref<LocateCommand | null>;
  locate: (agentKey: string, teamSessionId: string, teamId: string) => void;
} {
  function locate(agentKey: string, teamSessionId: string, teamId: string) {
    locateCommand.value = { agentKey, teamSessionId, teamId };
  }

  return { locateCommand, locate };
}
