// web/src/features/chat/__tests__/displayRegistry.spec.ts
// LBG-8 展示型通知注册表：noticeType → {parse, component} 分发与兜底语义。
import { defineComponent } from 'vue';
import { describe, expect, it } from 'vitest';
import { registerDisplayType, resolveDisplayPayload, resolveDisplayType } from '../displayRegistry';

const VALID_DELIVERABLES = JSON.stringify({ artifacts: [{ artifact_id: 'a1', name: 'r.md', format: 'markdown' }] });

describe('displayRegistry', () => {
  it('resolves the built-in deliverables registration', () => {
    const reg = resolveDisplayType('deliverables');
    expect(reg).toBeDefined();
    expect(reg?.noticeType).toBe('deliverables');
  });

  it('matches notice type case-insensitively and trims whitespace', () => {
    expect(resolveDisplayType(' Deliverables ')).toBeDefined();
    expect(resolveDisplayType('DELIVERABLES')).toBeDefined();
  });

  it('returns undefined for unregistered or empty notice types', () => {
    expect(resolveDisplayType('info')).toBeUndefined();
    expect(resolveDisplayType(undefined)).toBeUndefined();
    expect(resolveDisplayType('  ')).toBeUndefined();
  });

  it('resolveDisplayPayload returns component + parsed payload on hit', () => {
    const hit = resolveDisplayPayload('deliverables', VALID_DELIVERABLES);
    expect(hit).not.toBeNull();
    expect(hit?.component).toBe(resolveDisplayType('deliverables')?.component);
    expect(hit?.payload).toEqual([{ artifactId: 'a1', name: 'r.md', mimeType: 'text/plain' }]);
  });

  it('resolveDisplayPayload falls back to null on malformed payload (markdown fallback)', () => {
    expect(resolveDisplayPayload('deliverables', 'not-json')).toBeNull();
    expect(resolveDisplayPayload('deliverables', '{"artifacts":[]}')).toBeNull();
  });

  it('resolveDisplayPayload returns null for unregistered types', () => {
    expect(resolveDisplayPayload('info', VALID_DELIVERABLES)).toBeNull();
  });

  // ACC-LBG-8-01：新增展示类型只需一次注册，不碰分发主流程。
  it('accepts a demo display type via a single registration', () => {
    const DemoComponent = defineComponent({ template: '<div />' });
    registerDisplayType({
      noticeType: 'demo_card',
      parse: (content) => (content.includes('"ok":true') ? { ok: true } : null),
      component: DemoComponent,
    });
    const hit = resolveDisplayPayload('Demo_Card', '{"ok":true}');
    expect(hit?.component).toBe(DemoComponent);
    expect(hit?.payload).toEqual({ ok: true });
    expect(resolveDisplayPayload('demo_card', '{"ok":false}')).toBeNull();
  });

  it('later registration overrides the same noticeType', () => {
    const A = defineComponent({ template: '<div />' });
    const B = defineComponent({ template: '<div />' });
    registerDisplayType({ noticeType: 'override_me', parse: () => ({}), component: A });
    registerDisplayType({ noticeType: 'override_me', parse: () => ({}), component: B });
    expect(resolveDisplayType('override_me')?.component).toBe(B);
  });
});
