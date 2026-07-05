import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import {
  listSpiritTeams,
  cancelSpiritTeam,
  resumeSpiritTeam,
  archiveSpiritTeam,
  retrySpiritTeam,
  pauseSpiritTeam,
  unpauseSpiritTeam,
  injectSpiritTeam,
  pauseAgentSession,
  injectAgentSession,
} from '../../features/spirit/api';
import { i18n } from '../../i18n';
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
function parseMembers(md: Record<string, unknown>): SpiritMember[] {
  const raw = md.members;
  if (!Array.isArray(raw)) return [];
  return raw.map((m) => {
    const item = (m ?? {}) as Record<string, unknown>;
    return {
      agentId: String(item.agent_id ?? item.agentId ?? ''),
      agentKey: String(item.agent_key ?? item.agentKey ?? ''),
      displayName: String(item.display_name ?? item.agent_name ?? item.displayName ?? ''),
      role: String(item.role ?? ''),
      status: String(item.status ?? ''),
      avatarUrl: String(item.avatar_url ?? item.avatarUrl ?? ''),
    };
  });
}

function mergeTeamFields(existing: SpiritTeam, incoming: SpiritTeam) {
  // Merge members by agent_key: preserve real-time status from WS, but fill
  // in static profile fields (avatar_url, display_name, role) from the API.
  if (incoming.members.length > 0) {
    if (existing.members.length === 0) {
      existing.members = incoming.members;
    } else {
      for (const inMember of incoming.members) {
        const existingMember = existing.members.find((m) => m.agentKey === inMember.agentKey);
        if (!existingMember) {
          existing.members.push(inMember);
        } else {
          if (!existingMember.avatarUrl && inMember.avatarUrl) {
            existingMember.avatarUrl = inMember.avatarUrl;
          }
          if (!existingMember.displayName && inMember.displayName) {
            existingMember.displayName = inMember.displayName;
          }
          if (!existingMember.role && inMember.role) {
            existingMember.role = inMember.role;
          }
        }
      }
    }
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
  if (incoming.createdAt && !existing.createdAt) {
    existing.createdAt = incoming.createdAt;
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
  /** Tracks which sessionId the in-flight _loadPromise is loading. */
  let _loadSessionId: string | null = null;

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
    // B.9.1: Agent cards are ordered by team creation time, not by status.
    return [...teams.value].sort((a, b) => a.createdAt - b.createdAt);
  });

  /**
   * Load spirit teams from API with merge semantics.
   * API teams supplement WS-driven teams (which are more real-time).
   * WS events already added teams are preserved; API fills in missing fields.
   * Teams not in API response are removed (deleted/archived server-side).
   * Concurrent calls for the same sessionId are deduplicated.
   */
  async function loadSpiritTeams(spiritSessionId: string) {
    // Deduplicate: only await if a load is in flight for the SAME session.
    // If a load is in flight for a DIFFERENT session, we proceed to load this
    // session's data — the older load will skip its state mutation when it
    // completes (see _loadSessionId guard below).
    if (_loadPromise && _loadSessionId === spiritSessionId) {
      await _loadPromise;
      return;
    }
    loading.value = true;
    _loadSessionId = spiritSessionId;
    // When switching to a DIFFERENT spirit session, clear teams from the
    // previous session before loading. Without this, teams from session A
    // persist when switching to session B (especially when B has no teams).
    const isSessionSwitch = currentSpiritSessionId.value !== null && currentSpiritSessionId.value !== spiritSessionId;
    _loadPromise = (async () => {
      try {
        const apiTeams = await listSpiritTeams(spiritSessionId);
        // If a newer load superseded this one (different session ID requested
        // during the await), skip the state mutation to avoid clobbering the
        // newer load's results.
        if (_loadSessionId !== spiritSessionId) return;
        currentSpiritSessionId.value = spiritSessionId;
        if (isSessionSwitch) {
          // Clear teams from the previous session before merging new data.
          teams.value = [];
        }
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
        // Remove teams that no longer exist on the server — but only when the
        // API returned a non-empty list. An empty API response usually means
        // the server hasn't persisted yet or a transient failure; clearing all
        // teams in that case would wipe WS-driven teams (race condition fix).
        // Note: when isSessionSwitch is true, teams.value was already cleared
        // above, so this filter is a no-op for the switch case.
        if (apiTeams.length > 0 && !isSessionSwitch) {
          teams.value = teams.value.filter((t) => apiIds.has(t.id));
        }
      } catch {
        Notify.create({ type: 'negative', message: i18n.global.t('chat.teamStage.loadFailed'), position: 'top' });
      } finally {
        // Only clear if this is still the active load — otherwise a newer load
        // owns these slots and will clear them itself.
        if (_loadSessionId === spiritSessionId) {
          loading.value = false;
          _loadPromise = null;
          _loadSessionId = null;
        }
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
    _loadSessionId = null;
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
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.cancelFailed'), position: 'top' });
    }
  }

  async function resumeTeam(teamId: string) {
    try {
      await resumeSpiritTeam(teamId);
      updateTeamStatus(teamId, 'running');
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.resumeFailed'), position: 'top' });
    }
  }

  // pauseTeam transitions a running team run to paused state (B.5.3).
  // MVP: cancels in-flight member step + marks run as paused.
  async function pauseTeam(teamId: string) {
    try {
      await pauseSpiritTeam(teamId);
      updateTeamStatus(teamId, 'paused');
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.pauseFailed'), position: 'top' });
    }
  }

  // unpauseTeam transitions a paused team run back to running (B.5.3).
  // MVP: flips status marker; user must inject message to resume execution.
  async function unpauseTeam(teamId: string) {
    try {
      await unpauseSpiritTeam(teamId);
      updateTeamStatus(teamId, 'running');
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.unpauseFailed'), position: 'top' });
    }
  }

  // injectTeam injects a user message into the team's active/paused run
  // pending queue (B.5.3). Returns true on success.
  async function injectTeam(teamId: string, message: string): Promise<boolean> {
    try {
      await injectSpiritTeam(teamId, message);
      Notify.create({ type: 'positive', message: i18n.global.t('chat.teamStage.injectSent'), position: 'top' });
      return true;
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.injectFailed'), position: 'top' });
      return false;
    }
  }

  // pauseAgent pauses a running sub-agent session (B.5.3).
  // Used by MemberSessionPanel input bar's stop button.
  async function pauseAgent(sessionId: string): Promise<void> {
    try {
      await pauseAgentSession(sessionId);
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.pauseFailed'), position: 'top' });
    }
  }

  // injectAgent enqueues a user message to a sub-agent session's pending queue
  // (POST /v1/chat/enqueue). Used by MemberSessionPanel input bar's send button.
  async function injectAgent(sessionId: string, message: string): Promise<boolean> {
    try {
      await injectAgentSession(sessionId, message);
      Notify.create({ type: 'positive', message: i18n.global.t('chat.teamStage.injectSent'), position: 'top' });
      return true;
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.injectFailed'), position: 'top' });
      return false;
    }
  }

  async function archiveTeam(teamId: string) {
    try {
      await archiveSpiritTeam(teamId);
      // Remove archived team from the list
      teams.value = teams.value.filter((t) => t.id !== teamId);
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.archiveFailed'), position: 'top' });
    }
  }

  async function retryTeam(teamId: string) {
    try {
      await retrySpiritTeam(teamId);
      updateTeamStatus(teamId, 'pending');
    } catch {
      Notify.create({ type: 'warning', message: i18n.global.t('chat.teamStage.retryFailed'), position: 'top' });
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
        const ts = act.timestamp ? new Date(act.timestamp).getTime() : Date.now();
        handleTeamAssembled(teamId, md, act.session_id ?? '', String(act.status ?? ''), ts);
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

  function handleTeamAssembled(
    teamId: string,
    md: Record<string, unknown>,
    sessionId: string,
    activityStatus: string,
    timestampMs: number,
  ) {
    synthesisCompleted.value = false;
    synthesisResult.value = null;
    allTeamsCompleted.value = false;
    if (!teamId) return;

    const incomingStatus = String(md.status ?? activityStatus ?? '');
    const initialStatus: SpiritTeamStatus = isValidTeamStatus(incomingStatus) ? incomingStatus : 'pending';
    const members = parseMembers(md);

    const existing = teams.value.find((t) => t.id === teamId);
    if (existing) {
      // Merge identifying/structural fields from the assembled event without
      // overwriting real-time WS-updated fields (status, progressPct). This
      // prevents a later "assembled" event (e.g. publishTeamRunStartedEvent
      // with Stage=assembled, Status=running) from clobbering a running
      // status, and also prevents the initial pending assembled event from
      // overwriting status set by a progress event.
      if (md.team_name) existing.teamName = String(md.team_name);
      if (md.task_summary) existing.taskSummary = String(md.task_summary);
      if (md.mode) existing.mode = String(md.mode || 'coordinator') as SpiritTeamMode;
      if (md.session_id) existing.teamSessionId = String(md.session_id);
      if (md.dag_node_id) existing.dagNodeId = String(md.dag_node_id);
      if (Array.isArray(md.depends_on)) existing.dependsOn = md.depends_on.map(String);
      if (md.topology_reason) existing.topologyReason = String(md.topology_reason);
      if (members.length > 0 && existing.members.length === 0) {
        existing.members = members;
      }
      if (!existing.createdAt && timestampMs > 0) {
        existing.createdAt = timestampMs;
      }
      if (initialStatus !== 'pending' || existing.status === 'pending') {
        updateTeamStatus(teamId, initialStatus);
      }
    } else {
      addTeam({
        id: teamId,
        teamName: String(md.team_name ?? ''),
        taskSummary: String(md.task_summary ?? ''),
        status: initialStatus,
        mode: String(md.mode || 'coordinator') as SpiritTeamMode,
        memberAvatars: [],
        completedSteps: 0,
        totalSteps: Number(md.total_steps ?? 1),
        progressPct: 0,
        durationMs: Number(md.duration_ms ?? 0),
        spiritSessionId: sessionId,
        teamSessionId: String(md.session_id ?? ''),
        members,
        sharedAgentIds: [],
        createdAt: timestampMs > 0 ? timestampMs : Date.now(),
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
    pauseTeam,
    unpauseTeam,
    injectTeam,
    pauseAgent,
    injectAgent,
    archiveTeam,
    retryTeam,
    updateTeamProgress,
    updateTeamStatus,
    addTeam,
    handleSpiritActivityEvent,
  };
});
