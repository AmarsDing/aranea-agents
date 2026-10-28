import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useEcommerceLoopStore } from '../../stores/ecommerce-loop';
import { SHOPS } from './data';
import { cameraTransform, referenceFocuses, referenceTarget } from './referenceCamera';
import type { CaseId, ReferenceFocus, SceneId, ShopId, StepId, ViewMode, WorkflowActionPayload } from './types';

export function useEcommerceLoopPage() {
  const store = useEcommerceLoopStore();
  const {
    scene,
    focusStep,
    referenceFocus,
    companyFocused,
    selectedShop,
    selectedProduct,
    currentCases,
    storageWarning,
    viewMode,
    activeCase,
  } = storeToRefs(store);
  const { t } = useI18n();
  const $q = useQuasar();
  const viewport = ref<HTMLElement | null>(null);
  const root = ref<HTMLElement | null>(null);
  const scale = ref(0.5);
  const isFullscreen = ref(false);
  const drawerOpen = ref(false);
  const editing = ref(false);
  const dirty = ref(false);
  const selectedWorkflow = computed(() => currentCases.value[activeCase.value]);
  const shopName = computed(() => t('ecommerceLoop.shop', { name: selectedShop.value.toUpperCase() }));
  const referenceOptions = computed(() => referenceFocuses(scene.value));
  const measuredCamera = ref('none');
  const cameraStyle = computed(() => {
    if (scene.value === 1 && companyFocused.value)
      return { transform: 'translate(664px, 191px) scale(1.68) translate(-351.5px, -522px)' };
    return { transform: measuredCamera.value };
  });
  let observer: ResizeObserver | undefined;
  let drawerTrigger: HTMLElement | null = null;

  function changeContext(change: () => void) {
    if (!dirty.value) {
      change();
      return;
    }
    $q.dialog({
      title: t('ecommerceLoop.workflow.dirtyTitle'),
      message: t('ecommerceLoop.workflow.dirtyMessage'),
      cancel: { label: t('ecommerceLoop.workbench.keepEditing') },
      ok: { label: t('ecommerceLoop.workbench.discard') },
      persistent: true,
    }).onOk(() => {
      dirty.value = false;
      editing.value = false;
      change();
    });
  }

  function setScene(next: SceneId) {
    changeContext(() => store.setScene(next));
  }
  function selectShop(shop: ShopId) {
    if (shop === selectedShop.value) return;
    changeContext(() => {
      store.selectShop(shop);
    });
  }
  function onEnterShop(shop: ShopId) {
    changeContext(() => {
      store.selectShop(shop);
      store.setScene(3);
    });
  }
  function onViewChange(mode: ViewMode) {
    if (viewMode.value === mode) return;
    changeContext(() => {
      drawerOpen.value = false;
      editing.value = false;
      viewMode.value = mode;
    });
  }
  function onCaseChange(kind: CaseId) {
    if (kind === activeCase.value) return;
    changeContext(() => {
      editing.value = false;
      activeCase.value = kind;
    });
  }
  async function focus(step: StepId) {
    store.focus(step);
    await nextTick();
    root.value?.querySelector<HTMLElement>(`[data-reference-step="${step}"]`)?.focus({ preventScroll: true });
  }
  async function onReferenceFocus(next: ReferenceFocus) {
    store.focusReference(next);
    await nextTick();
    root.value?.querySelector<HTMLElement>(`[data-reference-focus="${next}"]`)?.focus({ preventScroll: true });
  }
  async function onFocusTask() {
    await nextTick();
    const heading = root.value?.querySelector<HTMLElement>('[data-task-heading]');
    heading?.scrollIntoView({ block: 'start', behavior: 'auto' });
    heading?.focus({ preventScroll: true });
  }

  function resize() {
    if (!viewport.value) return;
    const rect = viewport.value.getBoundingClientRect();
    scale.value = Math.max(0.1, Math.min(rect.width / 1920, rect.height / 1080));
  }
  function onNavigate(delta: number) {
    setScene(Math.min(5, Math.max(1, scene.value + delta)) as SceneId);
  }
  function onKeydown(event: KeyboardEvent) {
    const element = event.target;
    if (
      viewMode.value !== 'reference' ||
      drawerOpen.value ||
      event.isComposing ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      (element instanceof HTMLElement &&
        (element.isContentEditable ||
          element.closest('[role="dialog"], [role="menu"], .q-field') ||
          ['INPUT', 'TEXTAREA', 'SELECT'].includes(element.tagName)))
    )
      return;
    if (event.key !== 'Escape' && element instanceof HTMLElement && element.closest('button, a')) return;
    if (['ArrowRight', 'PageDown', 'ArrowLeft', 'PageUp'].includes(event.key)) {
      event.preventDefault();
      onNavigate(['ArrowRight', 'PageDown'].includes(event.key) ? 1 : -1);
    } else if (event.key === 'Escape') onOverview();
  }
  function onOverview() {
    store.focus(null);
    companyFocused.value = false;
    void nextTick(() =>
      root.value?.querySelector<HTMLElement>('[data-overview-control]')?.focus({ preventScroll: true }),
    );
  }
  function onOpenWorkflow(kind: CaseId = scene.value === 4 ? 'launch' : 'traffic') {
    changeContext(() => {
      const active = document.activeElement;
      drawerTrigger =
        active instanceof HTMLElement && active.matches('button, a, [role="button"]') && root.value?.contains(active)
          ? active
          : (root.value?.querySelector<HTMLElement>('.ec-controls-actions button') ?? null);
      activeCase.value = kind;
      editing.value = false;
      drawerOpen.value = true;
    });
  }
  function onCloseDrawer(dirty: boolean) {
    if (dirty) onDirty(true);
    changeContext(() => {
      drawerOpen.value = false;
      editing.value = false;
    });
  }
  function onDirty(value: boolean) {
    dirty.value = value;
  }
  function onDrawerHidden() {
    if (viewMode.value === 'reference' && drawerTrigger?.isConnected) drawerTrigger.focus({ preventScroll: true });
  }
  function onWorkflowAction(payload: WorkflowActionPayload) {
    const kind = activeCase.value;
    if (payload.version !== selectedWorkflow.value.version) {
      $q.notify({ type: 'warning', message: t('ecommerceLoop.stale') });
      return;
    }
    if (payload.action === 'cancel-edit') {
      changeContext(() => {
        editing.value = false;
      });
      return;
    }
    if (payload.action === 'revise' && !editing.value) {
      editing.value = true;
      return;
    }
    let changed = false;
    if (payload.action === 'approved' || payload.action === 'rejected')
      changed = store.decide(kind, payload.action, payload.version);
    else if (payload.action === 'revise') changed = store.revise(kind, payload.note ?? '', payload.version);
    else if (payload.action === 'publish') changed = store.publish(kind, payload.version);
    else if (payload.action === 'advance') changed = store.advance(kind);
    else if (payload.action === 'verify') changed = store.verify(kind);
    else if (payload.action === 'simulate-verification-failure') changed = store.simulateVerificationFailure(kind);
    else if (payload.action === 'feedback') changed = store.feedback(kind);
    if (changed) {
      dirty.value = false;
      editing.value = false;
    } else $q.notify({ type: 'warning', message: t('ecommerceLoop.stale') });
  }
  function onTrafficDecision(decision: 'approved' | 'rejected' | 'revise') {
    onOpenWorkflow('traffic');
    if (decision === 'revise') editing.value = true;
    else store.decide('traffic', decision, currentCases.value.traffic.version);
  }
  function onReset() {
    changeContext(() => {
      $q.dialog({
        title: t('ecommerceLoop.resetTitle'),
        message: t('ecommerceLoop.resetMessage'),
        cancel: { label: t('ecommerceLoop.cancel') },
        ok: { label: t('ecommerceLoop.confirm') },
        persistent: true,
      }).onOk(() => {
        editing.value = false;
        dirty.value = false;
        store.reset();
      });
    });
  }
  async function onFullscreen() {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await root.value?.requestFullscreen();
    } catch (error) {
      console.warn('Commerce reference fullscreen unavailable', error);
    }
  }
  function onFullscreenChange() {
    isFullscreen.value = !!document.fullscreenElement;
    resize();
  }
  function onShopChange(event: Event) {
    const select = event.target as HTMLSelectElement;
    const value = select.value;
    select.value = selectedShop.value;
    if (SHOPS.includes(value as ShopId)) selectShop(value as ShopId);
  }
  watch(viewport, (element) => {
    observer?.disconnect();
    if (element) observer?.observe(element);
    resize();
  });
  watch(
    [scene, focusStep, referenceFocus, viewMode, viewport],
    async (_, __, onCleanup) => {
      let stale = false;
      onCleanup(() => {
        stale = true;
      });
      await nextTick();
      if (stale) return;
      const target = referenceTarget(scene.value, focusStep.value, referenceFocus.value);
      const stage = root.value?.querySelector<HTMLElement>('.ec-stage');
      const camera = root.value?.querySelector<HTMLElement>('.ec-camera');
      const element = target && camera?.querySelector<HTMLElement>(target.selector);
      if (!target || !stage || !camera || !element) {
        measuredCamera.value = 'none';
        return;
      }
      // Undo the live camera (including an in-flight transition) to read source coordinates.
      const outer = stage.getBoundingClientRect();
      const rect = element.getBoundingClientRect();
      const matrix = new DOMMatrix(getComputedStyle(camera).transform);
      const stageScale = outer.width / 1920;
      measuredCamera.value = cameraTransform(
        {
          x: ((rect.left - outer.left) / stageScale - matrix.e) / matrix.a,
          y: ((rect.top - outer.top) / stageScale - matrix.f) / matrix.d,
          width: rect.width / stageScale / matrix.a,
        },
        target,
      );
    },
    { flush: 'post' },
  );
  onMounted(() => {
    store.restore();
    observer = new ResizeObserver(resize);
    if (viewport.value) observer.observe(viewport.value);
    resize();
    window.addEventListener('keydown', onKeydown);
    document.addEventListener('fullscreenchange', onFullscreenChange);
  });
  onUnmounted(() => {
    observer?.disconnect();
    window.removeEventListener('keydown', onKeydown);
    document.removeEventListener('fullscreenchange', onFullscreenChange);
  });
  return {
    root,
    viewport,
    scale,
    viewMode,
    activeCase,
    isFullscreen,
    scene,
    focusStep,
    referenceFocus,
    referenceOptions,
    companyFocused,
    selectedShop,
    selectedProduct,
    currentCases,
    storageWarning,
    cameraStyle,
    drawerOpen,
    editing,
    selectedWorkflow,
    shopName,
    onNavigate,
    onOverview,
    onOpenWorkflow,
    onCloseDrawer,
    onWorkflowAction,
    onTrafficDecision,
    onReset,
    onFullscreen,
    onShopChange,
    onViewChange,
    onCaseChange,
    onDirty,
    onDrawerHidden,
    onEnterShop,
    onFocusTask,
    setScene,
    focus,
    onReferenceFocus,
    selectShop,
    onFocusCompany: () => {
      companyFocused.value = !companyFocused.value;
    },
    onSelectProduct: (id: string) => {
      selectedProduct.value = id;
    },
  };
}
