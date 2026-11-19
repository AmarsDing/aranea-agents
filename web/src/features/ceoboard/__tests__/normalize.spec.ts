// 边界归一单测（整合 P2-8）：proto 生成类型 → 视图模型的全部换算规则。
import { describe, expect, it } from 'vitest';
import type { NodeDetail as ApiNodeDetail, Overview as ApiOverview } from '../../../services/kratos/ceoboard/v1/index';
import { healthOf, normalizeNodeDetail, normalizeOverview } from '../normalize';

describe('healthOf', () => {
  it('HEALTH_* 枚举 → 红黄绿灰字符串', () => {
    expect(healthOf('HEALTH_RED')).toBe('red');
    expect(healthOf('HEALTH_YELLOW')).toBe('yellow');
    expect(healthOf('HEALTH_GREEN')).toBe('green');
    expect(healthOf('HEALTH_GRAY')).toBe('gray');
  });

  it('UNSPECIFIED / undefined 归 gray(不参与红黄告警)', () => {
    expect(healthOf('HEALTH_UNSPECIFIED')).toBe('gray');
    expect(healthOf(undefined)).toBe('gray');
  });
});

describe('normalizeOverview', () => {
  it('camelCase→snake_case 全字段换算 + 缺省补齐', () => {
    const api: ApiOverview = {
      asOf: '2026-11-09T08:00:00Z',
      platforms: [
        {
          id: 'novatrader',
          name: 'NovaTrader',
          health: 'HEALTH_RED',
          probe: 'rest',
          lastOkAt: '2026-11-09T07:00:00Z',
          lastErr: 'timeout',
          latencyMs: 42,
        },
      ],
      alerts: [{ id: 'a1', level: 'HEALTH_YELLOW', target: 'wf.a', text: 't', since: '2026-11-09T07:30:00Z' }],
      approvals: {
        health: 'HEALTH_GREEN',
        lastOkAt: '2026-11-09T07:59:00Z',
        count: 1,
        items: [{ lane: 'publish', id: 'p1', title: '稿件', extra: 'e' }],
        note: undefined,
      },
      lanes: [
        {
          bizLine: 'invest',
          health: 'HEALTH_RED',
          collapsible: false,
          workflows: [
            {
              id: 'wf.a',
              name: '流程 A',
              bizLine: 'invest',
              owner: 'qa',
              version: 3,
              permissions: ['ceo'],
              health: 'HEALTH_RED',
              alerts: 2,
              volume1h: 17,
              nodes: [
                {
                  id: 'n1',
                  name: '下单',
                  order: 1,
                  health: 'HEALTH_RED',
                  reason: 'r',
                  metrics: [
                    {
                      id: 'm.lead',
                      unit: 'ms',
                      window: '1h',
                      value: 12,
                      display: '12ms',
                      health: 'HEALTH_GREEN',
                      note: undefined,
                      pending: '等待接入',
                      origin: 'event',
                    },
                  ],
                  source: {
                    adapter: 'nats-jetstream',
                    restPath: undefined,
                    skipped: false,
                    skipReason: undefined,
                    unwired: false,
                    origin: 'event',
                    stream: 'NT',
                    consuming: true,
                    connected: true,
                    msgCount: 5,
                    dupCount: 1,
                    streamMissing: false,
                    streamOptional: false,
                    backlog: true,
                    lastEventAt: '2026-11-09T07:58:00Z',
                    asOf: '2026-11-09T07:57:00Z',
                    lastOkAt: '2026-11-09T07:56:00Z',
                    lastErr: 'x',
                    failCount: 3,
                    latencyMs: 9,
                  },
                  drill: [
                    {
                      label: '看明细',
                      type: 'table',
                      path: '/x',
                      params: undefined,
                      fields: undefined,
                      emptyState: '暂无',
                      note: undefined,
                      permission: undefined,
                    },
                  ],
                },
              ],
            },
          ],
        },
      ],
      registry: {
        dir: '/d',
        loadedAt: '2026-11-09T07:00:00Z',
        loaded: ['wf.a'],
        failed: [{ file: 'f.yaml', errors: ['e1'] }],
        skipped: [{ workflow: 'wf.a', node: 'n1', metric: undefined, field: 'x', reason: 'r' }],
      },
      notice: 'n',
      registryNotice: 'rn',
    };

    const vm = normalizeOverview(api);
    expect(vm.as_of).toBe('2026-11-09T08:00:00Z');
    expect(vm.platforms[0]).toEqual({
      id: 'novatrader',
      name: 'NovaTrader',
      health: 'red',
      probe: 'rest',
      last_ok_at: '2026-11-09T07:00:00Z',
      last_err: 'timeout',
      latency_ms: 42,
    });
    expect(vm.alerts[0].level).toBe('yellow');
    expect(vm.approvals.items[0]).toEqual({ lane: 'publish', id: 'p1', title: '稿件', extra: 'e' });
    const wf = vm.lanes[0].workflows[0];
    expect(wf.biz_line).toBe('invest');
    expect(wf.volume_1h).toBe(17);
    const node = wf.nodes[0];
    expect(node.metrics[0].pending).toBe(true);
    expect(node.metrics[0].value).toBe(12);
    expect(node.source?.msg_count).toBe(5);
    expect(node.source?.dup_count).toBe(1);
    expect(node.source?.last_event_at).toBe('2026-11-09T07:58:00Z');
    expect(node.source?.as_of).toBe('2026-11-09T07:57:00Z');
    expect(node.source?.fail_count).toBe(3);
    expect(node.drill?.[0].empty_state).toBe('暂无');
    expect(vm.registry.failed[0]).toEqual({ file: 'f.yaml', errors: ['e1'] });
    expect(vm.notice).toBe('n');
    expect(vm.registry_notice).toBe('rn');
  });

  it('空消息归一为空集合 + 灰态审批,不抛错', () => {
    const vm = normalizeOverview({
      asOf: undefined,
      platforms: undefined,
      alerts: undefined,
      approvals: undefined,
      lanes: undefined,
      registry: undefined,
      notice: undefined,
      registryNotice: undefined,
    });
    expect(vm).toEqual({
      as_of: '',
      platforms: [],
      alerts: [],
      approvals: { health: 'gray', count: 0, items: [] },
      lanes: [],
      registry: { dir: '', loaded_at: '', loaded: [], failed: [], skipped: [] },
      notice: undefined,
      registry_notice: undefined,
    });
  });
});

