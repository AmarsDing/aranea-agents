import { describe, expect, it } from 'vitest';
import { parseA2UIJsonl } from '../a2uiParse';
import { reduceA2UISurface } from '../a2uiSurfaceState';

describe('reduceA2UISurface', () => {
  it('builds component tree from beginRendering and surfaceUpdate', () => {
    const raw = [
      '{"beginRendering":{"surfaceId":"s1","root":"root"}}',
      '{"surfaceUpdate":{"surfaceId":"s1","components":[{"id":"root","component":{"Column":{"children":{"explicitList":["t1"]}}}},{"id":"t1","component":{"Text":{"text":{"literalString":"Hello"}}}}]}}',
    ].join('\n');
    const lines = parseA2UIJsonl(raw);
    const surface = reduceA2UISurface(lines);
    expect(surface.ready).toBe(true);
    expect(surface.surfaceId).toBe('s1');
    expect(surface.rootId).toBe('root');
    expect(surface.components.t1?.component.Text).toBeTruthy();
  });

  it('merges dataModelUpdate into dataModel', () => {
    const raw = [
      '{"beginRendering":{"surfaceId":"s1","root":"btn"}}',
      '{"dataModelUpdate":{"surfaceId":"s1","path":"/","contents":[{"key":"label","valueString":"Go"}]}}',
      '{"surfaceUpdate":{"surfaceId":"s1","components":[{"id":"btn","component":{"Button":{"action":{"name":"submit"},"child":"lbl"}}},{"id":"lbl","component":{"Text":{"text":{"path":"/label"}}}}]}}',
    ].join('\n');
    const surface = reduceA2UISurface(parseA2UIJsonl(raw));
    expect(surface.dataModel.label).toBe('Go');
    expect(surface.components.btn).toBeTruthy();
  });
});
