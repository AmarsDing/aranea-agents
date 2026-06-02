import { createLearningLoopService } from "../../services";
import type { Observation, Pattern, KnowledgeProposal } from "../../services/kratos/learning_loop/v1/index";

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

function normalizeObservation(row: Observation): LearningObservation {
  return {
    id: row.id ?? "",
    agent_id: row.agentId ?? "",
    session_id: row.sessionId ?? "",
    kind: row.kind ?? "",
    content: row.content ?? "",
    metadata: row.metadata ?? "",
    observed_at: row.observedAt ?? ""
  };
}

function normalizePattern(row: Pattern): LearningPattern {
  return {
    id: row.id ?? "",
    agent_id: row.agentId ?? "",
    kind: row.kind ?? "",
    description: row.description ?? "",
    frequency: row.frequency ?? 0,
    confidence: row.confidence ?? 0,
    evidence: row.evidence ?? "",
    status: row.status ?? "",
    detected_at: row.detectedAt ?? ""
  };
}

function normalizeProposal(row: KnowledgeProposal): LearningProposal {
  return {
    id: row.id ?? "",
    agent_id: row.agentId ?? "",
    pattern_id: row.patternId ?? "",
    title: row.title ?? "",
    content: row.content ?? "",
    kind: row.kind ?? "",
    status: row.status ?? "",
    validated_at: row.validatedAt ?? "",
    approved_by: row.approvedBy ?? "",
    created_at: row.createdAt ?? "",
    updated_at: row.updatedAt ?? ""
  };
}

export async function listLearningObservations(
  agentId: string,
  since?: string
): Promise<LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map(normalizeObservation);
}

export async function listLearningPatterns(
  agentId: string,
  status?: string
): Promise<LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map(normalizePattern);
}

export async function listLearningProposals(
  agentId: string,
  status?: string
): Promise<LearningProposal[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListProposals({ agentId, status });
  return (res.items ?? []).map(normalizeProposal);
}

export async function approveLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.ApproveProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function rejectLearningProposal(
  agentId: string,
  proposalId: string
): Promise<LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.RejectProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function runLearningLoop(agentId: string): Promise<void> {
  const svc = createLearningLoopService();
  await svc.RunLoop({ agentId });
}
