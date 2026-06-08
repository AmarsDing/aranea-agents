export type SpiritTeamStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
  | 'archived';

export type SpiritTeamMode = 'coordinator' | 'sequential' | 'parallel' | 'critic_loop' | 'swarm' | 'adaptive';

export type SpiritMember = {
  agentId: string;
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
  durationMs: number;
  spiritSessionId: string;
  teamSessionId: string;
  members: SpiritMember[];
  sharedAgentIds: string[];
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

export type SynthesisStrategy = 'template' | 'prompt' | 'hybrid';

export type TeamSynthesisResult = {
  teamId: string;
  teamName: string;
  taskName: string;
  status: SpiritTeamStatus;
  summary: string;
  keyFindings?: string;
};

export type SynthesisOutput = {
  content: string;
  strategy: SynthesisStrategy;
  teamResults: TeamSynthesisResult[];
  synthesizedAt: string;
};

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
