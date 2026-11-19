// 公司运转图图模型单测（2026-11-10 方案 §4.2/§4.3 口径锁定）。
import { describe, expect, it } from 'vitest';
import { buildCompanyGraph, isNodeWorking, WORKING_FRESH_MS } from '../companyGraph';
import { overviewLevel } from '../healthRank';
import type { NodeView, Overview, SourceView, WorkflowView } from '../types';

const AS_OF = '2026-11-10T08:00:00Z';
const ago = (ms: number): string => new Date(new Date(AS_OF).getTime() - ms).toISOString();

function source(partial: Partial<SourceView> = {}): SourceView {
  return { adapter: 'nats-jetstream', ...partial };
}
function node(partial: Partial<NodeView> = {}): NodeView {
  return { id: 'n1', name: '环节一', order: 1, health: 'green', reason: '', metrics: [], ...partial };
}
function wf(partial: Partial<WorkflowView> = {}): WorkflowView {
  return {
    id: 'wf1',
    name: '流程一',
    biz_line: 'ecommerce',
    owner: 'team',
    version: 1,
    permissions: [],
    health: 'green',
    alerts: 0,
    volume_1h: 0,
    nodes: [node()],
    ...partial,
  };
}
function ov(partial: Partial<Overview> = {}): Overview {
  return {
    as_of: AS_OF,
    platforms: [],
    alerts: [],
    approvals: { health: 'green', count: 0, items: [] },
    lanes: [{ biz_line: 'ecommerce', collapsible: true, workflows: [wf()] }],
    registry: { dir: '', loaded_at: '', loaded: [], failed: [], skipped: [] },
    ...partial,
  };
}

describe('isNodeWorking', () => {
  it('事件轨新鲜（5min 内）→ 在干活', () => {
    expect(isNodeWorking(source({ last_event_at: ago(0) }), AS_OF)).toBe(true);
    expect(isNodeWorking(source({ last_event_at: ago(WORKING_FRESH_MS) }), AS_OF)).toBe(true);
  });

  it('事件轨陈旧（超过 5min）→ 不在干活', () => {
    expect(isNodeWorking(source({ last_event_at: ago(WORKING_FRESH_MS + 1) }), AS_OF)).toBe(false);
  });

  it('对账轨新鲜 → 在干活（rest-poll 无 consuming 也转圈）', () => {
    expect(isNodeWorking(source({ adapter: 'rest-poll', last_ok_at: ago(60_000) }), AS_OF)).toBe(true);
  });

  it('webhook 型只有 last_event_at（无 consuming）→ 新鲜即转圈', () => {
    expect(isNodeWorking(source({ adapter: 'webhook-recv', last_event_at: ago(30_000) }), AS_OF)).toBe(true);
  });

  it('双轨 OR：事件停但对账新鲜 → 在干活；反之亦然', () => {
    expect(isNodeWorking(source({ last_event_at: ago(3_600_000), last_ok_at: ago(1_000) }), AS_OF)).toBe(true);
    expect(isNodeWorking(source({ last_event_at: ago(1_000), last_ok_at: ago(3_600_000) }), AS_OF)).toBe(true);
    expect(isNodeWorking(source({ last_event_at: ago(3_600_000), last_ok_at: ago(3_600_000) }), AS_OF)).toBe(false);
  });

  it('consuming=true 但无新鲜事件 → 不转圈（通道开着≠在干活）', () => {
    expect(isNodeWorking(source({ consuming: true }), AS_OF)).toBe(false);
    expect(isNodeWorking(source({ consuming: true, last_event_at: ago(3_600_000) }), AS_OF)).toBe(false);
  });

  it('负差（事件晚于 as_of，时钟偏移）→ 保守不转圈', () => {
    expect(isNodeWorking(source({ last_event_at: ago(-1_000) }), AS_OF)).toBe(false);
  });

  it('skipped / unwired / 缺 source → 不转圈（拿不到就直说）', () => {
    expect(isNodeWorking(source({ skipped: true, last_event_at: ago(0) }), AS_OF)).toBe(false);
    expect(isNodeWorking(source({ unwired: true, last_ok_at: ago(0) }), AS_OF)).toBe(false);
    expect(isNodeWorking(undefined, AS_OF)).toBe(false);
  });

  it('as_of 非法 / 时间串非法 → 不转圈', () => {
    expect(isNodeWorking(source({ last_event_at: ago(0) }), 'not-a-date')).toBe(false);
    expect(isNodeWorking(source({ last_event_at: 'not-a-date' }), AS_OF)).toBe(false);
  });
});

