/**
 * 点击 GraphNode 定位到对应 TeamStagePanel 的 composable。
 *
 * 使用模块级 ref（单例模式），让 GraphStageBlock / GraphNode 等深层组件
 * 能够触发 ChatMessageList 中的滚动+高亮，避免 props/emit 跨层穿透。
 *
 * 流程：GraphStageBlock.locate(teamStageId) →
 *       ChatMessageList watch locateTeamStageCommand →
 *       querySelector('[data-team-stage-id="X"]') → scrollIntoView + 高亮。
 *
 * 设计参照 useScrollToActivity.ts（agent 定位机制）。
 */
import { ref, type Ref } from 'vue';

export interface LocateTeamStageCommand {
  teamStageId: string;
}

const locateTeamStageCommand = ref<LocateTeamStageCommand | null>(null);

export function useLocateTeamStage(): {
  locateTeamStageCommand: Ref<LocateTeamStageCommand | null>;
  locate: (teamStageId: string) => void;
} {
  function locate(teamStageId: string) {
    locateTeamStageCommand.value = { teamStageId };
  }

  return { locateTeamStageCommand, locate };
}
