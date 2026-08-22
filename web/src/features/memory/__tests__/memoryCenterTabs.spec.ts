import { describe, expect, it } from 'vitest';
import { resolveMemoryCenterTab } from '../memoryCenterTabs';

describe('resolveMemoryCenterTab', () => {
  it('keeps ops for platform admins', () => {
    expect(resolveMemoryCenterTab('ops', true)).toBe('ops');
  });

  it('falls back ops to trust for non-admins', () => {
    expect(resolveMemoryCenterTab('ops', false)).toBe('governance');
  });

  it('preserves existing trust and browse tabs', () => {
    expect(resolveMemoryCenterTab('governance', false)).toBe('governance');
    expect(resolveMemoryCenterTab('browse', true)).toBe('browse');
  });

  it('falls unknown tabs back to panorama', () => {
    expect(resolveMemoryCenterTab('unknown', true)).toBe('panorama');
    expect(resolveMemoryCenterTab('', false)).toBe('panorama');
  });
});
