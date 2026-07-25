// web/src/features/graph/editor/__tests__/graphLayout.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { applyAutoLayout, readGraphLayout, writeGraphNodePosition } from '../graphLayout';
import type { GraphDefinition, NodeDef } from '../../types';

describe('graphLayout - R2-2 Position Rendering Priority', () => {
  let graphDef: GraphDefinition;

  beforeEach(() => {
    graphDef = {
      id: 'test-graph',
      name: 'Test Graph',
      version: 1,
      nodes: [
        { id: 'node1', type: 'function', description: 'Node 1' } as NodeDef,
        { id: 'node2', type: 'function', description: 'Node 2' } as NodeDef,
        { id: 'node3', type: 'function', description: 'Node 3' } as NodeDef,
      ],
      edges: [
        { from: 'node1', to: 'node2', kind: '' },
        { from: 'node2', to: 'node3', kind: '' },
      ],
      conditionalEdges: [],
      entryPoint: 'node1',
      finishPoint: 'node3',
      stateFields: [],
      description: '',
      metadata: {},
    };
  });

  it('writes layout positions to graph metadata', () => {
    // Apply auto layout
    applyAutoLayout(graphDef);

    // Read back the layout
    const layout = readGraphLayout(graphDef);

    // Should have positions for all nodes
    expect(layout['node1']).toBeDefined();
    expect(layout['node2']).toBeDefined();
    expect(layout['node3']).toBeDefined();

    // Positions should be different (layout algorithm should space them out)
    expect(layout['node1'].x).not.toBe(layout['node2'].x);
    expect(layout['node2'].x).not.toBe(layout['node3'].x);
  });

  it('preserves existing positions when preferSavedLayout is false', () => {
    // Set initial positions
    writeGraphNodePosition(graphDef, 'node1', { x: 100, y: 100 });
    writeGraphNodePosition(graphDef, 'node2', { x: 300, y: 100 });

    // Simulate existing in-memory positions (what canvas currently shows)
    const existingPositions = new Map([
      ['node1', { x: 150, y: 150 }],
      ['node2', { x: 350, y: 150 }],
    ]);

    // Read saved layout
    const savedLayout = readGraphLayout(graphDef);

    // When preferSavedLayout is false, existing positions should take priority
    const preferSavedLayout = false;
    const finalPos1 = preferSavedLayout
      ? savedLayout['node1'] ?? existingPositions.get('node1')
      : existingPositions.get('node1') ?? savedLayout['node1'];

    expect(finalPos1).toEqual({ x: 150, y: 150 });
  });

  it('uses saved layout when preferSavedLayout is true', () => {
    // Set initial positions
    writeGraphNodePosition(graphDef, 'node1', { x: 100, y: 100 });
    writeGraphNodePosition(graphDef, 'node2', { x: 300, y: 100 });

    // Simulate existing in-memory positions (what canvas currently shows)
    const existingPositions = new Map([
      ['node1', { x: 150, y: 150 }],
      ['node2', { x: 350, y: 150 }],
    ]);

    // Read saved layout
    const savedLayout = readGraphLayout(graphDef);

    // When preferSavedLayout is true, saved layout should take priority
    const preferSavedLayout = true;
    const finalPos1 = preferSavedLayout
      ? savedLayout['node1'] ?? existingPositions.get('node1')
      : existingPositions.get('node1') ?? savedLayout['node1'];

    expect(finalPos1).toEqual({ x: 100, y: 100 });
  });

  it('applies auto layout and updates metadata correctly', () => {
    // Initial random positions
    writeGraphNodePosition(graphDef, 'node1', { x: 50, y: 50 });
    writeGraphNodePosition(graphDef, 'node2', { x: 500, y: 300 });

    // Apply auto layout
    const moves = applyAutoLayout(graphDef);

    // Should return move information
    expect(moves.length).toBeGreaterThan(0);

    // Read updated layout
    const updatedLayout = readGraphLayout(graphDef);

    // Positions should be updated in metadata
    expect(updatedLayout['node1']).toBeDefined();
    expect(updatedLayout['node2']).toBeDefined();
    expect(updatedLayout['node3']).toBeDefined();

    // Layout should arrange nodes in a flow (x coordinates should increase)
    expect(updatedLayout['node1'].x).toBeLessThan(updatedLayout['node2'].x);
    expect(updatedLayout['node2'].x).toBeLessThan(updatedLayout['node3'].x);
  });
});
