import { SHOPS } from './data';
import type { CaseId, DemoDraft, DemoPhase, DemoWorkflow, ShopWorkflows } from './types';

export const PHASES: DemoPhase[] = [
  'awaiting_decision',
  'rejected',
  'approved',
  'drafting',
  'awaiting_publish_approval',
  'verification_pending',
  'rework_required',
  'awaiting_feedback',
  'closed',
];
export const STORAGE_KEY = 'aranea:ecommerce-reference:v1';

export function createWorkflow(kind: CaseId): DemoWorkflow {
  return {
    kind,
    phase: 'awaiting_decision',
    version: 1,
    approvedVersion: null,
    publishVersion: null,
    note: '',
    drafts: [],
    history: ['awaiting_decision'],
  };
}

export function createShopWorkflows(): ShopWorkflows {
  return Object.fromEntries(
    SHOPS.map((shop) => [shop, { traffic: createWorkflow('traffic'), launch: createWorkflow('launch') }]),
  ) as ShopWorkflows;
}

function transition(flow: DemoWorkflow, next: DemoPhase): void {
  flow.phase = next;
  flow.history.push(next);
  if (flow.history.length > 80) flow.history = flow.history.slice(-80);
}

export function decide(flow: DemoWorkflow, decision: 'approved' | 'rejected', version: number): boolean {
  if (flow.phase !== 'awaiting_decision' || flow.version !== version) return false;
  flow.approvedVersion = decision === 'approved' ? version : null;
  transition(flow, decision);
  return true;
}

export function revise(flow: DemoWorkflow, note: string, version: number): boolean {
  if (
    !note.trim() ||
    note.length > 2000 ||
    flow.version !== version ||
    !['awaiting_decision', 'rejected', 'awaiting_publish_approval', 'rework_required'].includes(flow.phase)
  )
    return false;
  flow.note = note.trim();
  flow.version += 1;
  flow.approvedVersion = null;
  flow.publishVersion = null;
  flow.drafts = [];
  transition(flow, 'awaiting_decision');
  return true;
}

export function advance(flow: DemoWorkflow): boolean {
  if (flow.phase === 'approved' && flow.approvedVersion === flow.version) {
    transition(flow, 'drafting');
  } else if (flow.phase === 'drafting' && flow.approvedVersion === flow.version) {
    const products = flow.kind === 'launch' ? ['grapefruit', 'lime'] : ['seaSalt'];
    flow.drafts = products.map((productId) => ({
      id: `${flow.kind}-${productId}-v${flow.version}`,
      productId,
      titleKey: productId,
      scentKey: productId,
      published: false,
    }));
    transition(flow, 'awaiting_publish_approval');
  } else if (flow.phase === 'rework_required') {
    flow.version += 1;
    flow.approvedVersion = null;
    flow.publishVersion = null;
    flow.drafts = [];
    transition(flow, 'awaiting_decision');
  } else return false;
  return true;
}

export function publish(flow: DemoWorkflow, version: number): boolean {
  if (
    flow.phase !== 'awaiting_publish_approval' ||
    version !== flow.version ||
    flow.approvedVersion !== version ||
    !validDrafts(flow)
  )
    return false;
  flow.publishVersion = version;
  flow.drafts.forEach((draft) => {
    draft.published = true;
  });
  transition(flow, 'verification_pending');
  return true;
}

export function verify(flow: DemoWorkflow): boolean {
  if (flow.phase !== 'verification_pending') return false;
  const passed =
    flow.publishVersion === flow.version && validDrafts(flow) && flow.drafts.every((draft) => draft.published);
  transition(flow, passed ? 'awaiting_feedback' : 'rework_required');
  return true;
}

export function simulateVerificationFailure(flow: DemoWorkflow): boolean {
  if (flow.phase !== 'verification_pending' || !flow.drafts.length) return false;
  flow.drafts[0].scentKey = flow.kind === 'launch' ? 'lime' : 'grapefruit';
  return verify(flow);
}

export function receiveFeedback(flow: DemoWorkflow): boolean {
  if (flow.phase !== 'awaiting_feedback') return false;
  transition(flow, 'closed');
  return true;
}

export function validDrafts(flow: DemoWorkflow): boolean {
  const expected = flow.kind === 'launch' ? ['grapefruit', 'lime'] : ['seaSalt'];
  return (
    flow.drafts.length === expected.length &&
    expected.every(
      (product) =>
        flow.drafts.filter(
          (draft) => draft.productId === product && draft.titleKey === product && draft.scentKey === product,
        ).length === 1,
    )
  );
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function validDraft(value: unknown): value is DemoDraft {
  return (
    record(value) &&
    typeof value.id === 'string' &&
    typeof value.productId === 'string' &&
    typeof value.titleKey === 'string' &&
    typeof value.scentKey === 'string' &&
    typeof value.published === 'boolean'
  );
}

function validWorkflow(value: unknown, kind: CaseId): value is DemoWorkflow {
  if (
    !record(value) ||
    value.kind !== kind ||
    !PHASES.includes(value.phase as DemoPhase) ||
    !Number.isInteger(value.version) ||
    Number(value.version) < 1 ||
    typeof value.note !== 'string' ||
    value.note.length > 2000 ||
    !Array.isArray(value.drafts) ||
    !value.drafts.every(validDraft) ||
    !Array.isArray(value.history) ||
    value.history.length > 80 ||
    !value.history.every((phase) => PHASES.includes(phase as DemoPhase))
  )
    return false;
  const flow = value as unknown as DemoWorkflow;
  if (flow.approvedVersion !== null && flow.approvedVersion !== flow.version) return false;
  if (flow.publishVersion !== null && flow.publishVersion !== flow.version) return false;
  if (flow.history.at(-1) !== flow.phase) return false;
  if (
    [
      'approved',
      'drafting',
      'awaiting_publish_approval',
      'verification_pending',
      'awaiting_feedback',
      'closed',
    ].includes(flow.phase) &&
    flow.approvedVersion !== flow.version
  )
    return false;
  if (
    ['awaiting_publish_approval', 'verification_pending', 'awaiting_feedback', 'closed'].includes(flow.phase) &&
    !validDrafts(flow)
  )
    return false;
  if (
    ['verification_pending', 'awaiting_feedback', 'closed'].includes(flow.phase) &&
    (flow.publishVersion !== flow.version || !flow.drafts.every((draft) => draft.published))
  )
    return false;
  return true;
}

export function restoreWorkflows(raw: string): ShopWorkflows | null {
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!record(parsed) || parsed.schemaVersion !== 1 || !record(parsed.shops)) return null;
    const shops = parsed.shops;
    for (const shop of SHOPS) {
      const cases = shops[shop];
      if (!record(cases) || !validWorkflow(cases.traffic, 'traffic') || !validWorkflow(cases.launch, 'launch'))
        return null;
    }
    return shops as ShopWorkflows;
  } catch {
    return null;
  }
}
