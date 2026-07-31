import { describe, expect, it } from 'vitest';
import {
  buildGraphFromDefinition,
  definitionGraphFromCompileJSON,
  definitionGraphSource,
  definitionToJSON,
  definitionTopologyKey,
  definitionTopologyOverwriteKey,
  deriveMemberRolesForMode,
  groupTeamsByIndustry,
  inferTeamIndustryId,
  linkableGraphOptions,
  parseDefinition,
  rebuildDefinitionGraph,
  resetDefinition,
} from '../teamUtils';
import type { Team, TeamDefinition } from '../../../features/teams/types';
import type { Agent } from '../../../features/agents/types';
import type { PlatformResourceTreeNode } from '../../../features/platform/types';

describe('teamUtils.parseDefinition', () => {
  it('preserves runtime_engine, failure_policy, and linked_graph_id', () => {
    const team: Team = {
      id: 't1',
      team_key: 'demo',
      display_name: 'Demo',
      status: 'active',
      is_default: false,
      taxonomy_industry_id: '',
      definition_json: JSON.stringify({
        version: 1,
        mode: 'parallel',
        runtime_engine: 'graph',
        team_graph_runtime: true,
        linked_graph_id: 'g-1',
        failure_policy: { default: 'retry_then_block', parallel_fail: 'continue' },
        members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
      }),
      app_name: 'demo',
      linked_graph_id: 'g-team-col',
      has_active_run: false,
      created_at: '',
      updated_at: '',
      deleted_at: '',
    };

    const def = parseDefinition(team);
    expect(def.runtime_engine).toBe('graph');
    expect(def.team_graph_runtime).toBe(true);
    expect(def.linked_graph_id).toBe('g-1');
    expect(def.failure_policy?.parallel_fail).toBe('continue');

    const json = JSON.parse(definitionToJSON(def)) as Record<string, unknown>;
    expect(json.runtime_engine).toBe('graph');
    expect(json.team_graph_runtime).toBe(true);
    expect(json.failure_policy).toEqual({ default: 'retry_then_block', parallel_fail: 'continue' });
  });

  it('normalizes legacy native definition to graph on parse and serialize', () => {
    const team: Team = {
      id: 't2',
      team_key: 'legacy',
      display_name: 'Legacy',
      status: 'active',
      is_default: false,
      taxonomy_industry_id: '',
      definition_json: JSON.stringify({
        version: 1,
        mode: 'sequential',
        runtime_engine: 'native',
        team_graph_runtime: false,
        members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
      }),
      app_name: 'legacy',
      linked_graph_id: '',
      has_active_run: false,
      created_at: '',
      updated_at: '',
      deleted_at: '',
    };

    const def = parseDefinition(team);
    expect(def.runtime_engine).toBe('graph');
    expect(def.team_graph_runtime).toBe(true);

    const json = JSON.parse(definitionToJSON(def)) as Record<string, unknown>;
    expect(json.runtime_engine).toBe('graph');
    expect(json.team_graph_runtime).toBe(true);
  });
});

describe('teamUtils.definition source (M53 Phase 11)', () => {
  const mkTeam = (definitionJson: string): Team => ({
    id: 't-src',
    team_key: 'src',
    display_name: 'Source',
    status: 'active',
    is_default: false,
    taxonomy_industry_id: '',
    definition_json: definitionJson,
    app_name: 'src',
    linked_graph_id: '',
    has_active_run: false,
    created_at: '',
    updated_at: '',
    deleted_at: '',
  });

  it('round-trips source through parseDefinition and definitionToJSON', () => {
    const team = mkTeam(
      JSON.stringify({
        version: 1,
        source: 'custom',
        mode: 'sequential',
        members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
      }),
    );

    const def = parseDefinition(team);
    expect(def.source).toBe('custom');

    const json = JSON.parse(definitionToJSON(def)) as Record<string, unknown>;
    expect(json.source).toBe('custom');
  });

  it('definitionGraphSource mirrors backend GraphSource: empty/unknown falls back to preset', () => {
    const base = mkTeam(JSON.stringify({ version: 1, mode: 'sequential', members: [] }));
    expect(definitionGraphSource(parseDefinition(base))).toBe('preset');
    expect(definitionGraphSource({ source: 'custom' } as TeamDefinition)).toBe('custom');
    expect(definitionGraphSource({ source: 'linked_external' } as TeamDefinition)).toBe('linked_external');
    expect(definitionGraphSource({ source: 'weird' } as TeamDefinition)).toBe('preset');
    expect(definitionGraphSource({} as TeamDefinition)).toBe('preset');
  });

  it('resetDefinition clears stale source back to derived default', () => {
    const def = parseDefinition(
      mkTeam(
        JSON.stringify({
          version: 1,
          source: 'custom',
          mode: 'parallel',
          members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
        }),
      ),
    );
    resetDefinition(def);
    expect(def.source).toBeUndefined();
    expect(definitionGraphSource(def)).toBe('preset');
  });
});

