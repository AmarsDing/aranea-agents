import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { listSpiritTeams, cancelSpiritTeam } from '../../features/spirit/api';
import type { SpiritTeam, SpiritPanelMode, SpiritTeamMode, SpiritTeamStatus, TeamProgressView, SynthesisOutput } from '../../features/spirit/types';
import type { Envelope } from '../../realtime/envelope';
import type {
  SpiritPlanCreatedPayload,
  SpiritAllocationCreatedPayload,
  SpiritOrchestrationStartedPayload,
  SpiritOrchestrationCheckpointPayload,
  SpiritOrchestrationInterruptedPayload,
} from '../../realtime/envelope';
import { Notify } from 'quasar';

const VALID_TEAM_STATUSES = new Set<string>([
  'pending', 'running', 'completed', 'failed', 'cancelled', 'interrupted', 'archived',
]);

function isValidTeamStatus(s: string): s is SpiritTeamStatus {
  return VALID_TEAM_STATUSES.has(s);
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

  // Spirit Orchestration state (new envelope types)
  const planCreated = ref<SpiritPlanCreatedPayload | null>(null);
  const allocationCreated = ref<SpiritAllocationCreatedPayload | null>(null);
  const orchestrationStarted = ref<SpiritOrchestrationStartedPayload | null>(null);
  const lastCheckpoint = ref<SpiritOrchestrationCheckpointPayload | null>(null);
  const orchestrationInterrupted = ref<SpiritOrchestrationInterruptedPayload | null>(null);

  // Track the current spirit session ID for loadSpiritTeams and reset.
  const currentSpiritSessionId = ref<string | null>(null);

  const activeTeam = computed(() => teams.value.find((t) => t.id === activeTeamId.value) ?? null);

  const activeTeams = computed(() =>
    teams.value.filter((t) => t.status !== 'completed' && t.status !== 'failed' && t.status !== 'cancelled' && t.status !== 'archived'),
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
    planCreated.value = null;
    allocationCreated.value = null;
    orchestrationStarted.value = null;
    lastCheckpoint.value = null;
    orchestrationInterrupted.value = null;
    currentSpiritSessionId.value = null;
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

  function updateTeamProgress(progress: TeamProgressView[]) {
    teamProgress.value = progress;
    for (const p of progress) {
      const team = teams.value.find((t) => t.id === p.teamId);
      if (team) {
        team.status = p.status;
        if (p.progressPct >= 0) {
          if (team.totalSteps <= 0) {
            team.totalSteps = 100;
          }
          team.completedSteps = Math.round((p.progressPct * team.totalSteps) / 100);
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
        synthesisCompleted.value = false;
        synthesisResult.value = null;
        allTeamsCompleted.value = false;
        if (teamId) {
          addTeam({
            id: teamId,
            teamName: String(md.team_name ?? ''),
            taskSummary: String(md.task_summary ?? ''),
            status: 'pending',
            mode: (String(md.mode || 'coordinator')) as SpiritTeamMode,
            memberAvatars: [],
            completedSteps: 0,
            totalSteps: Number(md.total_steps ?? 1),
            durationMs: Number(md.duration_ms ?? 0),
            spiritSessionId: env.session_id ?? '',
            teamSessionId: String(md.session_id ?? ''),
            members: [],
            sharedAgentIds: [],
            dagNodeId: String(md.dag_node_id ?? ''),
            dependsOn: Array.isArray(md.depends_on) ? md.depends_on.map(String) : [],
            topologyReason: String(md.topology_reason ?? ''),
          });
        }
        break;

      case 'spirit_team_completed':
        if (teamId) {
          updateTeamStatus(teamId, 'completed');
          const completedTeam = teams.value.find((t) => t.id === teamId);
          if (completedTeam) {
            const dMs = Number(md.duration_ms ?? 0);
            if (dMs > 0) completedTeam.durationMs = dMs;
          }
        }
        break;

      case 'spirit_team_failed':
        if (teamId) {
          updateTeamStatus(teamId, 'failed');
          const failedTeam = teams.value.find((t) => t.id === teamId);
          if (failedTeam) {
            const dMs = Number(md.duration_ms ?? 0);
            if (dMs > 0) failedTeam.durationMs = dMs;
          }
        }
        break;

      case 'spirit_team_progress':
        if (teamId) {
          const pct = Number(md.progress_pct ?? 0);
          const durationMs = Number(md.duration_ms ?? 0);
          const newStatus = String(md.status ?? 'running');
          // Only allow forward state transitions — prevent running → pending regression.
          const team = teams.value.find((t) => t.id === teamId);
          if (team) {
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
            if (pct >= 0 && team.totalSteps > 0) {
              team.completedSteps = Math.round((pct * team.totalSteps) / 100);
            }
            if (durationMs > 0) {
              team.durationMs = durationMs;
            }
          }
        }
        break;

      case 'spirit_teams_all_completed':
        allTeamsCompleted.value = true;
        break;

      case 'spirit_synthesis_completed':
        synthesisCompleted.value = true;
        // Do NOT reset allTeamsCompleted — all teams completed is a fact
        // regardless of whether synthesis has also completed.
        {
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
            strategy: String(env.metadata?.strategy ?? 'template') as SynthesisOutput['strategy'],
            content: String(env.metadata?.content ?? ''),
            teamResults: rawResults.map((r) => ({
              teamId: String(r.team_id ?? ''),
              teamName: String(r.team_name ?? ''),
              taskName: String(r.task_name ?? ''),
              status: String(r.status ?? ''),
              summary: String(r.summary ?? ''),
              keyFindings: String(r.key_findings ?? ''),
            })),
            synthesizedAt: new Date().toISOString(),
          };
        }
        break;

      // --- New Spirit Orchestration envelope types ---

      case 'spirit_plan_created':
        {
          const payload = md as unknown as SpiritPlanCreatedPayload;
          planCreated.value = payload;
        }
        break;

      case 'spirit_allocation_created':
        {
          const payload = md as unknown as SpiritAllocationCreatedPayload;
          allocationCreated.value = payload;
        }
        break;

      case 'spirit_orchestration_started':
        {
          const payload = md as unknown as SpiritOrchestrationStartedPayload;
          orchestrationStarted.value = payload;
        }
        break;

      case 'spirit_orchestration_checkpoint':
        {
          const payload = md as unknown as SpiritOrchestrationCheckpointPayload;
          lastCheckpoint.value = payload;
        }
        break;

      case 'spirit_orchestration_interrupted':
        {
          const payload = md as unknown as SpiritOrchestrationInterruptedPayload;
          orchestrationInterrupted.value = payload;
        }
        break;
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
    planCreated,
    allocationCreated,
    orchestrationStarted,
    lastCheckpoint,
    orchestrationInterrupted,
    currentSpiritSessionId,
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
    updateTeamProgress,
    updateTeamStatus,
    addTeam,
    handleSpiritEnvelope,
  };
});
