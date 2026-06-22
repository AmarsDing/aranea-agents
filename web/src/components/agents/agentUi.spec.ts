import { describe, it, expect } from 'vitest';
import { deriveMemoryToolMode } from './agentUi';

describe('deriveMemoryToolMode', () => {
  it('returns "both" for empty/undefined deny list', () => {
    expect(deriveMemoryToolMode()).toBe('both');
    expect(deriveMemoryToolMode('')).toBe('both');
    expect(deriveMemoryToolMode('[]')).toBe('both');
  });

  it('returns "working_memory" when all framework memory tools are denied', () => {
    const deny = JSON.stringify(['memory_add', 'memory_update', 'memory_delete', 'memory_search', 'memory_load']);
    expect(deriveMemoryToolMode(deny)).toBe('working_memory');
  });

  it('returns "working_memory" even with extra denied tools', () => {
    const deny = JSON.stringify([
      'memory_add',
      'memory_update',
      'memory_delete',
      'memory_search',
      'memory_load',
      'some_other_tool',
    ]);
    expect(deriveMemoryToolMode(deny)).toBe('working_memory');
  });

  it('returns "framework_memory" when all working memory tools are denied', () => {
    const deny = JSON.stringify([
      'working_memory_read',
      'working_memory_list',
      'working_memory_write',
      'working_memory_patch',
      'working_memory_delete',
    ]);
    expect(deriveMemoryToolMode(deny)).toBe('framework_memory');
  });

  it('returns "both" when only some framework memory tools are denied', () => {
    const deny = JSON.stringify(['memory_add', 'memory_update']);
    expect(deriveMemoryToolMode(deny)).toBe('both');
  });

  it('returns "both" for invalid JSON', () => {
    expect(deriveMemoryToolMode('not json')).toBe('both');
  });
});
