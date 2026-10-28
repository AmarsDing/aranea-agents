import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useEcommerceLoopStore } from '../../../stores/ecommerce-loop';
import { restoreViewState, VIEW_STORAGE_KEY } from '../viewState';
import { STORAGE_KEY } from '../workflow';

beforeEach(() => {
  localStorage.clear();
  setActivePinia(createPinia());
});

describe('commerce view context', () => {
  it('keeps the launch page when switching stores and isolates task progress', () => {
    const store = useEcommerceLoopStore();
    store.setScene(4);
    store.decide('launch', 'approved', 1);
    store.selectShop('c');
    expect(store.scene).toBe(4);
    expect(store.activeCase).toBe('launch');
    expect(store.currentCases.launch.phase).toBe('awaiting_decision');
    store.selectShop('a');
    expect(store.currentCases.launch.phase).toBe('approved');
  });

  it('restores store, view, focused step, selection, task and business progress independently', () => {
    const first = useEcommerceLoopStore();
    first.viewMode = 'reference';
    first.setScene(4);
    first.selectShop('c');
    first.focus(5);
    first.selectedProduct = 'lavender';
    first.decide('launch', 'approved', 1);
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.viewMode).toBe('reference');
    expect(restored.scene).toBe(4);
    expect(restored.selectedShop).toBe('c');
    expect(restored.selectedProduct).toBe('lavender');
    expect(restored.activeCase).toBe('launch');
    expect(restored.focusStep).toBe(5);
    expect(restored.currentCases.launch.phase).toBe('approved');
    expect(restored.storageWarning).toBe('');
  });

  it('loads existing schema 1 workflows with the readable view as default', () => {
    const first = useEcommerceLoopStore();
    first.decide('traffic', 'approved', 1);
    expect(localStorage.getItem(VIEW_STORAGE_KEY)).toBeNull();
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.currentCases.traffic.phase).toBe('approved');
    expect(restored.viewMode).toBe('workbench');
  });

  it('does not lose a valid workflow when the visual context is corrupt', () => {
    useEcommerceLoopStore().decide('traffic', 'approved', 1);
    localStorage.setItem(VIEW_STORAGE_KEY, '{broken');
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.currentCases.traffic.phase).toBe('approved');
    expect(restored.storageWarning).toBe('invalid');
  });

  it('preserves a workflow warning when valid view restoration or navigation succeeds', () => {
    const first = useEcommerceLoopStore();
    first.selectShop('d');
    localStorage.setItem(STORAGE_KEY, '{broken');
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.selectedShop).toBe('d');
    restored.setScene(3);
    expect(restored.storageWarning).toBe('invalid');
  });

  it('rejects invalid stores, products, modes and focus values', () => {
    const store = useEcommerceLoopStore();
    store.selectShop('b');
    const saved = JSON.parse(localStorage.getItem(VIEW_STORAGE_KEY)!);
    for (const [field, value] of [
      ['selectedShop', 'z'],
      ['scene', 9],
      ['selectedProduct', 'unknown'],
      ['viewMode', 'admin'],
      ['focusStep', 12],
    ]) {
      expect(
        restoreViewState(JSON.stringify({ ...saved, context: { ...saved.context, [String(field)]: value } })),
      ).toBeNull();
    }
  });

  it('normalizes focus that does not belong to the restored scene', () => {
    const store = useEcommerceLoopStore();
    store.selectShop('b');
    const saved = JSON.parse(localStorage.getItem(VIEW_STORAGE_KEY)!);
    const context = restoreViewState(
      JSON.stringify({ ...saved, context: { ...saved.context, scene: 3, focusStep: 5, companyFocused: true } }),
    );
    expect(context?.focusStep).toBeNull();
    expect(context?.companyFocused).toBe(false);
  });

  it('restores a report camera without advancing or approving the task', () => {
    const first = useEcommerceLoopStore();
    first.setScene(3);
    first.focusReference('suggestion');
    setActivePinia(createPinia());
    const restored = useEcommerceLoopStore();
    restored.restore();
    expect(restored.referenceFocus).toBe('suggestion');
    expect(restored.currentCases.traffic.phase).toBe('awaiting_decision');
    expect(restored.currentCases.traffic.approvedVersion).toBeNull();
    expect(restored.currentCases.traffic.drafts).toEqual([]);
  });

  it('keeps steps and panel cameras exclusive and ignores cross-scene targets', () => {
    const store = useEcommerceLoopStore();
    store.setScene(2);
    store.focusReference('ceo-summary');
    store.focus(4);
    expect(store.referenceFocus).toBeNull();
    store.focusReference('ceo-detail');
    expect(store.focusStep).toBeNull();
    store.focusReference('anomaly');
    expect(store.referenceFocus).toBe('ceo-detail');
    store.setScene(4);
    expect(store.referenceFocus).toBeNull();
  });

  it('accepts old context without camera and normalizes unknown or conflicting targets', () => {
    const store = useEcommerceLoopStore();
    store.setScene(2);
    const saved = JSON.parse(localStorage.getItem(VIEW_STORAGE_KEY)!);
    delete saved.context.referenceFocus;
    expect(restoreViewState(JSON.stringify(saved))?.referenceFocus).toBeNull();
    for (const referenceFocus of ['unknown', 'anomaly', 'ceo-detail']) {
      const context = { ...saved.context, focusStep: 3, referenceFocus };
      expect(restoreViewState(JSON.stringify({ schemaVersion: 1, context }))?.referenceFocus).toBeNull();
    }
  });
});
