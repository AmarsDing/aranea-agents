import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { listSpiritTeams, cancelSpiritTeam, resumeSpiritTeam, archiveSpiritTeam, retrySpiritTeam } from '../../features/spirit/api';
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
import type { Envelope } from '../../realtime/envelope';
import type {
  SpiritPlanCreatedPayload,
  SpiritAllocationCreatedPayload,
  SpiritOrchestrationStartedPayload,
  SpiritOrchestrationCheckpointPayload,
  SpiritOrchestrationInterruptedPayload,
} from '../../realtime/envelope';
import { Notify } from 'quasar';

const VALID_STRATEGIES = new Set<string>(['template', 'prompt', 'hybrid']);

function isValidStrategy(s: string): boolean {
  return VALID_STRATEGIES.has(s);
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

  // Track the current spirit session ID for loadSpiritTeams and reset.
  const currentSpiritSessionId = ref<string | null>(null);

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

  async function loadSpiritTeams(spiritSessionId: string) {
    loading.value = true;
    try {
      teams.value = await listSpiritTeams(spiritSessionId);
      currentSpiritSessionId.value = spiritSessionId;
    } catch {
      Notify.create({ type: 'negative', message: '加载团队列表失败', position: 'top' });
    } finally {
      loading.value = false;
    }
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
    if (team) {
      team.status = status;
    }
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

  function handleSpiritEnvelope(env: Envelope) {
    const md = (env.metadata ?? {}) as Record<string, unknown>;
    const teamId = String(md.team_id ?? '');

    switch (env.type) {
      case 'spirit_team_assembled':
        handleTeamAssembled(teamId, md, env);
        break;
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
      case 'spirit_synthesis_completed':
        handleSynthesisCompleted(env);
        break;
      case 'spirit_plan_created':
        planCreated.value = md as unknown as SpiritPlanCreatedPayload;
        break;
      case 'spirit_allocation_created':
        allocationCreated.value = md as unknown as SpiritAllocationCreatedPayload;
        break;
      case 'spirit_orchestration_started':
        handleOrchestrationStarted(md);
        break;
      case 'spirit_orchestration_checkpoint':
        lastCheckpoint.value = md as unknown as SpiritOrchestrationCheckpointPayload;
        break;
      case 'spirit_orchestration_interrupted':
        orchestrationInterrupted.value = md as unknown as SpiritOrchestrationInterruptedPayload;
        break;
      case 'butler.orchestration.started':
      case 'butler.orchestration.completed':
        break;
      case 'butler.orchestration.failed':
        if (teamId) updateTeamStatus(teamId, 'failed');
        break;
    }
  }

  // --- Spirit envelope handlers (split from handleSpiritEnvelope) ---

  function handleTeamAssembled(teamId: string, md: Record<string, unknown>, env: Envelope) {
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
        spiritSessionId: env.session_id ?? '',
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
    const existing = teams.value.find((t) => t.id === teamId);
    if (existing?.status === 'cancelled') return;
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
    const team = teams.value.find((t) => t.id === teamId);
    if (!team) return;

    const regressions: Record<string, Set<string>> = {
      running: new Set(['pending']),
      completed: new Set(['pending', 'running']),
      failed: new Set(['pending', 'running']),
      cancelled: new Set(['pending', 'running']),
      archived: new Set(['pending', 'running', 'completed', 'failed', 'cancelled']),
    };
    const blocked = regressions[team.status];
    if (!blocked || !blocked.has(newStatus)) {
      if (isValidTeamStatus(newStatus)) {
        team.status = newStatus;
      }
    }
    if (pct >= 0) team.progressPct = pct;
    if (durationMs > 0) team.durationMs = durationMs;
  }

  function handleAllTeamsCompleted(md: Record<string, unknown>) {
    allTeamsCompleted.value = true;
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

  function handleSynthesisCompleted(env: Envelope) {
    synthesisCompleted.value = true;
    const rawResults = Array.isArray(env.metadata?.team_results)
      ? (env.metadata.team_results as Array<{
          team_id: string;
          team_name: string;
          task_name: string;
          status: string;
          summary: string;
          key_findings: string;
        }>)
      : [];
    synthesisResult.value = {
      strategy: isValidStrategy(String(env.metadata?.strategy ?? 'template'))
        ? (String(env.metadata?.strategy ?? 'template') as SynthesisOutput['strategy'])
        : 'template',
      content: String(env.metadata?.content ?? ''),
      teamResults: rawResults.map((r) => ({
        teamId: String(r.team_id ?? ''),
        teamName: String(r.team_name ?? ''),
        taskName: String(r.task_name ?? ''),
        status: isValidTeamStatus(String(r.status ?? ''))
          ? (String(r.status ?? '') as SpiritTeamStatus)
          : 'failed',
        summary: String(r.summary ?? ''),
        keyFindings: String(r.key_findings ?? ''),
      })),
      synthesizedAt: new Date().toISOString(),
    };
  }

  function handleOrchestrationStarted(md: Record<string, unknown>) {
    const payload = md as unknown as SpiritOrchestrationStartedPayload;
    orchestrationStarted.value = payload;
    if (payload.max_concurrent_teams && payload.max_concurrent_teams > 0) {
      maxConcurrentTeams.value = payload.max_concurrent_teams;
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
    handleSpiritEnvelope,
  };
});
