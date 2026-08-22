import { describe, expect, it } from 'vitest';
import { formatStaffingCaption } from '../graphTeamNodeUi';

describe('formatStaffingCaption', () => {
  it('joins specialty and assigned person', () => {
    expect(
      formatStaffingCaption({ DomainPath: '创作/文案', AssignedName: '文案专项', MatchLayer: 'roster' }),
    ).toBe('创作/文案 · 文案专项 · roster');
  });

  it('returns empty when staffing is missing', () => {
    expect(formatStaffingCaption(undefined)).toBe('');
    expect(formatStaffingCaption({})).toBe('');
  });
});
