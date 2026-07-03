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
    const positions = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    expect(positions.get('a')?.y).toBe(0);
    expect(positions.get('b')?.y).toBeGreaterThan(0);
    expect(positions.get('c')?.y).toBeGreaterThan(positions.get('b')!.y);
  });

  it('lays out parallel steps side by side', () => {
    const { layoutDAG } = usePlanDAGLayout();
    const steps = [mkStep('a'), mkStep('b', ['a']), mkStep('c', ['a']), mkStep('d', ['b', 'c'])];
    const positions = layoutDAG(steps, { width: 600, nodeWidth: 120, nodeHeight: 60, gapX: 40, gapY: 30 });
    // b and c should be at the same y level
    expect(positions.get('b')?.y).toBe(positions.get('c')?.y);
    // b and c should be at different x positions
    expect(positions.get('b')?.x).not.toBe(positions.get('c')?.x);
  });
});
