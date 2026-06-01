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
  status: string;
  mode: string;
  memberAvatars: string[];
  completedSteps: number;
  totalSteps: number;
  spiritSessionId: string;
  teamSessionId: string;
  members: SpiritMember[];
  sharedAgentIds: string[];
};

export type SpiritPanelMode = "spirit" | "team" | "member";
