// web/src/features/chat/composables/useGraphNodeTeam.ts
//
// GraphNode → TeamStage → TeamRun → MemberSession 解析链路。
// GraphTeamNode（渲染成员行）与 GraphStageBlock（heightOf 布局 + 成员弹框）共用，
// 保证卡片内容高度与 DAG 布局高度始终一致。
import { useActivityQueries } from './useActivityQueries';
import { useAppStore } from '../../../stores/app';
import type { GraphNode, MemberSession, PlanStep, TeamStage } from '../v2Types';

/** 团队节点成员行视图模型：运行时行（Planned=false，Session 有值）或编排期计划行（Planned=true）。 */
export interface TeamNodeMember {
  /** 运行时行 = MemberSession.ID；计划行 = `planned:<agentKey>` */
  ID: string;
  AgentKey: string;
  /** 展示名：运行时取 AgentName||AgentKey；计划行取 agents 目录 display_name||key */
  Name: string;
  Status: string;
  Planned: boolean;
  Session?: MemberSession;
}

export function useGraphNodeTeam() {
  const store = useActivityQueries();
  const appStore = useAppStore();

  /** GraphNode → TeamStage：TeamStageID 优先，DagNodeID 兜底（后端未回填时）。 */
  function teamStageOf(node: GraphNode): TeamStage | undefined {
    if (node.TeamStageID) {
      const ts = store.teamStages().get(node.TeamStageID);
      if (ts) return ts;
    }
    if (node.DagNodeID) {
      for (const ts of store.teamStages().values()) {
        if (ts.DagNodeID === node.DagNodeID) return ts;
      }
    }
    return undefined;
  }

  /** TeamStage → 全部 TeamRun 的 MemberSession（按时间/Seq 排序）。 */
  function membersOf(node: GraphNode): MemberSession[] {
    const ts = teamStageOf(node);
    if (!ts) return [];
    const out: MemberSession[] = [];
    for (const tr of store.getTeamStageTeamRuns(ts.ID)) {
      out.push(...store.getTeamRunMemberSessions(tr.ID));
    }
    return out;
  }

  function staffingOf(node: GraphNode): PlanStep | undefined {
    if (!node.DagNodeID) return undefined;
    return store.planSteps().get(node.DagNodeID);
  }

  /**
   * 节点展示成员：运行时 MemberSession 优先；尚未 dispatch（无 TeamStage）时
   * 回退到编排期已定员的 PlanStep.AgentKeys（名称经 agents 目录解析为中文名）。
   * 计划行状态恒 pending、不可点击、无耗时，运行时成员创建后无缝接管。
   */
  function displayMembersOf(node: GraphNode): TeamNodeMember[] {
    const runtime = membersOf(node);
    if (runtime.length > 0) {
      return runtime.map((ms) => ({
        ID: ms.ID,
        AgentKey: ms.AgentKey,
        Name: ms.AgentName || ms.AgentKey,
        Status: ms.Status,
        Planned: false,
        Session: ms,
      }));
    }
    const keys = staffingOf(node)?.AgentKeys ?? [];
    return keys.map((key) => {
      const ag = appStore.agents.find((a) => a.agent_key === key);
      return {
        ID: `planned:${key}`,
        AgentKey: key,
        Name: ag?.display_name || key,
        Status: 'pending',
        Planned: true,
      };
    });
  }

  return { teamStageOf, membersOf, staffingOf, displayMembersOf };
}
