import { createLearningLoopService } from '../../services';
import type { Observation, Pattern, KnowledgeProposal } from '../../services/kratos/learning_loop/v1/index';
export type { LearningObservation, LearningPattern, LearningProposal } from './learning.types';

function normalizeObservation(row: Observation): import('./learning.types').LearningObservation {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    session_id: row.sessionId ?? '',
    kind: row.kind ?? '',
    content: row.content ?? '',
    metadata: row.metadata ?? '',
    observed_at: row.observedAt ?? '',
  };
}

function normalizePattern(row: Pattern): import('./learning.types').LearningPattern {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    kind: row.kind ?? '',
    description: row.description ?? '',
    frequency: row.frequency ?? 0,
    confidence: row.confidence ?? 0,
    evidence: row.evidence ?? '',
    status: row.status ?? '',
    detected_at: row.detectedAt ?? '',
  };
}

function normalizeProposal(row: KnowledgeProposal): import('./learning.types').LearningProposal {
  return {
    id: row.id ?? '',
    agent_id: row.agentId ?? '',
    pattern_id: row.patternId ?? '',
    title: row.title ?? '',
    content: row.content ?? '',
    kind: row.kind ?? '',
    status: row.status ?? '',
    validated_at: row.validatedAt ?? '',
    approved_by: row.approvedBy ?? '',
    created_at: row.createdAt ?? '',
    updated_at: row.updatedAt ?? '',
  };
}

export async function listLearningObservations(
  agentId: string,
  since?: string,
): Promise<import('./learning.types').LearningObservation[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListObservations({ agentId, since });
  return (res.items ?? []).map(normalizeObservation);
}

export async function listLearningPatterns(
  agentId: string,
  status?: string,
): Promise<import('./learning.types').LearningPattern[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListPatterns({ agentId, status });
  return (res.items ?? []).map(normalizePattern);
}

export async function listLearningProposals(
  agentId: string,
  status?: string,
): Promise<import('./learning.types').LearningProposal[]> {
  const svc = createLearningLoopService();
  const res = await svc.ListProposals({ agentId, status });
  return (res.items ?? []).map(normalizeProposal);
}

export async function approveLearningProposal(
  agentId: string,
  proposalId: string,
): Promise<import('./learning.types').LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.ApproveProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function rejectLearningProposal(
  agentId: string,
  proposalId: string,
): Promise<import('./learning.types').LearningProposal> {
  const svc = createLearningLoopService();
  const res = await svc.RejectProposal({ agentId, id: proposalId });
  return normalizeProposal(res);
}

export async function runLearningLoop(agentId: string): Promise<void> {
  const svc = createLearningLoopService();
  await svc.RunLoop({ agentId });
}
