import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import {
  listSpiritTeams,
  cancelSpiritTeam,
  resumeSpiritTeam,
  archiveSpiritTeam,
  retrySpiritTeam,
} from '../../features/spirit/api';
import type {
  SpiritTeam,
  SpiritPanelMode,
  SpiritTeamMode,
  SpiritTeamStatus,
  TeamProgressView,
  SynthesisOutput,
  DQScoreBreakdown,
  EvolutionSuggestion,
  CompletionStats,
} from '../../features/spirit/types';
import { isValidTeamStatus } from '../../features/spirit/types';
import type { ActivityEvent } from '../../realtime/activityEvent';

// --- Spirit Orchestration Event Payloads (inlined from deleted envelope.ts) ---
// These are type views over ActivityEvent.activity.meta for the spirit orchestration
// channel. The backend publishes them as ActivityEvent with kind=spirit_*.
type SpiritPlanCreatedPayload = {
  plan_id: string;
  spirit_session_id: string;
  complexity_level: string;
  complexity_score: number;
  strategy: string;
  strategy_reason: string;
  topology_hint: string;
  subtask_count: number;
};

type SpiritAllocationCreatedPayload = {
  allocation_id: string;
  task_plan_id: string;
  spirit_session_id: string;
  allocation_count: number;
  status: string;
};

type SpiritOrchestrationStartedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  strategy: string;
  status: string;
  task_plan_id: string;
  allocation_id: string;
  team_ids?: string[];
  max_concurrent_teams?: number;
};

type SpiritOrchestrationCheckpointPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  checkpoint_id: string;
  step: string;
  status: string;
};

type SpiritOrchestrationInterruptedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  status: string;
};

import { Notify } from 'quasar';

/** Orchestration phase derived from WS events. */
export type OrchestrationPhase = 'idle' | 'planning' | 'allocating' | 'orchestrating' | 'completed' | 'interrupted';

const VALID_STRATEGIES = new Set<string>(['template', 'prompt', 'hybrid']);

function isValidStrategy(s: string): boolean {
  return VALID_STRATEGIES.has(s);
}

/**
 * Merge fields from an API-fetched team into an existing WS-driven team.
 * Only fills in fields that WS events don't carry (members, tokenIn, etc.).
 * Never overwrites WS-updated fields (status, progressPct) which are more real-time.
 */
function mergeTeamFields(existing: SpiritTeam, incoming: SpiritTeam) {
  if (incoming.members.length > 0 && existing.members.length === 0) {
    existing.members = incoming.members;
  }
  if (incoming.memberAvatars.length > 0 && existing.memberAvatars.length === 0) {
    existing.memberAvatars = incoming.memberAvatars;
  }
  if (incoming.tokenIn && !existing.tokenIn) {
    existing.tokenIn = incoming.tokenIn;
  }
  if (incoming.tokenOut && !existing.tokenOut) {
    existing.tokenOut = incoming.tokenOut;
  }
  if (incoming.dqScore && !existing.dqScore) {
    existing.dqScore = incoming.dqScore;
  }
  if (incoming.evolutionSuggestion && !existing.evolutionSuggestion) {
    existing.evolutionSuggestion = incoming.evolutionSuggestion;
  }
  if (incoming.interruptReason && !existing.interruptReason) {
    existing.interruptReason = incoming.interruptReason;
  }
}

