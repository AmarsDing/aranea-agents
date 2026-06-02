export type LearningObservation = {
  id: string;
  agent_id: string;
  session_id: string;
  kind: string;
  content: string;
  metadata: string;
  observed_at: string;
};

export type LearningPattern = {
  id: string;
  agent_id: string;
  kind: string;
  description: string;
  frequency: number;
  confidence: number;
  evidence: string;
  status: string;
  detected_at: string;
};

export type LearningProposal = {
  id: string;
  agent_id: string;
  pattern_id: string;
  title: string;
  content: string;
  kind: string;
  status: string;
  validated_at: string;
  approved_by: string;
  created_at: string;
  updated_at: string;
};