describe('buildCompanyGraph', () => {
  it('聚合：流程数/环节数/业务量/告警求和，working 逐层上传', () => {
    const g = buildCompanyGraph(
      ov({
        lanes: [
          {
            biz_line: 'ecommerce',
            collapsible: true,
            workflows: [
              wf({
                id: 'w1',
                volume_1h: 10,
                alerts: 1,
                nodes: [
                  node({ id: 'a', source: source({ last_event_at: ago(1_000) }) }),
                  node({ id: 'b', order: 2, source: source({ last_event_at: ago(3_600_000) }) }),
                ],
              }),
              wf({ id: 'w2', volume_1h: 5, alerts: 2, nodes: [node({ id: 'c' })] }),
            ],
          },
          { biz_line: 'content', collapsible: true, workflows: [wf({ id: 'w3', volume_1h: 7 })] },
        ],
        alerts: [
          { id: 'a1', level: 'red', target: 'x', text: '', since: AS_OF },
          { id: 'a2', level: 'yellow', target: 'y', text: '', since: AS_OF },
        ],
        approvals: { health: 'green', count: 3, items: [] },
      }),
    );
    expect(g.totalWorkflows).toBe(3);
    expect(g.totalNodes).toBe(4);
    expect(g.workingNodes).toBe(1);
    expect(g.alertCount).toBe(2);
    expect(g.approvalCount).toBe(3);
    expect(g.lanes[0].volume1h).toBe(15);
    expect(g.lanes[0].alerts).toBe(3);
    expect(g.lanes[0].working).toBe(true);
    expect(g.lanes[0].workflows[0].working).toBe(true);
    expect(g.lanes[0].workflows[1].working).toBe(false);
    expect(g.lanes[1].working).toBe(false);
  });

  it('lane 健康 = 最差 workflow（与泳道卡同口径）', () => {
    const g = buildCompanyGraph(
      ov({
        lanes: [
          {
            biz_line: 'ecommerce',
            collapsible: false,
            workflows: [
              wf({ id: 'g', health: 'green' }),
              wf({ id: 'r', health: 'red' }),
              wf({ id: 'y', health: 'yellow' }),
            ],
          },
        ],
      }),
    );
    expect(g.lanes[0].health).toBe('red');
  });

  it('worstReason 仅红/黄时取最差节点原因；全绿/灰为空（组件走 runningOk 文案）', () => {
    const g = buildCompanyGraph(
      ov({
        lanes: [
          {
            biz_line: 'ecommerce',
            collapsible: false,
            workflows: [
              wf({
                id: 'bad',
                health: 'red',
                nodes: [node({ id: 'ok', health: 'green' }), node({ id: 'bad', health: 'red', reason: '探针失败: x' })],
              }),
              wf({ id: 'good', health: 'green', nodes: [node({ health: 'green', reason: 'durable 消费中' })] }),
            ],
          },
        ],
      }),
    );
    expect(g.lanes[0].workflows[0].worstHealth).toBe('red');
    expect(g.lanes[0].workflows[0].worstReason).toBe('探针失败: x');
    expect(g.lanes[0].workflows[1].worstReason).toBe('');
    expect(g.lanes[0].workflows[1].worstHealth).toBe('green');
  });

  it('空 lanes / 空 nodes 不崩', () => {
    const g = buildCompanyGraph(ov({ lanes: [] }));
    expect(g.lanes).toEqual([]);
    expect(g.totalWorkflows).toBe(0);
    expect(g.totalNodes).toBe(0);
    expect(g.level).toBe('green');
    const g2 = buildCompanyGraph(
      ov({ lanes: [{ biz_line: 'ecommerce', collapsible: true, workflows: [wf({ nodes: [] })] }] }),
    );
    expect(g2.lanes[0].health).toBe('green');
    expect(g2.lanes[0].workflows[0].worstHealth).toBe('gray');
    expect(g2.lanes[0].workflows[0].worstReason).toBe('');
  });

  it('skipped/unwired 节点不参与转圈', () => {
    const g = buildCompanyGraph(
      ov({
        lanes: [
          {
            biz_line: 'ecommerce',
            collapsible: true,
            workflows: [
              wf({
                nodes: [
                  node({ id: 's', source: source({ skipped: true, last_event_at: ago(0) }) }),
                  node({ id: 'u', source: source({ unwired: true, last_ok_at: ago(0) }) }),
                ],
              }),
            ],
          },
        ],
      }),
    );
    expect(g.workingNodes).toBe(0);
    expect(g.lanes[0].working).toBe(false);
  });
});

describe('overviewLevel（Hero 与 CEO 卡共用口径）', () => {
  it('平台离线 → red（即使流程全绿）', () => {
    const o = ov({ platforms: [{ id: 'p', name: 'P', health: 'red', probe: 'rest' }] });
    expect(overviewLevel(o)).toBe('red');
    expect(buildCompanyGraph(o).level).toBe('red');
  });

  it('红流程 → red', () => {
    const o = ov({ lanes: [{ biz_line: 'ecommerce', collapsible: false, workflows: [wf({ health: 'red' })] }] });
    expect(overviewLevel(o)).toBe('red');
  });

  it('黄流程 → yellow', () => {
    const o = ov({ lanes: [{ biz_line: 'ecommerce', collapsible: false, workflows: [wf({ health: 'yellow' })] }] });
    expect(overviewLevel(o)).toBe('yellow');
  });

  it('待拍板 > 0 → yellow（即使流程全绿）', () => {
    const o = ov({ approvals: { health: 'green', count: 1, items: [] } });
    expect(overviewLevel(o)).toBe('yellow');
    expect(buildCompanyGraph(o).level).toBe('yellow');
  });

  it('全绿无待拍板 → green', () => {
    expect(overviewLevel(ov())).toBe('green');
  });
});
