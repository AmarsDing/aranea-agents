export type TimelinePhaseType = 'planning' | 'allocation' | 'orchestration' | 'delivery';

export type TimelineStepStatus = 'running' | 'completed' | 'failed' | 'skipped';

export interface TimelineStep {
  name: string;
  startedAt: number;
  durationMs: number;
  status: TimelineStepStatus;
  metadata?: Record<string, unknown>;
}

export interface TimelinePhase {
  phase: TimelinePhaseType;
  startedAt: number;
  durationMs: number;
  steps: TimelineStep[];
  result?: string;
  status: TimelineStepStatus;
}

export interface OrchestrationTimelineData {
  phases: TimelinePhase[];
  totalDurationMs: number;
  runId?: string;
}
