import { describe, expect, it } from 'vitest';
import { resolveA2UIChildIds } from '../a2uiChildren';
import type { A2UISurfaceState } from '../a2uiSurfaceState';

function surface(overrides: Partial<A2UISurfaceState> = {}): A2UISurfaceState {
  return {
    surfaceId: 's1',
    rootId: 'root',
    components: {
      a: { id: 'a', component: { Text: { text: { literalString: 'A' } } } },
      b: { id: 'b', component: { Text: { text: { literalString: 'B' } } } },
      tpl: { id: 'tpl', component: { Text: { text: { literalString: 'T' } } } },
    },
    dataModel: { items: { x: 'tpl', y: 'b' } },
    ready: true,
    deleted: false,
    ...overrides,
  };
}

describe('resolveA2UIChildIds', () => {
  it('resolves explicitList', () => {
    const ids = resolveA2UIChildIds({ explicitList: ['a', 'b', 'missing'] }, surface());
    expect(ids).toEqual(['a', 'b']);
  });

  it('resolves template dataBinding map', () => {
    const ids = resolveA2UIChildIds({ template: { componentId: 'tpl', dataBinding: '/items' } }, surface());
    expect(ids).toEqual(['tpl', 'b']);
  });
});
