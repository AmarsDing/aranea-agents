import { createSkillEvolutionService } from '../../services';
import type { SkillProposal } from '../../services/kratos/skill_evolution/v1/index';

export type { SkillProposal };

function mapProposal(raw: SkillProposal) {
  return {
    id: String(raw.id ?? ''),
    agent_id: String(raw.agentId ?? ''),
    pattern_hash: String(raw.patternHash ?? ''),
    pattern_desc: String(raw.patternDesc ?? ''),
    skill_name: String(raw.skillName ?? ''),
    skill_md: String(raw.skillMd ?? ''),
    status: String(raw.status ?? ''),
    approved_by: String(raw.approvedBy ?? ''),
    rejected_by: String(raw.rejectedBy ?? ''),
    created_at: String(raw.createdAt ?? ''),
    approved_at: String(raw.approvedAt ?? ''),
  };
}

export type SkillProposalRow = ReturnType<typeof mapProposal>;

export async function listSkillProposals(input: {
  agentId: string;
  status?: string;
  page?: number;
  pageSize?: number;
}): Promise<{ items: SkillProposalRow[]; total: number; page: number; pageSize: number }> {
  const svc = createSkillEvolutionService();
  const res = await svc.ListSkillProposals({
    agentId: input.agentId,
    status: input.status,
    page: input.page,
    pageSize: input.pageSize,
  });
  return {
    items: (res.items ?? []).map(mapProposal),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? input.page ?? 1),
    pageSize: Number(res.pageSize ?? input.pageSize ?? 20),
  };
}

export async function getSkillProposal(id: string): Promise<SkillProposalRow> {
  const svc = createSkillEvolutionService();
  const res = await svc.GetSkillProposal({ id });
  return mapProposal(res);
}

export async function approveSkillProposal(id: string, approvedBy: string): Promise<SkillProposalRow> {
  const svc = createSkillEvolutionService();
  const res = await svc.ApproveSkillProposal({ id, approvedBy });
  return mapProposal(res);
}

export async function rejectSkillProposal(id: string, rejectedBy: string): Promise<SkillProposalRow> {
  const svc = createSkillEvolutionService();
  const res = await svc.RejectSkillProposal({ id, rejectedBy });
  return mapProposal(res);
}

export async function registerSkillProposal(id: string): Promise<SkillProposalRow> {
  const svc = createSkillEvolutionService();
  const res = await svc.RegisterSkillProposal({ id });
  return mapProposal(res);
}

export async function triggerSkillDetection(agentId: string): Promise<SkillProposalRow[]> {
  const svc = createSkillEvolutionService();
  const res = await svc.TriggerSkillDetection({ agentId });
  return (res.proposals ?? []).map(mapProposal);
}
