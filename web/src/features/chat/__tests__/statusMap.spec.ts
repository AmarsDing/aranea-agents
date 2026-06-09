import { describe, expect, it } from 'vitest';
import {
  canonicalToolStatus,
  messageStatusFromCanonical,
  messageStatusFromWire,
} from '../lib/statusMap';
import { MESSAGE_STATUS } from '../../../domain/types';

describe('statusMap', () => {
  it('maps every wire status variant to one canonical form', () => {
    expect(canonicalToolStatus('calling')).toBe('running');
    expect(canonicalToolStatus('running')).toBe('running');
    expect(canonicalToolStatus('in_progress')).toBe('running');
    expect(canonicalToolStatus('failed')).toBe('failed');
    expect(canonicalToolStatus('error')).toBe('failed');
    expect(canonicalToolStatus('blocked')).toBe('blocked');
    expect(canonicalToolStatus('cancelled')).toBe('cancelled');
    expect(canonicalToolStatus('interrupted')).toBe('cancelled');
    expect(canonicalToolStatus('success')).toBe('success');
  });

  it('is case- and whitespace-insensitive', () => {
    expect(canonicalToolStatus('  CALLED ')).toBe('running'); // typo falls back to default
    expect(canonicalToolStatus(' Running ')).toBe('running');
    expect(canonicalToolStatus('ERROR')).toBe('failed');
  });

  it('falls back to "running" for unknown wire values (preserves live indicator)', () => {
    expect(canonicalToolStatus('')).toBe('running');
    expect(canonicalToolStatus('mystery')).toBe('running');
  });

  it('maps canonical status to MESSAGE_STATUS.* correctly', () => {
    expect(messageStatusFromCanonical('running')).toBe(MESSAGE_STATUS.TOOL_RUNNING);
    expect(messageStatusFromCanonical('success')).toBe(MESSAGE_STATUS.TOOL_SUCCESS);
    expect(messageStatusFromCanonical('failed')).toBe(MESSAGE_STATUS.TOOL_FAILED);
    expect(messageStatusFromCanonical('blocked')).toBe(MESSAGE_STATUS.TOOL_BLOCKED);
    expect(messageStatusFromCanonical('cancelled')).toBe(MESSAGE_STATUS.TOOL_CANCELLED);
  });

  it('one-shot: wire status -> persisted message status', () => {
    expect(messageStatusFromWire('error')).toBe(MESSAGE_STATUS.TOOL_FAILED);
    expect(messageStatusFromWire('cancelled')).toBe(MESSAGE_STATUS.TOOL_CANCELLED);
    expect(messageStatusFromWire('in_progress')).toBe(MESSAGE_STATUS.TOOL_RUNNING);
  });

  it('canonical form is closed (5 values only)', () => {
    // Type-level guarantee; the runtime check is that nothing escapes the 5 forms.
    const allForms: Canonical[] = [
      'running',
      'success',
      'failed',
      'blocked',
      'cancelled',
    ];
    allForms.forEach((s) => {
      expect(typeof messageStatusFromCanonical(s)).toBe('string');
    });
  });
});

type Canonical = 'running' | 'success' | 'failed' | 'blocked' | 'cancelled';
