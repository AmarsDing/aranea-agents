// proto 生成类型（camelCase + HEALTH_* 枚举 + RFC3339 时间串）→ 视图模型
// （snake_case + 红黄绿灰字符串）的唯一换算点（整合 P2-8）。
// 组件层只面对 ./types 的稳定契约；双跑对比同样以视图模型做归一。
// 口径：缺省值一律补齐（数字 0 / 串 '' / 数组 []），health 未知档归 gray——
// 与原 JSON 协议下「字段缺失即零值」的行为一致。

import type {
  AlertView as ApiAlert,
  ApprovalsView as ApiApprovals,
  Health as ApiHealth,
  LaneView as ApiLane,
  MetricView as ApiMetric,
  NodeDetail as ApiNodeDetail,
  NodeView as ApiNode,
  Overview as ApiOverview,
  PlatformView as ApiPlatform,
  RegistryReport as ApiRegistry,
  SourceView as ApiSource,
  WorkflowView as ApiWorkflow,
} from '../../services/kratos/ceoboard/v1/index';
import type {
  AlertView,
  ApprovalsView,
  LaneView,
  MetricView,
  NodeDetail,
  NodeView,
  Overview,
  PlatformView,
  RegistryView,
  SourceView,
  WorkflowView,
} from './types';

export function healthOf(h?: ApiHealth): string {
  switch (h) {
    case 'HEALTH_RED':
      return 'red';
    case 'HEALTH_YELLOW':
      return 'yellow';
    case 'HEALTH_GREEN':
      return 'green';
    default:
      // UNSPECIFIED / undefined 归 gray（未接入/未知，不参与红黄告警）。
      return 'gray';
  }
}

const s = (v: string | undefined, d = ''): string => v ?? d;
const n = (v: number | undefined, d = 0): number => v ?? d;

function normalizeMetric(m: ApiMetric): MetricView {
  return {
    id: s(m.id),
    unit: s(m.unit),
    window: s(m.window),
    value: m.value ?? null,
    display: s(m.display),
    health: healthOf(m.health),
    note: s(m.note) || undefined,
    pending: !!m.pending,
    origin: s(m.origin) || undefined,
  };
}

function normalizeSource(src: ApiSource): SourceView {
  return {
    adapter: s(src.adapter),
    rest_path: s(src.restPath) || undefined,
    skipped: src.skipped || undefined,
    skip_reason: s(src.skipReason) || undefined,
    unwired: src.unwired || undefined,
    origin: s(src.origin) || undefined,
    stream: s(src.stream) || undefined,
    consuming: src.consuming || undefined,
    connected: src.connected || undefined,
    stream_missing: src.streamMissing || undefined,
    stream_optional: src.streamOptional || undefined,
    backlog: src.backlog || undefined,
    msg_count: src.msgCount,
    dup_count: src.dupCount,
    last_event_at: s(src.lastEventAt) || undefined,
    as_of: s(src.asOf) || undefined,
    last_ok_at: s(src.lastOkAt) || undefined,
    last_err: s(src.lastErr) || undefined,
    fail_count: src.failCount,
    latency_ms: src.latencyMs,
  };
}

function normalizeNode(node: ApiNode): NodeView {
  return {
    id: s(node.id),
    name: s(node.name),
    order: n(node.order),
    health: healthOf(node.health),
    reason: s(node.reason),
    metrics: (node.metrics ?? []).map(normalizeMetric),
    source: node.source ? normalizeSource(node.source) : undefined,
    drill: (node.drill ?? []).map((d) => ({
      label: s(d.label),
      type: s(d.type),
      path: s(d.path),
      empty_state: s(d.emptyState) || undefined,
      note: s(d.note) || undefined,
    })),
  };
}

function normalizeWorkflow(wf: ApiWorkflow): WorkflowView {
  return {
    id: s(wf.id),
    name: s(wf.name),
    biz_line: s(wf.bizLine),
    owner: s(wf.owner),
    version: n(wf.version),
    permissions: wf.permissions ?? [],
    health: healthOf(wf.health),
    alerts: n(wf.alerts),
    volume_1h: n(wf.volume1h),
    nodes: (wf.nodes ?? []).map(normalizeNode),
  };
}

function normalizeLane(l: ApiLane): LaneView {
  return {
    biz_line: s(l.bizLine),
    collapsible: !!l.collapsible,
    workflows: (l.workflows ?? []).map(normalizeWorkflow),
  };
}

function normalizePlatform(p: ApiPlatform): PlatformView {
  return {
    id: s(p.id),
    name: s(p.name),
    health: healthOf(p.health),
    probe: s(p.probe),
    last_ok_at: s(p.lastOkAt) || undefined,
    last_err: s(p.lastErr) || undefined,
    latency_ms: p.latencyMs,
  };
}

function normalizeAlert(a: ApiAlert): AlertView {
  return {
    id: s(a.id),
    level: healthOf(a.level),
    target: s(a.target),
    text: s(a.text),
    since: s(a.since),
  };
}

function normalizeApprovals(a?: ApiApprovals): ApprovalsView {
  if (!a) return { health: 'gray', count: 0, items: [] };
  return {
    health: healthOf(a.health),
    last_ok_at: s(a.lastOkAt) || undefined,
    count: n(a.count),
    items: (a.items ?? []).map((it) => ({
      lane: s(it.lane),
      id: s(it.id),
      title: s(it.title),
      extra: s(it.extra) || undefined,
    })),
    note: s(a.note) || undefined,
  };
}

function normalizeRegistry(r?: ApiRegistry): RegistryView {
  return {
    dir: s(r?.dir),
    loaded_at: s(r?.loadedAt),
    loaded: r?.loaded ?? [],
    failed: (r?.failed ?? []).map((f) => ({ file: s(f.file), errors: f.errors ?? [] })),
    skipped: (r?.skipped ?? []).map((k) => ({
      workflow: s(k.workflow),
      node: s(k.node) || undefined,
      metric: s(k.metric) || undefined,
      field: s(k.field),
      reason: s(k.reason),
    })),
  };
}

export function normalizeOverview(o: ApiOverview): Overview {
  return {
    as_of: s(o.asOf),
    platforms: (o.platforms ?? []).map(normalizePlatform),
    alerts: (o.alerts ?? []).map(normalizeAlert),
    approvals: normalizeApprovals(o.approvals),
    lanes: (o.lanes ?? []).map(normalizeLane),
    registry: normalizeRegistry(o.registry),
    notice: s(o.notice) || undefined,
    registry_notice: s(o.registryNotice) || undefined,
  };
}

export function normalizeNodeDetail(d: ApiNodeDetail): NodeDetail {
  const node = normalizeNode(d.node ?? ({} as ApiNode));
  return {
    ...node,
    workflow_id: s(d.workflowId),
    workflow_name: s(d.workflowName),
    enter: s(d.enter),
    exit: s(d.exit),
    as_of: s(d.asOf),
    poll_snippet: s(d.pollSnippet) || undefined,
    events: (d.events ?? []).map((e) => ({
      source: s(e.source),
      event: s(e.event),
      key: s(e.key),
      entity_id: s(e.entityId) || undefined,
      at: s(e.at),
      digest: s(e.digest),
      status: s(e.status) || undefined,
    })),
    nats_events: (d.natsEvents ?? []).map((e) => ({
      event_uid: s(e.uid),
      subject: s(e.subject),
      subject_key: s(e.subjectKey),
      event_id: s(e.eventId),
      occurred_at: s(e.occurredAt) || undefined,
      received_at: s(e.receivedAt),
    })),
  };
}
