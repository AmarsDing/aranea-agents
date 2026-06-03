/** Shared UI helpers for learning loop components. */

export function formatDate(iso: string): string {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}

export function formatConfidence(v: number): string {
  if (v === 0) return '—';
  return (v * 100).toFixed(1) + '%';
}

export function patternKindColor(kind: string): string {
  switch (kind) {
    case 'tool_usage':
      return 'blue';
    case 'error':
      return 'red';
    case 'behavior':
      return 'teal';
    case 'preference':
      return 'purple';
    default:
      return 'grey';
  }
}

export function patternStatusColor(status: string): string {
  switch (status) {
    case 'active':
      return 'positive';
    case 'archived':
      return 'grey';
    case 'superseded':
      return 'warning';
    default:
      return 'grey';
  }
}

export function patternStatusLabel(status: string): string {
  switch (status) {
    case 'active':
      return '活跃';
    case 'archived':
      return '已归档';
    case 'superseded':
      return '已替代';
    default:
      return status;
  }
}

export function proposalKindColor(kind: string): string {
  switch (kind) {
    case 'tool_optimization':
      return 'blue';
    case 'prompt_refinement':
      return 'purple';
    case 'skill_creation':
      return 'teal';
    case 'behavior_adjustment':
      return 'orange';
    default:
      return 'grey';
  }
}

export function proposalStatusColor(status: string): string {
  switch (status) {
    case 'approved':
      return 'positive';
    case 'rejected':
      return 'negative';
    case 'pending':
      return 'warning';
    default:
      return 'grey';
  }
}

export function proposalStatusLabel(status: string): string {
  switch (status) {
    case 'approved':
      return '已审批';
    case 'rejected':
      return '已拒绝';
    case 'pending':
      return '待审批';
    default:
      return status;
  }
}
