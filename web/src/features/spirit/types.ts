export type SpiritTeamStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted'
  | 'archived';

export type SpiritTeamMode = 'coordinator' | 'sequential' | 'parallel' | 'critic_loop' | 'swarm' | 'adaptive' | 'direct';

export type SpiritMember = {
  agentId: string;
  agentKey: string;
  displayName: string;
  role: string;
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
  dependsOn?: string[];
  topologyReason?: string;
};

export type SpiritPanelMode = 'spirit' | 'team' | 'member';

export type TeamProgressView = {
  teamId: string;
  teamName: string;
  status: string;
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
  status: string;
  summary: string;
  keyFindings?: string;
};

export type SynthesisOutput = {
  content: string;
  strategy: SynthesisStrategy;
  teamResults: TeamSynthesisResult[];
  synthesizedAt: string;
};