describe('teamUtils F2 helpers (M53 Phase 11)', () => {
  const mkDef = (overrides: Partial<TeamDefinition> = {}): TeamDefinition => ({
    version: 1,
    description: '',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
    ...overrides,
  });

  it('definitionTopologyOverwriteKey changes with topology and enable_checkpoint', () => {
    const base = definitionTopologyOverwriteKey(mkDef());
    expect(definitionTopologyOverwriteKey(mkDef())).toBe(base);
    // checkpoint 开关参与指纹（缺省镜像后端默认 true）
    expect(definitionTopologyOverwriteKey(mkDef({ enable_checkpoint: false }))).not.toBe(base);
    expect(definitionTopologyOverwriteKey(mkDef({ enable_checkpoint: true }))).toBe(base);
    // 拓扑字段变化
    expect(definitionTopologyOverwriteKey(mkDef({ mode: 'parallel' }))).not.toBe(base);
    // 非拓扑字段不变化
    expect(definitionTopologyOverwriteKey(mkDef({ description: 'changed' }))).toBe(base);
  });

  it('linkableGraphOptions excludes team-owned graphs and keeps independent assets', () => {
    const graphs = [
      { id: 'g1', name: '独立图', metadata: {} },
      { id: 'g2', name: 'Team 自有图', metadata: { team_owned: true } },
      { id: 'g3', name: '', metadata: { team_owned: false } },
      { id: '', name: '空 id', metadata: {} },
    ];
    const options = linkableGraphOptions(graphs);
    expect(options.map((o) => o.value)).toEqual(['g1', 'g3']);
    // name 缺省回退 id
    expect(options[1]).toEqual({ label: 'g3', value: 'g3' });
  });
});

