import { computed, type ComputedRef } from 'vue';
import type { PromptBreakdown, PromptPreviewReport } from '../contextBreakdown';
import { computeBreakdown } from '../contextBreakdown';
import type { ComposerUsageSnapshot } from '../composerUsageMetrics';

export function useContextBreakdown(deps: {
  usageSnapshot: ComputedRef<ComposerUsageSnapshot | null>;
  promptPreview: ComputedRef<PromptPreviewReport | null>;
  toolCallCount: ComputedRef<number>;
  messageCount: ComputedRef<number>;
}): ComputedRef<PromptBreakdown | null> {
  return computed(() => {
    const snap = deps.usageSnapshot.value;
    if (!snap) return null;

    return computeBreakdown(
      deps.promptPreview.value,
      snap.contextUsedTokens ?? 0,
      snap.contextWindow ?? 0,
      deps.toolCallCount.value,
      deps.messageCount.value,
      undefined,
    );
  });
}