export const useSpiritTeamStore = defineStore('spiritTeam', () => {
  const teams = ref<SpiritTeam[]>([]);
  const expandedTeamIds = ref<Set<string>>(new Set());
  const activePanelMode = ref<SpiritPanelMode>('spirit');
  const activeTeamId = ref<string | null>(null);
  const activeMemberId = ref<string | null>(null);
  const loading = ref(false);
  const teamProgress = ref<TeamProgressView[]>([]);
  const allTeamsCompleted = ref(false);
  const synthesisCompleted = ref(false);
  const synthesisResult = ref<SynthesisOutput | null>(null);

  // Aggregated token usage from spirit_teams_all_completed event
  const aggregatedTokenIn = ref(0);
  const aggregatedTokenOut = ref(0);

  // Spirit Orchestration state (new envelope types)
  const planCreated = ref<SpiritPlanCreatedPayload | null>(null);
  const allocationCreated = ref<SpiritAllocationCreatedPayload | null>(null);
  const orchestrationStarted = ref<SpiritOrchestrationStartedPayload | null>(null);
  const lastCheckpoint = ref<SpiritOrchestrationCheckpointPayload | null>(null);
  const orchestrationInterrupted = ref<SpiritOrchestrationInterruptedPayload | null>(null);

  /** Orchestration phase derived from WS events (idle → planning → allocating → orchestrating → completed/interrupted). */
  const orchestrationPhase = ref<OrchestrationPhase>('idle');

  // Track the current spirit session ID for loadSpiritTeams and reset.
  const currentSpiritSessionId = ref<string | null>(null);

  /** In-flight load promise to deduplicate concurrent loadSpiritTeams calls. */
  let _loadPromise: Promise<void> | null = null;

  /** Max concurrent teams quota from backend ParallelConfig. */
  const maxConcurrentTeams = ref<number | null>(null);

  /** Last DQ score from spirit_team_completed event. */
  const lastDqScore = ref<DQScoreBreakdown | null>(null);
  /** Last evolution suggestion from spirit_team_completed event. */
  const lastEvolutionSuggestion = ref<EvolutionSuggestion | null>(null);

  /** Team completion breakdown from spirit_teams_all_completed event. */
  const completionStats = ref<CompletionStats | null>(null);

  const activeTeam = computed(() => teams.value.find((t) => t.id === activeTeamId.value) ?? null);

  const activeTeams = computed(() =>
    teams.value.filter(
      (t) => t.status !== 'completed' && t.status !== 'failed' && t.status !== 'cancelled' && t.status !== 'archived',
    ),
  );

  const completedTeams = computed(() => teams.value.filter((t) => t.status === 'completed'));

  const runningTeamCount = computed(
    () => teams.value.filter((t) => t.status === 'running' || t.status === 'pending').length,
  );

  const sortedTeams = computed(() => {
    const statusOrder: Record<string, number> = {
      running: 0,
      pending: 1,
      interrupted: 2,
      completed: 3,
      failed: 4,
      cancelled: 5,
      archived: 6,
    };
    return [...teams.value].sort((a, b) => (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99));
  });

  /**
   * Load spirit teams from API with merge semantics.
   * API teams supplement WS-driven teams (which are more real-time).
   * WS events already added teams are preserved; API fills in missing fields.
   * Teams not in API response are removed (deleted/archived server-side).
   * Concurrent calls for the same sessionId are deduplicated.
   */
  async function loadSpiritTeams(spiritSessionId: string) {
    // Deduplicate: if a load is already in flight for this session, await it
    if (_loadPromise) {
      await _loadPromise;
      // After awaiting, if sessionId changed (e.g. reset), skip
      if (currentSpiritSessionId.value !== spiritSessionId) return;
    }
    loading.value = true;
    _loadPromise = (async () => {
      try {
        const apiTeams = await listSpiritTeams(spiritSessionId);
        currentSpiritSessionId.value = spiritSessionId;
        const apiIds = new Set(apiTeams.map((t) => t.id));
        for (const team of apiTeams) {
          const existing = teams.value.find((t) => t.id === team.id);
          if (existing) {
            // WS already has this team: only fill in fields WS events don't carry
            mergeTeamFields(existing, team);
          } else {
            // API has a team WS hasn't delivered yet (e.g. historical team)
            teams.value.push(team);
          }
        }
        // Remove teams that no longer exist on the server
        teams.value = teams.value.filter((t) => apiIds.has(t.id));
      } catch {
        Notify.create({ type: 'negative', message: '加载团队列表失败', position: 'top' });
      } finally {
        loading.value = false;
        _loadPromise = null;
      }
    })();
    await _loadPromise;
  }

  /** Reload teams for the current spirit session (e.g. after WS reconnect). */
  async function reloadTeams() {
    if (currentSpiritSessionId.value) {
      await loadSpiritTeams(currentSpiritSessionId.value);
    }
  }

  /** Reset all store state — call when switching spirit sessions or leaving chat. */
  function reset() {
    teams.value = [];
    expandedTeamIds.value = new Set();
    activePanelMode.value = 'spirit';
    activeTeamId.value = null;
    activeMemberId.value = null;
    loading.value = false;
    _loadPromise = null;
    teamProgress.value = [];
    allTeamsCompleted.value = false;
    synthesisCompleted.value = false;
    synthesisResult.value = null;
    aggregatedTokenIn.value = 0;
    aggregatedTokenOut.value = 0;
    planCreated.value = null;
    allocationCreated.value = null;
    orchestrationStarted.value = null;
    lastCheckpoint.value = null;
    orchestrationInterrupted.value = null;
    orchestrationPhase.value = 'idle';
    currentSpiritSessionId.value = null;
    maxConcurrentTeams.value = null;
    lastDqScore.value = null;
    lastEvolutionSuggestion.value = null;
    completionStats.value = null;
  }

  function selectTeam(teamId: string) {
    activeTeamId.value = teamId;
    activePanelMode.value = 'team';
    activeMemberId.value = null;
  }

  function selectMember(memberId: string) {
    activeMemberId.value = memberId;
    activePanelMode.value = 'member';
  }

  function returnToSpirit() {
    activePanelMode.value = 'spirit';
    activeTeamId.value = null;
    activeMemberId.value = null;
  }

  function toggleTeamExpand(teamId: string) {
    const next = new Set(expandedTeamIds.value);
    if (next.has(teamId)) {
      next.delete(teamId);
    } else {
      next.add(teamId);
    }
    expandedTeamIds.value = next;
  }

  async function cancelTeam(teamId: string) {
    try {
      await cancelSpiritTeam(teamId);
      // Update status to cancelled instead of removing — consistent with backend behavior.
      updateTeamStatus(teamId, 'cancelled');
      const next = new Set(expandedTeamIds.value);
      next.delete(teamId);
      expandedTeamIds.value = next;
      if (activeTeamId.value === teamId) {
        returnToSpirit();
      }
    } catch {
      Notify.create({ type: 'warning', message: '取消团队请求可能未生效，请刷新确认', position: 'top' });
    }
  }

  async function resumeTeam(teamId: string) {
    try {
      await resumeSpiritTeam(teamId);
      updateTeamStatus(teamId, 'running');
    } catch {
      Notify.create({ type: 'warning', message: '恢复团队请求可能未生效，请刷新确认', position: 'top' });
    }
  }

  async function archiveTeam(teamId: string) {
    try {
      await archiveSpiritTeam(teamId);
      // Remove archived team from the list
      teams.value = teams.value.filter((t) => t.id !== teamId);
    } catch {
      Notify.create({ type: 'warning', message: '归档团队请求可能未生效，请刷新确认', position: 'top' });
    }
  }

  async function retryTeam(teamId: string) {
    try {
      await retrySpiritTeam(teamId);
      updateTeamStatus(teamId, 'pending');
    } catch {
      Notify.create({ type: 'warning', message: '重试团队请求可能未生效，请刷新确认', position: 'top' });
    }
  }

  function updateTeamProgress(progress: TeamProgressView[]) {
    teamProgress.value = progress;
    for (const p of progress) {
      const team = teams.value.find((t) => t.id === p.teamId);
      if (team) {
        team.status = p.status;
        // Use progressPct directly instead of reverse-computing completedSteps
        // from totalSteps (which is always 1 from the backend, making the
        // computed progress permanently 0% or 100%).
        if (p.progressPct >= 0) {
          team.progressPct = p.progressPct;
        }
      }
    }
  }

  function updateTeamStatus(teamId: string, status: SpiritTeamStatus) {
    const team = teams.value.find((t) => t.id === teamId);
    if (!team) return;

    // Terminal state protection: completed/failed/cancelled cannot be overridden
    // by lower-priority events. This prevents race conditions where
    // spirit_team_failed arrives after spirit_team_completed.
    const terminalStates: SpiritTeamStatus[] = ['completed', 'failed', 'cancelled'];
    if (terminalStates.includes(team.status) && !terminalStates.includes(status)) {
      return; // Block non-terminal → terminal regression
    }
    // Same-level terminal override: only allow if new status has equal or higher priority
    // Priority: completed > failed > cancelled (completed should not be overridden by failed)
    const terminalPriority: Record<string, number> = { completed: 3, failed: 2, cancelled: 1 };
    if (terminalStates.includes(team.status) && terminalStates.includes(status)) {
      if ((terminalPriority[status] ?? 0) <= (terminalPriority[team.status] ?? 0)) {
        return; // Don't downgrade terminal status (e.g. completed → failed)
      }
    }

    team.status = status;
  }

  function addTeam(team: SpiritTeam) {
    // Dedup: if team already exists, update it instead of adding a duplicate.
    const idx = teams.value.findIndex((t) => t.id === team.id);
    if (idx >= 0) {
      teams.value[idx] = team;
    } else {
      teams.value.push(team);
    }
  }

  /**
   * Activity-First: Handle a Spirit/Team ActivityEvent.
   *
   * Maps kind+stage to the equivalent handler. Field mapping:
   *   env.metadata   → ev.activity.meta
   *   env.session_id → ev.activity.session_id
   *   env.type       → ev.activity.kind + ev.activity.stage
   */
  function handleSpiritActivityEvent(ev: ActivityEvent) {
    const act = ev.activity;
    const md = (act.meta ?? {}) as Record<string, unknown>;
    const teamId = String(md.team_id ?? act.team_id ?? '');
    const kind = act.kind;
    const stage = act.stage;

    // Map kind+stage to the legacy envelope type
    let envType = '';
    if (kind === 'team_stage') {
      if (stage === 'assembled') envType = 'spirit_team_assembled';
      else if (stage === 'progress') envType = 'spirit_team_progress';
      else if (stage === 'interrupted') envType = 'spirit_team_interrupted';
      else if (stage === 'cancelled') envType = 'spirit_team_cancelled';
      else if (stage === 'completed' || stage === 'finished') envType = 'spirit_team_completed';
      else if (stage === 'failed') envType = 'spirit_team_failed';
      else if (stage === 'orchestration_started') envType = 'spirit_orchestration_started';
      else if (stage === 'orchestration_checkpoint') envType = 'spirit_orchestration_checkpoint';
      else if (stage === 'orchestration_interrupted') envType = 'spirit_orchestration_interrupted';
      else if (stage === 'all_completed' || stage === 'summary') envType = 'spirit_teams_all_completed';
    } else if (kind === 'session') {
      if (stage === 'plan_created') envType = 'spirit_plan_created';
      else if (stage === 'allocation_created') envType = 'spirit_allocation_created';
      else if (stage === 'orchestration_started') envType = 'spirit_orchestration_started';
      else if (stage === 'orchestration_checkpoint') envType = 'spirit_orchestration_checkpoint';
      else if (stage === 'orchestration_interrupted') envType = 'spirit_orchestration_interrupted';
      else if (stage === 'synthesis_completed') envType = 'spirit_synthesis_completed';
      else if (stage === 'orchestration_completed') envType = 'butler.orchestration.completed';
      else if (stage === 'orchestration_failed') envType = 'butler.orchestration.failed';
    } else if (kind === 'plan') {
      envType = 'spirit_plan_created';
    }

    switch (envType) {
      case 'spirit_team_assembled': {
        handleTeamAssembled(teamId, md, act.session_id ?? '');
        break;
      }
      case 'spirit_team_completed':
        handleTeamCompleted(teamId, md);
        break;
      case 'spirit_team_failed':
        handleTeamFailed(teamId, md);
        break;
      case 'spirit_team_cancelled':
        if (teamId) updateTeamStatus(teamId, 'cancelled');
        break;
      case 'spirit_team_interrupted':
        handleTeamInterrupted(teamId, md);
        break;
      case 'spirit_team_progress':
        handleTeamProgress(teamId, md);
        break;
      case 'spirit_teams_all_completed':
        handleAllTeamsCompleted(md);
        break;
      case 'spirit_synthesis_completed': {
        handleSynthesisCompleted(md);
        break;
      }
      case 'spirit_plan_created':
        planCreated.value = md as unknown as SpiritPlanCreatedPayload;
        orchestrationPhase.value = 'planning';
        break;
      case 'spirit_allocation_created':
        allocationCreated.value = md as unknown as SpiritAllocationCreatedPayload;
        orchestrationPhase.value = 'allocating';
        break;
      case 'spirit_orchestration_started':
        handleOrchestrationStarted(md);
        break;
      case 'spirit_orchestration_checkpoint':
        lastCheckpoint.value = md as unknown as SpiritOrchestrationCheckpointPayload;
        break;
      case 'spirit_orchestration_interrupted':
        orchestrationInterrupted.value = md as unknown as SpiritOrchestrationInterruptedPayload;
        orchestrationPhase.value = 'interrupted';
        break;
      case 'butler.orchestration.started':
      case 'butler.orchestration.completed':
        break;
      case 'butler.orchestration.failed':
        if (teamId) {
          const existing = teams.value.find((t) => t.id === teamId);
          if (existing && !['completed', 'failed', 'cancelled'].includes(existing.status)) {
            updateTeamStatus(teamId, 'failed');
          }
        }
        break;
    }
  }

  // --- Spirit handlers ---

  function handleTeamAssembled(teamId: string, md: Record<string, unknown>, sessionId: string) {
    synthesisCompleted.value = false;
    synthesisResult.value = null;
    allTeamsCompleted.value = false;
    if (teamId) {
      addTeam({
        id: teamId,
        teamName: String(md.team_name ?? ''),
        taskSummary: String(md.task_summary ?? ''),
        status: 'pending',
        mode: String(md.mode || 'coordinator') as SpiritTeamMode,
        memberAvatars: [],
        completedSteps: 0,
        totalSteps: Number(md.total_steps ?? 1),
        progressPct: 0,
        durationMs: Number(md.duration_ms ?? 0),
        spiritSessionId: sessionId,
        teamSessionId: String(md.session_id ?? ''),
        members: [],
        sharedAgentIds: [],
        dagNodeId: String(md.dag_node_id ?? ''),
        graphExecutionId: '',
        interruptReason: '',
        dependsOn: Array.isArray(md.depends_on) ? md.depends_on.map(String) : [],
        topologyReason: String(md.topology_reason ?? ''),
      });
    }
  }

  function handleTeamCompleted(teamId: string, md: Record<string, unknown>) {
    if (!teamId) return;
    updateTeamStatus(teamId, 'completed');
    const team = teams.value.find((t) => t.id === teamId);
    if (!team) return;

    const dMs = Number(md.duration_ms ?? 0);
    if (dMs > 0) team.durationMs = dMs;

    const tokenIn = Number(md.total_token_in ?? 0);
    const tokenOut = Number(md.total_token_out ?? 0);
    if (tokenIn > 0 || tokenOut > 0) {
      team.tokenIn = tokenIn;
      team.tokenOut = tokenOut;
    }

    const dqRaw = md.dq_score as Record<string, number> | undefined;
    if (dqRaw && typeof dqRaw.overall === 'number') {
      const dqScore: DQScoreBreakdown = {
        validity: dqRaw.validity ?? 0,
        specificity: dqRaw.specificity ?? 0,
        correctness: dqRaw.correctness ?? 0,
        overall: dqRaw.overall,
      };
      team.dqScore = dqScore;
      lastDqScore.value = dqScore;
    }

    const evoRaw = md.evolution_suggestion as Record<string, unknown> | undefined;
    if (evoRaw && typeof evoRaw.suggestedTopology === 'string') {
      const evo: EvolutionSuggestion = {
        currentTopology: String(evoRaw.currentTopology ?? ''),
        suggestedTopology: String(evoRaw.suggestedTopology),
        reason: String(evoRaw.reason ?? ''),
        dqScore: Number(evoRaw.dqScore ?? 0),
      };
      team.evolutionSuggestion = evo;
      lastEvolutionSuggestion.value = evo;
    }
  }

  function handleTeamFailed(teamId: string, md: Record<string, unknown>) {
    if (!teamId) return;
    // Don't override completed/cancelled with failed (terminal state protection)
    const existing = teams.value.find((t) => t.id === teamId);
    if (existing?.status === 'completed' || existing?.status === 'cancelled') return;
    updateTeamStatus(teamId, 'failed');
    const team = teams.value.find((t) => t.id === teamId);
    if (!team) return;

    const dMs = Number(md.duration_ms ?? 0);
    if (dMs > 0) team.durationMs = dMs;
    const tkIn = Number(md.total_token_in ?? 0);
    const tkOut = Number(md.total_token_out ?? 0);
    if (tkIn > 0 || tkOut > 0) {
      team.tokenIn = tkIn;
      team.tokenOut = tkOut;
    }
  }

  function handleTeamInterrupted(teamId: string, md: Record<string, unknown>) {
    if (!teamId) return;
    updateTeamStatus(teamId, 'interrupted');
    const team = teams.value.find((t) => t.id === teamId);
    if (team && md.interrupt_reason) {
      team.interruptReason = String(md.interrupt_reason);
    }
  }

  function handleTeamProgress(teamId: string, md: Record<string, unknown>) {
    if (!teamId) return;
    const pct = Number(md.progress_pct ?? 0);
    const durationMs = Number(md.duration_ms ?? 0);
    const newStatus = String(md.status ?? 'running');
    if (!isValidTeamStatus(newStatus)) return;
    const team = teams.value.find((t) => t.id === teamId);
    if (!team) return;

    // Use updateTeamStatus for consistent terminal state protection
    updateTeamStatus(teamId, newStatus as SpiritTeamStatus);

    if (pct >= 0) team.progressPct = pct;
    if (durationMs > 0) team.durationMs = durationMs;
  }

  function handleAllTeamsCompleted(md: Record<string, unknown>) {
    allTeamsCompleted.value = true;
    orchestrationPhase.value = 'completed';
    const aggTokenIn = Number(md.total_token_in ?? 0);
    const aggTokenOut = Number(md.total_token_out ?? 0);
    if (aggTokenIn > 0 || aggTokenOut > 0) {
      aggregatedTokenIn.value = aggTokenIn;
      aggregatedTokenOut.value = aggTokenOut;
    }
    const total = Number(md.total_teams ?? 0);
    if (total > 0) {
      completionStats.value = {
        totalTeams: total,
        completedTeams: Number(md.completed_teams ?? 0),
        failedTeams: Number(md.failed_teams ?? 0),
      };
    }
  }

  function handleSynthesisCompleted(md: Record<string, unknown>) {
    synthesisCompleted.value = true;
    const rawResults = Array.isArray(md.team_results)
      ? (md.team_results as Array<{
          team_id: string;
          team_name: string;
          task_name: string;
          status: string;
          summary: string;
          key_findings: string;
        }>)
      : [];
    synthesisResult.value = {
      strategy: isValidStrategy(String(md.strategy ?? 'template'))
        ? (String(md.strategy ?? 'template') as SynthesisOutput['strategy'])
        : 'template',
      content: String(md.content ?? ''),
      teamResults: rawResults.map((r) => ({
        teamId: String(r.team_id ?? ''),
        teamName: String(r.team_name ?? ''),
        taskName: String(r.task_name ?? ''),
        status: isValidTeamStatus(String(r.status ?? '')) ? (String(r.status ?? '') as SpiritTeamStatus) : 'failed',
        summary: String(r.summary ?? ''),
        keyFindings: String(r.key_findings ?? ''),
      })),
      synthesizedAt: new Date().toISOString(),
    };
  }

  function handleOrchestrationStarted(md: Record<string, unknown>) {
    const payload = md as unknown as SpiritOrchestrationStartedPayload;
    orchestrationStarted.value = payload;
    orchestrationPhase.value = 'orchestrating';
    if (payload.max_concurrent_teams && payload.max_concurrent_teams > 0) {
      maxConcurrentTeams.value = payload.max_concurrent_teams;
    }
    // Teams are now persisted in DB — merge API data to ensure list completeness
    if (currentSpiritSessionId.value) {
      void loadSpiritTeams(currentSpiritSessionId.value);
    }
  }

  return {
    teams,
    expandedTeamIds,
    activePanelMode,
    activeTeamId,
    activeMemberId,
    loading,
    teamProgress,
    allTeamsCompleted,
    synthesisCompleted,
    synthesisResult,
    aggregatedTokenIn,
    aggregatedTokenOut,
    planCreated,
    allocationCreated,
    orchestrationStarted,
    lastCheckpoint,
    orchestrationInterrupted,
    orchestrationPhase,
    currentSpiritSessionId,
    maxConcurrentTeams,
    lastDqScore,
    lastEvolutionSuggestion,
    completionStats,
    activeTeam,
    activeTeams,
    completedTeams,
    runningTeamCount,
    sortedTeams,
    loadSpiritTeams,
    reloadTeams,
    reset,
    selectTeam,
    selectMember,
    returnToSpirit,
    toggleTeamExpand,
    cancelTeam,
    resumeTeam,
    archiveTeam,
    retryTeam,
    updateTeamProgress,
    updateTeamStatus,
    addTeam,
    handleSpiritActivityEvent,
  };
});
