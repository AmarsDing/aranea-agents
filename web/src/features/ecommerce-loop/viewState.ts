import { SCENES, SHOPS, STEPS, PRODUCT_METRICS } from './data';
import type { DemoViewState } from './types';
import { referenceFocuses } from './referenceCamera';

export const VIEW_STORAGE_KEY = 'aranea:ecommerce-view:v1';

export function restoreViewState(raw: string): DemoViewState | null {
  try {
    const parsed = JSON.parse(raw) as { schemaVersion?: unknown; context?: Partial<DemoViewState> };
    const value = parsed?.context;
    if (
      parsed?.schemaVersion !== 1 ||
      !value ||
      !['workbench', 'reference'].includes(String(value.viewMode)) ||
      !SCENES.includes(value.scene!) ||
      !SHOPS.includes(value.selectedShop!) ||
      !['traffic', 'launch'].includes(String(value.activeCase)) ||
      !PRODUCT_METRICS.some((product) => product.id === value.selectedProduct) ||
      typeof value.companyFocused !== 'boolean' ||
      (value.focusStep !== null && !STEPS.includes(value.focusStep!))
    )
      return null;
    return {
      viewMode: value.viewMode!,
      scene: value.scene!,
      selectedShop: value.selectedShop!,
      selectedProduct: value.selectedProduct!,
      activeCase: value.activeCase!,
      companyFocused: value.scene === 1 && value.companyFocused,
      focusStep: [2, 4].includes(value.scene!) ? value.focusStep! : null,
      referenceFocus:
        !([2, 4].includes(value.scene!) && value.focusStep) &&
        referenceFocuses(value.scene!).includes(value.referenceFocus!)
          ? value.referenceFocus!
          : null,
    };
  } catch {
    return null;
  }
}
