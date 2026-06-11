import { describe, expect, it } from 'vitest';
import {
  __resetTodoFingerprintCache,
  computeTodoFingerprint,
  readLastFingerprint,
  writeLastFingerprint,
} from '../composables/todoColumnFingerprint';
import type { TodoItem } from '../agentTreeTypes';

function makeItem(overrides: Partial<TodoItem> = {}): TodoItem {
  return {
    id: 't1',
    content: 'Task',
    activeForm: '',
    status: 'pending',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('todoColumnFingerprint', () => {
  it('returns an empty string for an empty list', () => {
    expect(computeTodoFingerprint([])).toBe('');
    expect(computeTodoFingerprint(null as unknown as readonly TodoItem[])).toBe('');
  });

  it('produces a stable string for the same id:status pairs', () => {
    const a = [makeItem({ id: '1', status: 'pending' }), makeItem({ id: '2', status: 'completed' })];
    const b = [makeItem({ id: '1', status: 'pending' }), makeItem({ id: '2', status: 'completed' })];
    expect(computeTodoFingerprint(a)).toBe(computeTodoFingerprint(b));
  });

  it('differs when an item status changes (length unchanged)', () => {
    const before = [makeItem({ id: '1', status: 'pending' })];
    const after = [makeItem({ id: '1', status: 'completed' })];
    expect(computeTodoFingerprint(before)).not.toBe(computeTodoFingerprint(after));
  });

  it('differs when an item id changes', () => {
    const before = [makeItem({ id: '1', status: 'pending' })];
    const after = [makeItem({ id: '2', status: 'pending' })];
    expect(computeTodoFingerprint(before)).not.toBe(computeTodoFingerprint(after));
  });

  it('ignores content changes (status-only detection)', () => {
    const before = [makeItem({ id: '1', status: 'pending', content: 'A' })];
    const after = [makeItem({ id: '1', status: 'pending', content: 'B' })];
    // The fingerprint intentionally excludes content — content edits
    // are reflected in TodoCard, not in the column pulse animation.
    expect(computeTodoFingerprint(before)).toBe(computeTodoFingerprint(after));
  });

  it('preserves order (reordering is a different fingerprint)', () => {
    const a = [makeItem({ id: '1' }), makeItem({ id: '2' })];
    const b = [makeItem({ id: '2' }), makeItem({ id: '1' })];
    expect(computeTodoFingerprint(a)).not.toBe(computeTodoFingerprint(b));
  });

  describe('cross-instance cache', () => {
    it('stores and reads the last fingerprint for a column key', () => {
      __resetTodoFingerprintCache();
      const fp = '1:pending|2:completed';
      expect(readLastFingerprint('pending')).toBeUndefined();
      writeLastFingerprint('pending', fp);
      expect(readLastFingerprint('pending')).toBe(fp);
    });

    it('isolates fingerprints across column keys', () => {
      __resetTodoFingerprintCache();
      writeLastFingerprint('pending', 'a');
      writeLastFingerprint('in_progress', 'b');
      expect(readLastFingerprint('pending')).toBe('a');
      expect(readLastFingerprint('in_progress')).toBe('b');
    });

    it('clears the slot when the stored value is empty', () => {
      __resetTodoFingerprintCache();
      writeLastFingerprint('pending', 'a');
      writeLastFingerprint('pending', '');
      expect(readLastFingerprint('pending')).toBeUndefined();
    });

    it('reset clears the cache', () => {
      __resetTodoFingerprintCache();
      writeLastFingerprint('pending', 'a');
      __resetTodoFingerprintCache();
      expect(readLastFingerprint('pending')).toBeUndefined();
    });
  });
});
