import { computed, ref, watch } from 'vue';
import { defineStore } from 'pinia';
import {
  advance,
  createShopWorkflows,
  decide,
  publish,
  receiveFeedback,
  restoreWorkflows,
  revise,
  simulateVerificationFailure,
  STORAGE_KEY,
  verify,
} from '../../features/ecommerce-loop/workflow';
import { restoreViewState, VIEW_STORAGE_KEY } from '../../features/ecommerce-loop/viewState';
import type {
  CaseId,
  DemoWorkflow,
  ReferenceFocus,
  SceneId,
  ShopId,
  StepId,
  ViewMode,
} from '../../features/ecommerce-loop/types';
import { referenceFocuses } from '../../features/ecommerce-loop/referenceCamera';

export const useEcommerceLoopStore = defineStore('ecommerce-loop', () => {
  const scene = ref<SceneId>(1);
  const focusStep = ref<StepId | null>(null);
  const referenceFocus = ref<ReferenceFocus | null>(null);
  const companyFocused = ref(false);
  const selectedShop = ref<ShopId>('a');
  const selectedProduct = ref('sea-salt');
  const viewMode = ref<ViewMode>('workbench');
  const activeCase = ref<CaseId>('traffic');
  const shops = ref(createShopWorkflows());
  const workflowWarning = ref('');
  const viewWarning = ref('');
  const storageWarning = computed(() => workflowWarning.value || viewWarning.value);
  const currentCases = computed(() => shops.value[selectedShop.value]);
  let restoring = false;

  watch(
    [viewMode, scene, focusStep, referenceFocus, companyFocused, selectedShop, selectedProduct, activeCase],
    () => {
      if (restoring) return;
      try {
        localStorage.setItem(
          VIEW_STORAGE_KEY,
          JSON.stringify({
            schemaVersion: 1,
            context: {
              viewMode: viewMode.value,
              scene: scene.value,
              focusStep: focusStep.value,
              referenceFocus: referenceFocus.value,
              companyFocused: companyFocused.value,
              selectedShop: selectedShop.value,
              selectedProduct: selectedProduct.value,
              activeCase: activeCase.value,
            },
          }),
        );
        viewWarning.value = '';
      } catch {
        viewWarning.value = 'unavailable';
      }
    },
    { flush: 'sync' },
  );

  function persist() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ schemaVersion: 1, shops: shops.value }));
      workflowWarning.value = '';
    } catch {
      workflowWarning.value = 'unavailable';
    }
  }

  function restore() {
    restoring = true;
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) {
        const restored = restoreWorkflows(raw);
        if (restored) shops.value = restored;
        else workflowWarning.value = 'invalid';
      }
    } catch {
      workflowWarning.value = 'unavailable';
    }
    try {
      const raw = localStorage.getItem(VIEW_STORAGE_KEY);
      if (raw) {
        const context = restoreViewState(raw);
        if (context) {
          viewMode.value = context.viewMode;
          scene.value = context.scene;
          focusStep.value = context.focusStep;
          referenceFocus.value = context.referenceFocus;
          companyFocused.value = context.companyFocused;
          selectedShop.value = context.selectedShop;
          selectedProduct.value = context.selectedProduct;
          activeCase.value = context.activeCase;
        } else viewWarning.value = 'invalid';
      }
    } catch {
      viewWarning.value = 'unavailable';
    } finally {
      restoring = false;
    }
  }

  function setScene(next: SceneId) {
    scene.value = next;
    focusStep.value = null;
    referenceFocus.value = next === 3 ? 'funnel-overview' : null;
    companyFocused.value = false;
    if (next === 4) activeCase.value = 'launch';
    else if (next === 3) activeCase.value = 'traffic';
  }

  function selectShop(shop: ShopId) {
    selectedShop.value = shop;
  }

  function focus(step: StepId | null) {
    focusStep.value = focusStep.value === step ? null : step;
    referenceFocus.value = null;
  }

  function focusReference(next: ReferenceFocus) {
    if (!referenceFocuses(scene.value).includes(next)) return;
    focusStep.value = null;
    referenceFocus.value = referenceFocus.value === next ? null : next;
  }

  function apply(kind: CaseId, action: (flow: DemoWorkflow) => boolean): boolean {
    const changed = action(currentCases.value[kind]);
    if (changed) persist();
    return changed;
  }

  function reset() {
    shops.value = createShopWorkflows();
    focusStep.value = null;
    referenceFocus.value = null;
    persist();
  }

  return {
    scene,
    focusStep,
    referenceFocus,
    companyFocused,
    selectedShop,
    selectedProduct,
    viewMode,
    activeCase,
    currentCases,
    shops,
    storageWarning,
    restore,
    setScene,
    selectShop,
    focus,
    focusReference,
    reset,
    decide: (kind: CaseId, decision: 'approved' | 'rejected', version: number) =>
      apply(kind, (flow) => decide(flow, decision, version)),
    revise: (kind: CaseId, note: string, version: number) => apply(kind, (flow) => revise(flow, note, version)),
    advance: (kind: CaseId) => apply(kind, advance),
    publish: (kind: CaseId, version: number) => apply(kind, (flow) => publish(flow, version)),
    verify: (kind: CaseId) => apply(kind, verify),
    simulateVerificationFailure: (kind: CaseId) => apply(kind, simulateVerificationFailure),
    feedback: (kind: CaseId) => apply(kind, receiveFeedback),
  };
});