describe('normalizeNodeDetail', () => {
  it('明细字段 + 双流事件映射(uid→event_uid)', () => {
    const d: ApiNodeDetail = {
      node: {
        id: 'n1',
        name: '下单',
        order: 1,
        health: 'HEALTH_YELLOW',
        reason: 'r',
        metrics: undefined,
        source: undefined,
        drill: undefined,
      },
      workflowId: 'wf.a',
      workflowName: '流程 A',
      enter: "event(subject='x')",
      exit: 'poll(ok)',
      asOf: '2026-11-09T08:00:00Z',
      pollSnippet: '{"k":1}',
      events: [
        {
          source: 'aranea',
          event: 'lead',
          key: 'k1',
          entityId: 'E1',
          at: '2026-11-09T07:00:00Z',
          digest: 'd',
          fields: undefined,
          deduped: false,
          status: 'ok',
          errMsg: undefined,
        },
      ],
      natsEvents: [
        {
          uid: 'nt:1',
          platform: 'novatrader',
          subject: 'trade.fill',
          subjectKey: 'trade.fill',
          eventId: 'e1',
          entityId: 'E2',
          occurredAt: '2026-11-09T06:59:00Z',
          receivedAt: '2026-11-09T07:00:00Z',
          importedFromSpool: false,
          payloadComplete: true,
        },
      ],
    };
    const vm = normalizeNodeDetail(d);
    expect(vm.workflow_id).toBe('wf.a');
    expect(vm.workflow_name).toBe('流程 A');
    expect(vm.health).toBe('yellow');
    expect(vm.metrics).toEqual([]);
    expect(vm.poll_snippet).toBe('{"k":1}');
    expect(vm.events?.[0].entity_id).toBe('E1');
    expect(vm.nats_events?.[0]).toEqual({
      event_uid: 'nt:1',
      subject: 'trade.fill',
      subject_key: 'trade.fill',
      event_id: 'e1',
      occurred_at: '2026-11-09T06:59:00Z',
      received_at: '2026-11-09T07:00:00Z',
    });
    expect(vm.fetch_err).toBeUndefined();
  });

  it('node 缺失时归一为灰态空节点', () => {
    const vm = normalizeNodeDetail({
      node: undefined,
      workflowId: 'wf.a',
      workflowName: 'A',
      enter: '',
      exit: '',
      asOf: '',
      pollSnippet: undefined,
      events: undefined,
      natsEvents: undefined,
    });
    expect(vm.health).toBe('gray');
    expect(vm.id).toBe('');
    expect(vm.events).toEqual([]);
    expect(vm.nats_events).toEqual([]);
  });
});