describe('teamUtils.definitionTopologyKey', () => {
  const mkMembers = () => [
    { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
    { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
  ];
  const mkBase = (): TeamDefinition => ({
    version: 1,
    description: 'd',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: mkMembers(),
  });

  it('ignores non-topology fields (description/timeout/failure_policy/intent_anchor)', () => {
    const next: TeamDefinition = {
      ...mkBase(),
      description: 'changed',
      timeout_seconds: 120,
      failure_policy: { default: 'fail_fast' },
      intent_anchor_agent_id: 'a1',
    };
    expect(definitionTopologyKey(next)).toBe(definitionTopologyKey(mkBase()));
  });

  it('changes when mode changes', () => {
    expect(definitionTopologyKey({ ...mkBase(), mode: 'parallel' })).not.toBe(definitionTopologyKey(mkBase()));
  });

  it('changes when synthesizer_agent_id changes', () => {
    expect(definitionTopologyKey({ ...mkBase(), synthesizer_agent_id: 'a2' })).not.toBe(
      definitionTopologyKey(mkBase()),
    );
  });

  it('changes when member agent_id / role / name / enabled / sort_order change', () => {
    const baseKey = definitionTopologyKey(mkBase());
    const variants = [
      [{ ...mkMembers()[0]!, agent_id: 'a9' }, mkMembers()[1]!],
      [{ ...mkMembers()[0]!, role: 'coordinator' }, mkMembers()[1]!],
      [{ ...mkMembers()[0]!, name: '改名' }, mkMembers()[1]!],
      [{ ...mkMembers()[0]!, enabled: false }, mkMembers()[1]!],
      [{ ...mkMembers()[0]!, sort_order: 99 }, mkMembers()[1]!],
    ];
    for (const members of variants) {
      expect(definitionTopologyKey({ ...mkBase(), members })).not.toBe(baseKey);
    }
  });

  it('changes when a member is added or removed', () => {
    const baseKey = definitionTopologyKey(mkBase());
    expect(definitionTopologyKey({ ...mkBase(), members: [mkMembers()[0]!] })).not.toBe(baseKey);
    const added = [...mkMembers(), { agent_id: 'a3', role: 'worker', name: 'W3', enabled: true, sort_order: 30 }];
    expect(definitionTopologyKey({ ...mkBase(), members: added })).not.toBe(baseKey);
  });

  it('is stable regardless of member array order when sort_order matches', () => {
    const reversed = [...mkMembers()].reverse();
    expect(definitionTopologyKey({ ...mkBase(), members: reversed })).toBe(definitionTopologyKey(mkBase()));
  });
});

describe('teamUtils.rebuildDefinitionGraph', () => {
  const mkMembers = () => [
    { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
    { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
  ];
  const mkBase = (): TeamDefinition => ({
    version: 1,
    description: '',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: mkMembers(),
  });

  it('rebuilds a stale graph to reflect the current mode', () => {
    // 陈旧 graph（linear layout）与当前 mode=parallel 不一致（ADR-08 割裂点 1 场景）
    const staleGraph = buildGraphFromDefinition(mkBase());
    const def: TeamDefinition = { ...mkBase(), mode: 'parallel', synthesizer_agent_id: 'a2', graph: staleGraph };
    const graph = rebuildDefinitionGraph(def);
    expect(graph.layout).toBe('parallel');
    expect(graph.nodes.some((node) => node.type === 'join')).toBe(true);
    expect(graph.nodes.some((node) => node.agent_id === 'a2')).toBe(true);
  });

  it('keeps positions of surviving nodes when layout is unchanged', () => {
    // 模拟画布手工拖动 / 后端模板坐标：member-10 被移动到 (999, 888)
    const previous = buildGraphFromDefinition(mkBase());
    previous.nodes = previous.nodes.map((node) => (node.id === 'member-10' ? { ...node, x: 999, y: 888 } : node));
    const def: TeamDefinition = {
      ...mkBase(),
      graph: previous,
      members: [...mkMembers(), { agent_id: 'a3', role: 'worker', name: 'W3', enabled: true, sort_order: 30 }],
    };
    const graph = rebuildDefinitionGraph(def);
    const kept = graph.nodes.find((node) => node.id === 'member-10');
    expect(kept?.x).toBe(999);
    expect(kept?.y).toBe(888);
    expect(graph.nodes.some((node) => node.id === 'member-30')).toBe(true);
  });

  it('drops stale positions when layout changes', () => {
    const previous = buildGraphFromDefinition(mkBase());
    previous.nodes = previous.nodes.map((node) => (node.id === 'end' ? { ...node, x: 1234 } : node));
    const def: TeamDefinition = { ...mkBase(), mode: 'parallel', graph: previous };
    const graph = rebuildDefinitionGraph(def);
    expect(graph.layout).toBe('parallel');
    expect(graph.nodes.find((node) => node.id === 'end')?.x).not.toBe(1234);
  });
});

// ADR-08 A2：后端 compile 返回的 definition_graph_json 是 canonical 模板图（含 start/end
// 装饰节点、无坐标）；前端只负责坐标赋予与存活节点位置保留，不再本地重实现模板逻辑。
describe('teamUtils.definitionGraphFromCompileJSON', () => {
  const backendSpec = JSON.stringify({
    version: 1,
    layout: 'sequential',
    nodes: [
      { id: 'start', type: 'start', label: 'Start' },
      { id: 'member-10', type: 'agent', label: 'W1', agent_id: 'a1', role: 'worker' },
      { id: 'member-20', type: 'agent', label: 'W2', agent_id: 'a2', role: 'worker' },
      { id: 'end', type: 'end', label: 'End' },
    ],
    edges: [
      { source: 'start', target: 'member-10' },
      { source: 'member-10', target: 'member-20', label: 'flow' },
      { source: 'member-20', target: 'end' },
    ],
  });

  it('parses backend template spec into a definition graph', () => {
    const graph = definitionGraphFromCompileJSON(backendSpec);
    expect(graph).not.toBeNull();
    expect(graph?.layout).toBe('sequential');
    expect(graph?.nodes.map((n) => n.id)).toEqual(['start', 'member-10', 'member-20', 'end']);
    expect(graph?.nodes[1]).toMatchObject({ type: 'agent', agent_id: 'a1', role: 'worker', label: 'W1' });
    expect(graph?.edges).toHaveLength(3);
    // 后端模板边无 id 时回退为 source-target
    expect(graph?.edges[0]?.id).toBe('start-member-10');
    expect(graph?.edges[1]).toMatchObject({ source: 'member-10', target: 'member-20', label: 'flow' });
  });

  it('assigns grid positions when no prior graph exists', () => {
    const graph = definitionGraphFromCompileJSON(backendSpec);
    const byId = new Map(graph!.nodes.map((n) => [n.id, n]));
    expect(byId.get('start')).toMatchObject({ x: 0, y: 80 });
    expect(byId.get('member-10')?.x).toBe(160);
    expect(byId.get('member-20')?.x).toBe(310);
    expect(byId.get('end')?.x).toBe(460);
  });

  it('keeps positions of surviving nodes when layout matches prior graph', () => {
    const prior = {
      version: 1,
      layout: 'sequential',
      nodes: [
        { id: 'start', type: 'start', label: '开始', x: 0, y: 80 },
        { id: 'member-10', type: 'agent', label: 'W1', agent_id: 'a1', role: 'worker', x: 999, y: 888 },
        { id: 'end', type: 'end', label: '结束', x: 460, y: 80 },
      ],
      edges: [],
    };
    const graph = definitionGraphFromCompileJSON(backendSpec, prior);
    const kept = graph?.nodes.find((n) => n.id === 'member-10');
    expect(kept).toMatchObject({ x: 999, y: 888 });
    // 新节点 member-20 仍获得网格坐标
    const added = graph?.nodes.find((n) => n.id === 'member-20');
    expect(added?.x).toBeGreaterThan(0);
  });

  it('drops prior positions when layout changed', () => {
    const prior = {
      version: 1,
      layout: 'parallel',
      nodes: [{ id: 'member-10', type: 'agent', label: 'W1', agent_id: 'a1', role: 'worker', x: 999, y: 888 }],
      edges: [],
    };
    const graph = definitionGraphFromCompileJSON(backendSpec, prior);
    expect(graph?.nodes.find((n) => n.id === 'member-10')?.x).not.toBe(999);
  });

  it('returns null for empty / invalid / node-less specs', () => {
    expect(definitionGraphFromCompileJSON('')).toBeNull();
    expect(definitionGraphFromCompileJSON('not json')).toBeNull();
    expect(definitionGraphFromCompileJSON('{}')).toBeNull();
    expect(definitionGraphFromCompileJSON('{"version":1,"nodes":[]}')).toBeNull();
  });
});

// ADR-08 A3：编辑器中角色不再人工设置，由 mode + 成员顺序派生（位置制）。
// 派生规则与团队模板语义一致：parallel 末尾成员汇总、coordinator 首位协调、
// critic_loop 生成/评审交替；adaptive/swarm 不约束角色，保持用户可编辑。
describe('teamUtils.deriveMemberRolesForMode', () => {
  const mkMembers = () => [
    { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
    { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
    { agent_id: 'a3', role: 'worker', name: 'W3', enabled: true, sort_order: 30 },
  ];
  const mkDef = (mode: string): TeamDefinition => ({
    version: 1,
    description: '',
    mode,
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: mkMembers(),
  });

  it('sequential forces every enabled member to worker and clears synthesizer', () => {
    const def = mkDef('sequential');
    def.members[1]!.role = 'coordinator';
    def.synthesizer_agent_id = 'a2';
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['worker', 'worker', 'worker']);
    expect(def.synthesizer_agent_id ?? '').toBe('');
  });

  it('parallel makes the last enabled member synthesizer and syncs synthesizer_agent_id', () => {
    const def = mkDef('parallel');
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['worker', 'worker', 'synthesizer']);
    expect(def.synthesizer_agent_id).toBe('a3');
  });

  it('parallel skips disabled members when picking the synthesizer', () => {
    const def = mkDef('parallel');
    def.members[2]!.enabled = false;
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['worker', 'synthesizer', 'worker']);
    expect(def.synthesizer_agent_id).toBe('a2');
  });

  it('parallel respects sort_order rather than array order', () => {
    const def = mkDef('parallel');
    def.members = [def.members[2]!, def.members[0]!, def.members[1]!];
    deriveMemberRolesForMode(def);
    expect(def.members.map((m) => m.role)).toEqual(['synthesizer', 'worker', 'worker']);
    expect(def.synthesizer_agent_id).toBe('a3');
  });

  it('coordinator makes the first enabled member coordinator', () => {
    const def = mkDef('coordinator');
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['coordinator', 'worker', 'worker']);
    expect(def.synthesizer_agent_id ?? '').toBe('');
  });

  it('critic_loop alternates generator/critic by sort order', () => {
    const def = mkDef('critic_loop');
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['generator', 'critic', 'generator']);
  });

  it('adaptive leaves roles untouched but clears stale synthesizer_agent_id', () => {
    const def = mkDef('adaptive');
    def.members[0]!.role = 'coordinator';
    def.synthesizer_agent_id = 'a1';
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(def.members.map((m) => m.role)).toEqual(['coordinator', 'worker', 'worker']);
    expect(def.synthesizer_agent_id ?? '').toBe('');
  });

  it('does not touch disabled member roles', () => {
    const def = mkDef('coordinator');
    def.members[0]!.enabled = false;
    def.members[0]!.role = 'critic';
    deriveMemberRolesForMode(def);
    expect(def.members[0]!.role).toBe('critic');
    expect(def.members[1]!.role).toBe('coordinator');
  });

  it('is idempotent: second call reports no change', () => {
    const def = mkDef('parallel');
    expect(deriveMemberRolesForMode(def)).toBe(true);
    expect(deriveMemberRolesForMode(def)).toBe(false);
  });
});

describe('teamUtils.groupTeamsByIndustry', () => {
  const taxonomyTree = [
    {
      id: 'ind-1',
      resource: 'taxonomy-nodes',
      key: 'finance',
      name: '金融',
      description: '',
      status: 'active',
      enabled: true,
      sort_order: 10,
      parent_id: '',
      level: 'industry',
      agent_id: '',
      provider: '',
      model: '',
      config_json: '',
      metadata_json: '',
      children: [
        {
          id: 'dep-1',
          resource: 'taxonomy-nodes',
          key: 'research',
          name: '研究部',
          description: '',
          status: 'active',
          enabled: true,
          sort_order: 10,
          parent_id: 'ind-1',
          level: 'department',
          agent_id: '',
          provider: '',
          model: '',
          config_json: '',
          metadata_json: '',
          children: [
            {
              id: 'pos-1',
              resource: 'taxonomy-nodes',
              key: 'analyst',
              name: '分析师',
              description: '',
              status: 'active',
              enabled: true,
              sort_order: 10,
              parent_id: 'dep-1',
              level: 'position',
              agent_id: '',
              provider: '',
              model: '',
              config_json: '',
              metadata_json: '',
              children: [],
            },
          ],
        },
      ],
    },
  ] as unknown as PlatformResourceTreeNode[];

  const agents: Agent[] = [
    {
      id: 'a1',
      taxonomy_position_id: 'pos-1',
    } as Agent,
  ];

  const team: Team = {
    id: 't1',
    team_key: 'demo',
    display_name: 'Demo',
    status: 'active',
    is_default: false,
    taxonomy_industry_id: '',
    definition_json: JSON.stringify({
      version: 1,
      mode: 'sequential',
      members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
    }),
    app_name: 'demo',
    linked_graph_id: '',
    has_active_run: false,
    created_at: '',
    updated_at: '',
    deleted_at: '',
  };

  it('infers industry from member agents', () => {
    expect(inferTeamIndustryId(team, agents, taxonomyTree)).toBe('ind-1');
    const groups = groupTeamsByIndustry([team], agents, taxonomyTree);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.label).toBe('金融');
    expect(groups[0]?.teams).toHaveLength(1);
  });

  it('still shows teams under disabled industries', () => {
    const disabledTree: PlatformResourceTreeNode[] = [
      {
        ...taxonomyTree[0]!,
        enabled: false,
      },
    ];
    const groups = groupTeamsByIndustry([team], agents, disabledTree);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.label).toBe('金融（已停用）');
    expect(groups[0]?.teams).toHaveLength(1);
  });

  it('places uncategorized teams exactly once in 未分类 group', () => {
    const orphanTeam: Team = {
      ...team,
      id: 't-orphan',
      taxonomy_industry_id: '',
      definition_json: JSON.stringify({
        version: 1,
        mode: 'sequential',
        members: [],
      }),
    };
    const groups = groupTeamsByIndustry([orphanTeam], [], taxonomyTree);
    const uncategorized = groups.find((g) => g.id === '__uncategorized__');
    expect(uncategorized).toBeDefined();
    expect(uncategorized?.teams).toHaveLength(1);
    expect(uncategorized?.teams[0]?.id).toBe('t-orphan');
    // Team must not appear in any other group simultaneously.
    const totalOccurrences = groups.reduce((sum, g) => sum + g.teams.filter((t) => t.id === 't-orphan').length, 0);
    expect(totalOccurrences).toBe(1);
  });

  it('does not duplicate uncategorized teams alongside categorized teams', () => {
    const orphanTeam: Team = {
      ...team,
      id: 't-orphan',
      taxonomy_industry_id: '',
      definition_json: JSON.stringify({
        version: 1,
        mode: 'sequential',
        members: [],
      }),
    };
    const groups = groupTeamsByIndustry([team, orphanTeam], agents, taxonomyTree);
    const allIds = groups.flatMap((g) => g.teams.map((t) => t.id));
    expect(allIds.sort()).toEqual(['t-orphan', 't1']);
  });
});
