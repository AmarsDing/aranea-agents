import type { TeamDefinition, TeamDefinitionGraphNode } from './types';

export function buildGraphFromDefinition(def: TeamDefinition): NonNullable<TeamDefinition['graph']> {
  const members = [...(def.members || [])].sort((a, b) => a.sort_order - b.sort_order);
  const layout = graphLayoutForMode(def.mode || 'sequential');
  const mode = def.mode || 'sequential';
  const memberNodes: TeamDefinitionGraphNode[] = members.map((member, index) => ({
    id: graphMemberID(member, index),
    type: 'agent',
    label: member.name || member.role || `Agent ${index + 1}`,
    agent_id: member.agent_id,
    role: member.role,
    x: graphX(mode, index, members.length),
    y: graphY(mode, index, members.length),
  }));
  const nodes: TeamDefinitionGraphNode[] = [{ id: 'start', type: 'start', label: '开始', x: 0, y: 80 }, ...memberNodes];
  if (mode === 'parallel' && memberNodes.length > 1) {
    nodes.push({ id: 'join', type: 'join', label: '并行汇合', x: graphEndX(mode, members.length) - 90, y: 80 });
  }
  nodes.push({ id: 'end', type: 'end', label: '结束', x: graphEndX(mode, members.length), y: 80 });
  return { version: 1, layout, nodes, edges: buildGraphEdges(mode, members, def.synthesizer_agent_id) };
}

function buildGraphEdges(mode: string, members: TeamDefinition['members'], synthesizerAgentId?: string) {
  const ids = members.map(graphMemberID);
  if (ids.length === 0) return [{ id: 'start-end', source: 'start', target: 'end', label: 'no members' }];
  if (mode === 'adaptive') {
    return [
      { id: 'start-adaptive', source: 'start', target: ids[0], label: 'select topology' },
      ...ids.slice(0, -1).map((id, index) => ({
        id: `${id}-${ids[index + 1]}`,
        source: id,
        target: ids[index + 1],
        label: 'candidate',
      })),
      { id: `${ids[ids.length - 1]}-end`, source: ids[ids.length - 1], target: 'end', label: 'final' },
    ];
  }
  if (mode === 'parallel') {
    const synthId = resolveSynthesizerMemberId(members, synthesizerAgentId);
    const workerIds = synthId ? ids.filter((id) => id !== synthId) : ids;
    const downstream = synthId || 'end';
    const edges = workerIds.map((id) => ({ id: `start-${id}`, source: 'start', target: id, label: 'fan out' }));
    edges.push(...workerIds.map((id) => ({ id: `${id}-join`, source: id, target: 'join', label: 'join' })));
    edges.push({ id: 'join-downstream', source: 'join', target: downstream, label: synthId ? 'synthesize' : 'finish' });
    if (synthId) edges.push({ id: `${synthId}-end`, source: synthId, target: 'end', label: 'final' });
    return edges;
  }
  if (mode === 'critic_loop') {
    const edges = [{ id: `start-${ids[0]}`, source: 'start', target: ids[0], label: 'draft' }];
    for (let i = 0; i < ids.length - 1; i++)
      edges.push({
        id: `${ids[i]}-${ids[i + 1]}`,
        source: ids[i],
        target: ids[i + 1],
        label: i === 0 ? 'review' : 'revise',
      });
    if (ids.length > 1)
      edges.push({
        id: `${ids[ids.length - 1]}-${ids[0]}-loop`,
        source: ids[ids.length - 1],
        target: ids[0],
        label: 'optional loop',
      });
    edges.push({ id: `${ids[ids.length - 1]}-end`, source: ids[ids.length - 1], target: 'end', label: 'approved' });
    return edges;
  }
  return ['start', ...ids, 'end'].slice(0, -1).map((source, index, chain) => {
    const target = index === chain.length - 1 ? 'end' : chain[index + 1];
    return {
      id: `${source}-${target}`,
      source,
      target,
      label: mode === 'coordinator' && index === 0 ? 'plan' : 'next',
    };
  });
}

function graphMemberID(member: TeamDefinition['members'][number], index: number) {
  return `member-${member.sort_order || index + 1}`;
}

function resolveSynthesizerMemberId(members: TeamDefinition['members'], synthesizerAgentId?: string) {
  const synthAgent = String(synthesizerAgentId || '').trim();
  if (synthAgent) {
    const idx = members.findIndex((m) => String(m.agent_id || '').trim() === synthAgent);
    if (idx >= 0) return graphMemberID(members[idx], idx);
  }
  const synth = members.find((m) => String(m.role || '').toLowerCase() === 'synthesizer');
  if (synth) return graphMemberID(synth, members.indexOf(synth));
  return '';
}

function graphLayoutForMode(mode: string) {
  if (mode === 'adaptive') return 'adaptive';
  if (mode === 'parallel') return 'parallel';
  if (mode === 'critic_loop') return 'loop';
  if (mode === 'coordinator') return 'coordinator';
  return 'linear';
}

function graphX(mode: string, index: number, _total: number) {
  if (mode === 'parallel') return 160;
  return 160 + index * 150;
}

function graphY(mode: string, index: number, _total: number) {
  if (mode !== 'parallel') return 80;
  const offset = (index - (_total - 1) / 2) * 74;
  return 80 + offset;
}

function graphEndX(mode: string, total: number) {
  if (mode === 'parallel') return 360;
  return 160 + Math.max(total, 1) * 150;
}
