// 经营驾驶舱视图模型（整合 P2-8）：与 ceo-dashboard 原前端 `web/src/types.ts`
// 逐字段一致——保留 snake_case 与 'red'|'yellow'|'green'|'gray' 字符串健康度，
// 作为组件层的稳定契约；proto 生成类型（camelCase + HEALTH_* 枚举）与本模型
// 之间的换算全部收敛在 ./normalize.ts（双跑对比也以本模型为准做归一）。

export type HealthLevel = 'red' | 'yellow' | 'green' | 'gray';

export interface MetricView {
  id: string;
  unit: string;
  window: string;
  value: number | null;
  display: string;
  health: string;
  note?: string;
  pending?: boolean;
  origin?: string;
}
export interface SourceView {
  adapter: string;
  rest_path?: string;
  skipped?: boolean;
  skip_reason?: string;
  unwired?: boolean;
  origin?: string;
  stream?: string;
  consuming?: boolean;
  connected?: boolean;
  stream_missing?: boolean;
  stream_optional?: boolean;
  backlog?: boolean;
  msg_count?: number;
  dup_count?: number;
  last_event_at?: string;
  as_of?: string;
  last_ok_at?: string;
  last_err?: string;
  fail_count?: number;
  latency_ms?: number;
}
export interface DrillView {
  label: string;
  type: string;
  path: string;
  empty_state?: string;
  note?: string;
}
export interface NodeView {
  id: string;
  name: string;
  order: number;
  health: string;
  reason: string;
  metrics: MetricView[];
  source?: SourceView;
  drill?: DrillView[];
}
export interface WorkflowView {
  id: string;
  name: string;
  biz_line: string;
  owner: string;
  version: number;
  permissions: string[];
  health: string;
  alerts: number;
  volume_1h: number;
  nodes: NodeView[];
}
export interface LaneView {
  biz_line: string;
  collapsible: boolean;
  workflows: WorkflowView[];
}
export interface PlatformView {
  id: string;
  name: string;
  health: string;
  probe: string;
  last_ok_at?: string;
  last_err?: string;
  latency_ms?: number;
}
export interface AlertView {
  id: string;
  level: string;
  target: string;
  text: string;
  since: string;
}
export interface ApprovalItem {
  lane: string;
  id: string;
  title: string;
  extra?: string;
}
export interface ApprovalsView {
  health: string;
  last_ok_at?: string;
  count: number;
  items: ApprovalItem[];
  note?: string;
}
export interface RegistryView {
  dir: string;
  loaded_at: string;
  loaded: string[];
  failed: { file: string; errors: string[] }[];
  skipped: { workflow: string; node?: string; metric?: string; field: string; reason: string }[];
}
export interface Overview {
  as_of: string;
  platforms: PlatformView[];
  alerts: AlertView[];
  approvals: ApprovalsView;
  lanes: LaneView[];
  registry: RegistryView;
  notice?: string;
  registry_notice?: string;
}
export interface DeadLetter {
  id: number;
  kind: string;
  source: string;
  ref_key: string;
  payload: string;
  reason: string;
  failed_at: string;
  replayed_at?: string;
}
export interface NodeDetail extends NodeView {
  workflow_id: string;
  workflow_name: string;
  enter: string;
  exit: string;
  as_of: string;
  poll_snippet?: string;
  /** 下钻回源失败时的降级说明（前端本地字段，不由后端消息表达）。 */
  fetch_err?: string;
  events?: {
    source: string;
    event: string;
    key: string;
    entity_id?: string;
    at: string;
    digest: string;
    status?: string;
  }[];
  nats_events?: {
    event_uid: string;
    subject: string;
    subject_key: string;
    event_id: string;
    occurred_at?: string;
    received_at: string;
  }[];
}

// ——— 电商经营快照（GET /v1/ceoboard/company）———
// 该接口是只读 JSON BFF，字段与后端 bizview.Snapshot 一一对应；
// 所有金额单位为 micro USD（1 USD = 1e6），前端按需换算展示。

export interface OpsDrill {
  label: string;
  label_key?: string;
  path: string;
}

export interface OpsMetric {
  id: string;
  name: string;
  name_key?: string;
  value: number;
  display: string;
  unit: string;
  window: string;
  health: string;
  source: string;
  rule?: string;
  note?: string;
  pending: boolean;
  drill?: OpsDrill;
}

export interface OpsEntity {
  id: string;
  name: string;
  name_key?: string;
  kind: string;
  count: number;
  status?: string;
  health: string;
  note?: string;
  drill?: OpsDrill;
}

export interface OpsModule {
  id: string;
  name: string;
  name_key?: string;
  tone: string;
  job: string;
  job_key?: string;
  scope: string;
  scope_key?: string;
  cadence: string;
  cadence_key?: string;
  health: string;
  working: boolean;
  metrics: OpsMetric[];
  entities: OpsEntity[];
}

export interface OpsAnomaly {
  id: string;
  code: string;
  level: string;
  module: string;
  text: string;
  params?: Record<string, number | string>;
  since?: string;
  drill?: OpsDrill;
}

export interface OpsCoverage {
  id: string;
  name: string;
  name_key?: string;
  ready: boolean;
  note: string;
  note_key?: string;
}

export interface CompanyFacts {
  name: string;
  health: string;
  departments: number;
  positions: number;
  agents: number;
  teams: number;
  active_sessions_24h: number;
  running_runs: number;
  today_cost_micro_usd: number;
  today_tokens: number;
}

export interface CompanySnapshot {
  as_of: string;
  window: string;
  scope_note: string;
  company: CompanyFacts;
  kpis: OpsMetric[];
  modules: OpsModule[];
  anomalies: OpsAnomaly[];
  coverage: OpsCoverage[];
  notice?: string;
}

