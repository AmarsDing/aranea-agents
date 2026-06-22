import { describe, expect, it } from 'vitest';
import { isToolUseEvent } from '../lib/isToolUseEvent';

describe('isToolUseEvent', () => {
  it('accepts a well-formed ToolUseEvent', () => {
    expect(
      isToolUseEvent({
        tool_name: 'read_file',
        tool_label: '读取文件',
        phase: 'after',
        status: 'success',
      }),
    ).toBe(true);
  });

  it('rejects null / undefined / primitives', () => {
    expect(isToolUseEvent(null)).toBe(false);
    expect(isToolUseEvent(undefined)).toBe(false);
    expect(isToolUseEvent('tool')).toBe(false);
    expect(isToolUseEvent(42)).toBe(false);
  });

  it('rejects objects missing the required shape (regression: avoid silent cast)', () => {
    // Only one of the four required fields is present — must be rejected, not
    // cast through to undefined-reads.
    expect(isToolUseEvent({ tool_name: 'x' })).toBe(false);
    expect(isToolUseEvent({ tool_name: 'x', tool_label: 'y' })).toBe(false);
    expect(isToolUseEvent({ tool_name: 'x', tool_label: 'y', phase: 'p' })).toBe(false);
  });

  it('rejects wrong types (status being a number, etc.)', () => {
    expect(isToolUseEvent({ tool_name: 'x', tool_label: 'y', phase: 'p', status: 1 })).toBe(false);
  });
});
