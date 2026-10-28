import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import {
  advance,
  createShopWorkflows,
  createWorkflow,
  decide,
  publish,
  receiveFeedback,
  restoreWorkflows,
  revise,
  simulateVerificationFailure,
  verify,
} from '../workflow';
import { formatRate, PRODUCT_METRICS } from '../data';
import { useEcommerceLoopStore } from '../../../stores/ecommerce-loop';

describe('commerce reference workflow', () => {
  it('requires separate approval for producing and publishing drafts', () => {
    const flow = createWorkflow('launch');
    expect(advance(flow)).toBe(false);
    expect(publish(flow, 1)).toBe(false);
    expect(decide(flow, 'approved', 1)).toBe(true);
    expect(decide(flow, 'approved', 1)).toBe(false);
    expect(advance(flow)).toBe(true);
    expect(advance(flow)).toBe(true);
    expect(flow.phase).toBe('awaiting_publish_approval');
    expect(flow.drafts).toHaveLength(2);
    expect(flow.drafts.every((draft) => !draft.published)).toBe(true);
    expect(advance(flow)).toBe(false);
    expect(verify(flow)).toBe(false);
    expect(publish(flow, 1)).toBe(true);
    expect(verify(flow)).toBe(true);
    expect(flow.phase).toBe('awaiting_feedback');
    expect(receiveFeedback(flow)).toBe(true);
    expect(flow.phase).toBe('closed');
  });

  it('never assigns rejected work and does not reuse approval after revision', () => {
    const flow = createWorkflow('traffic');
    decide(flow, 'rejected', 1);
    expect(advance(flow)).toBe(false);
    expect(revise(flow, '', 1)).toBe(false);
    expect(revise(flow, 'Use a different image', 1)).toBe(true);
    expect(flow.version).toBe(2);
    expect(decide(flow, 'approved', 1)).toBe(false);
    expect(decide(flow, 'approved', 2)).toBe(true);
    advance(flow);
    advance(flow);
    expect(flow.drafts).toHaveLength(1);
    expect(revise(flow, 'Change the draft', 2)).toBe(true);
    expect(publish(flow, 2)).toBe(false);
    expect(flow.approvedVersion).toBeNull();
    expect(flow.publishVersion).toBeNull();
  });

  it('requires rework if one of the two products is missing', () => {
    const flow = createWorkflow('launch');
    decide(flow, 'approved', 1);
    advance(flow);
    advance(flow);
    expect(publish(flow, 9)).toBe(false);
    publish(flow, 1);
    flow.drafts.pop();
    verify(flow);
    expect(flow.phase).toBe('rework_required');
    expect(receiveFeedback(flow)).toBe(false);
    advance(flow);
    expect(flow.phase).toBe('awaiting_decision');
    expect(flow.version).toBe(2);
    expect(flow.approvedVersion).toBeNull();
  });

  it('does not publish duplicate or mismatched product drafts', () => {
    const flow = createWorkflow('launch');
    decide(flow, 'approved', 1);
    advance(flow);
    advance(flow);
    flow.drafts[1].scentKey = 'grapefruit';
    expect(publish(flow, 1)).toBe(false);
    flow.drafts[1].scentKey = 'lime';
    flow.drafts[1].productId = 'grapefruit';
    expect(publish(flow, 1)).toBe(false);
  });

  it('keeps a failed verification recoverable and requires fresh approval after rework', () => {
    const shops = createShopWorkflows();
    const flow = shops.a.launch;
    expect(simulateVerificationFailure(flow)).toBe(false);
    decide(flow, 'approved', 1);
    advance(flow);
    advance(flow);
    publish(flow, 1);
    expect(simulateVerificationFailure(flow)).toBe(true);
    expect(flow.phase).toBe('rework_required');
    const recovered = restoreWorkflows(JSON.stringify({ schemaVersion: 1, shops }));
    expect(recovered?.a.launch.phase).toBe('rework_required');
    expect(receiveFeedback(flow)).toBe(false);
    advance(flow);
    expect(publish(flow, 1)).toBe(false);
    expect(decide(flow, 'approved', 1)).toBe(false);
    decide(flow, 'approved', 2);
    advance(flow);
    advance(flow);
    publish(flow, 2);
    verify(flow);
    expect(flow.phase).toBe('awaiting_feedback');
  });

  it('validates restored records and rejects impossible completed states', () => {
    const shops = createShopWorkflows();
    const serialize = () => JSON.stringify({ schemaVersion: 1, shops });
    expect(restoreWorkflows(serialize())).toEqual(shops);
    shops.a.launch.phase = 'closed';
    shops.a.launch.history.push('closed');
    expect(restoreWorkflows(serialize())).toBeNull();
    expect(restoreWorkflows('{broken')).toBeNull();
    expect(restoreWorkflows('{}')).toBeNull();
  });

  it('calculates conditional funnel rates without inventing missing values', () => {
    expect(PRODUCT_METRICS.reduce((sum, row) => sum + row.visitors, 0)).toBe(1240);
    expect(PRODUCT_METRICS.reduce((sum, row) => sum + row.addToCart, 0)).toBe(74);
    expect(PRODUCT_METRICS.reduce((sum, row) => sum + row.orders, 0)).toBe(31);
    expect(formatRate(74, 1240)).toBe('6.0%');
    expect(formatRate(31, 74)).toBe('41.9%');
    expect(formatRate(0, 10)).toBe('0.0%');
    expect(formatRate(null, 10)).toBe('—');
    expect(formatRate(1, 0)).toBe('—');
  });
});

describe('commerce store isolation and recovery', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
  });
  it('keeps shops and cases independent, and navigation does not advance work', () => {
    const store = useEcommerceLoopStore();
    store.decide('launch', 'approved', 1);
    store.setScene(4);
    store.focus(5);
    expect(store.currentCases.launch.phase).toBe('approved');
    expect(store.currentCases.traffic.phase).toBe('awaiting_decision');
    store.selectShop('b');
    expect(store.currentCases.launch.phase).toBe('awaiting_decision');
    store.selectShop('a');
    expect(store.currentCases.launch.phase).toBe('approved');
  });
  it('restores saved approval after refreshing the store', () => {
    const first = useEcommerceLoopStore();
    first.decide('traffic', 'rejected', 1);
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.currentCases.traffic.phase).toBe('rejected');
    expect(restored.advance('traffic')).toBe(false);
  });
});
