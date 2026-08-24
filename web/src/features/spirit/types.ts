/** 精灵助手（系统内置总管家）的 agent_key，与后端 biz.SpiritAgentKey 对齐。全站唯一出处，禁止再写字面量。 */
export const SPIRIT_AGENT_KEY = '__spirit__';

export type SpiritTeamStatus =
  | 'pending'
  | 'running'
  | 'paused'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
  | 'archived';

/** Runtime type guard for SpiritTeamStatus — validates WS/API pushed values. */
const VALID_TEAM_STATUSES: ReadonlySet<string> = new Set<string>([
  'pending',
  'running',
  'paused',
  'completed',
  'failed',
  'cancelled',
  'interrupted',
  'archived',
]);
export function isValidTeamStatus(s: string): s is SpiritTeamStatus {
  return VALID_TEAM_STATUSES.has(s);
}

export type SpiritTeamMode = 'coordinator' | 'sequential' | 'parallel' | 'critic_loop' | 'swarm' | 'adaptive';

/** Runtime type guard for SpiritTeamMode — validates WS/API pushed values. */
const VALID_TEAM_MODES: ReadonlySet<string> = new Set<string>([
  'coordinator',
  'sequential',
  'parallel',
  'critic_loop',
  'swarm',
  'adaptive',
]);
export function isValidTeamMode(s: string): s is SpiritTeamMode {
  return VALID_TEAM_MODES.has(s);
}

/** Spirit status bar data — shared between ChatMessagePanel and SpiritStatusBar. */
export type SpiritStatusBarData = {
  runningTeamCount: number;
  interruptedTeamCount: number;
  /** Number of teams that have reached a terminal "completed" state. */
  completedTeamCount: number;
  /** Total number of teams in the current orchestration. */
  totalTeamCount: number;
  tokenUsage?: { in: number; out: number } | null;
  /** Context usage ratio (0-1) for the data_usage ring-style display. */
  contextRatio?: number | null;
  /** Current context tokens used. */
  contextUsedTokens?: number | null;
  /** Model context window size in tokens. */
  contextWindow?: number | null;
  /** Latest turn's prompt-assembly breakdown (WS context_usage push only). */
  contextBudget?: import('../session/types').ContextBudgetSnapshot | null;
  complexityLevel?: string | null;
  complexityReason?: string | null;
  checkpointStep?: string | null;
  dqScore?: number | null;
};

export type SpiritMember = {
  /** Catalog / entity agent ID (not the chat session ID). */
  agentId: string;
  /**
   * Member chat session ID used by Pause/Resume/Cancel RPCs.
   * Populated from v2 TeamStage.ChildSessionID or MemberSession.SessionID.
   */
  chatSessionId?: string;
  agentKey: string;
  displayName: string;
  role: string;
  // status is kept as string because the backend sends member-specific lifecycle
  // states (e.g. "idle", "running", "error") that differ from SpiritTeamStatus.
  status: string;
  avatarUrl: string;
};

export type SpiritTeam = {
  id: string;
  teamName: string;
  taskSummary: string;
  status: SpiritTeamStatus;
  mode: SpiritTeamMode;
  memberAvatars: string[];
  completedSteps: number;
  totalSteps: number;
  /** Progress percentage from backend (0-100). Used directly for progress bar
   *  rendering instead of reverse-computing from completedSteps/totalSteps. */
  progressPct: number;
  durationMs: number;
  spiritSessionId: string;
  teamSessionId: string;
  members: SpiritMember[];
  sharedAgentIds: string[];
  /** Creation timestamp (ms) used for sidebar ordering (B.9.1). */
  createdAt: number;
  dagNodeId?: string;
  graphExecutionId?: string;
  dependsOn?: string[];
  topologyReason?: string;
  interruptReason?: string;
  tokenIn?: number;
  tokenOut?: number;
  dqScore?: DQScoreBreakdown;
  evolutionSuggestion?: EvolutionSuggestion;
};

export type SpiritPanelMode = 'spirit' | 'team' | 'member';

export type TeamProgressView = {
  teamId: string;
  teamName: string;
  status: SpiritTeamStatus;
  progressPct: number;
  currentStep: string;
  durationMs: number;
};

export type ParallelConfig = {
  maxConcurrentTeams: number;
  maxTeamConcurrency: number;
  teamTimeoutSeconds: number;
  autoArchiveSeconds: number;
  maxSessionDepth: number;
};

export type TaskNodeID = string;

export type TaskNode = {
  id: TaskNodeID;
  taskName: string;
  description: string;
  dependsOn: TaskNodeID[];
  mode: SpiritTeamMode;
  agentKeys: string[];
};

export type TopologyType = 'parallel' | 'sequential' | 'hybrid' | 'coordinator';

/** Verification gate type injected into the orchestration graph. */
export type VerificationNodeType = 'output_format' | 'task_completion' | 'human_approval';

/** A verification gate node in the DAG. */
export type VerificationNode = {
  nodeId: string;
  type: VerificationNodeType;
  afterNode: string;
  failureAction: 'skip' | 'retry_then_block' | 'interrupt_before';
  status?: 'pending' | 'passed' | 'failed' | 'skipped';
  issues?: string[];
  label?: string;
  failureReason?: string;
  retryCount?: number;
  maxRetries?: number;
};

/** DQ (Deployment Quality) score breakdown. */
export interface DQScoreBreakdown {
  validity: number;
  specificity: number;
  correctness: number;
  overall: number;
}

/** Evolution suggestion from DQ analysis. */
export interface EvolutionSuggestion {
  currentTopology: string;
  suggestedTopology: string;
  reason: string;
  dqScore: number;
}

/** Team completion breakdown from spirit_teams_all_completed event. */
export interface CompletionStats {
  totalTeams: number;
  completedTeams: number;
  failedTeams: number;
}

/** Task row data for UnifiedExecutionPanel task breakdown section. */
export type TaskRow = {
  id: string;
  taskName: string;
  teamLabel: string | null;
  isRunning: boolean;
  statusText: string;
};

/** DAG flow node data for UnifiedExecutionPanel dependencies section. */
export type DagFlowNode = {
  id: string;
  name: string;
  state: 'done' | 'running' | 'waiting' | 'failed' | 'interrupted';
  depLabels: string[];
};
