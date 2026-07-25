// web/src/features/chat/composables/__tests__/usePlanDAGLayout.spec.ts
import { describe, it, expect } from 'vitest';
import { usePlanDAGLayout } from '../usePlanDAGLayout';
import type { PlanStep } from '../../v2Types';

function mkStep(id: string, deps: string[] = [], over: Partial<PlanStep> = {}): PlanStep {
  return {
    ID: id,
    PlanID: 'pb1',
    TaskID: 'tk1',
    Label: id,
    Description: '',
    DependsOn: deps,
    MappedTeamStageID: '',
    Status: 'pending',
    AutoSynthesis: false,
    StartedAt: '',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Result: null,
    Error: null,
    ...over,
  };
}

describe('usePlanDAGLayout', () => {
  it('lays out sequential steps in a single column', () => {
    const { layoutDAG } = usePlanDAGLayout();
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['b'])];
    const { positions } = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    expect(positions.get('a')?.y).toBe(0);
    expect(positions.get('b')?.y).toBeGreaterThan(0);
    expect(positions.get('c')?.y).toBeGreaterThan(positions.get('b')!.y);
  });

  it('lays out parallel steps side by side', () => {
    const { layoutDAG } = usePlanDAGLayout();
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
    const { positions } = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    // b and c should be at the same y level
    expect(positions.get('b')?.y).toBe(positions.get('c')?.y);
    // b and c should be at different x positions
    expect(positions.get('b')?.x).not.toBe(positions.get('c')?.x);
  });

  it('computes dynamic width based on actual content (linear chain narrows)', () => {
    const { layoutDAG } = usePlanDAGLayout();
    // Linear chain: 3 nodes, 1 per layer → width should be nodeWidth + 2*padX (default 20)
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['b'])];
    const { computedWidth } = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    // 120 (nodeWidth) + 2*20 (padX) = 160
    expect(computedWidth).toBe(160);
  });

  it('computes dynamic width for parallel layers (widest layer wins)', () => {
    const { layoutDAG } = usePlanDAGLayout();
    // 2 parallel nodes in layer 1: width = 2*120 + 40 (gap) + 2*20 (padX) = 320
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
    const { computedWidth } = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    expect(computedWidth).toBe(320);
  });

  describe('horizontal orientation', () => {
    const H_OPTS = { width: 640, nodeWidth: 156, nodeHeight: 56, gapX: 64, gapY: 16, orientation: 'horizontal' as const, padX: 20, padY: 12 };

    it('lays out sequential steps in a single row (same y, increasing x)', () => {
      const { layoutDAG } = usePlanDAGLayout();
      const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['b'])];
      const { positions } = layoutDAG(steps, H_OPTS);
      expect(positions.get('a')?.y).toBe(12);
      expect(positions.get('b')?.y).toBe(12);
      expect(positions.get('c')?.y).toBe(12);
      expect(positions.get('a')?.x).toBe(20);
      expect(positions.get('b')!.x).toBeGreaterThan(positions.get('a')!.x);
      expect(positions.get('c')!.x).toBeGreaterThan(positions.get('b')!.x);
    });

    it('lays out parallel steps in the same column, vertically centered', () => {
      const { layoutDAG } = usePlanDAGLayout();
      const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
      const { positions } = layoutDAG(steps, H_OPTS);
      // b and c same column (same x), stacked vertically
      expect(positions.get('b')?.x).toBe(positions.get('c')?.x);
      expect(positions.get('b')?.y).toBe(12);
      expect(positions.get('c')?.y).toBe(12 + 56 + 16);
      // single-node columns centered against tallest column (2*56+16=128): y = 12 + (128-56)/2 = 48
      expect(positions.get('a')?.y).toBe(48);
      expect(positions.get('d')?.y).toBe(48);
    });

    it('computes structural width/height from layer count and tallest column', () => {
      const { layoutDAG } = usePlanDAGLayout();
      const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
      const { computedWidth, computedHeight } = layoutDAG(steps, H_OPTS);
      // width = 2*padX + 3 layers*156 + 2 gaps*64 = 40 + 468 + 128 = 636
      expect(computedWidth).toBe(636);
      // height = 2*padY + tallest column (2*56+16) = 24 + 128 = 152
      expect(computedHeight).toBe(152);
    });
  });
});
