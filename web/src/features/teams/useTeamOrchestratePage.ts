import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { compiledGraphToGraphDef } from '../orchestration/compileApi';
import type { CompileTeamGraphResult } from '../orchestration/compileApi';
import type { GraphDefinition } from '../graph/types';
import type { Team, TeamDefinition, TeamRun } from './types';
import { findActiveTeamRun } from './api';
import { parseDefinition } from '../../components/teams/teamUtils';
import { useTeamsStore } from '../../stores/teams';
import { useGraphStore } from '../../stores/graph';
import { useOrchestrationStore } from '../../stores/orchestration';
import { useOrchestrationStream } from '../orchestration/useOrchestrationStream';
import { buildExecNodeStates } from '../orchestration/teamGraphAdapter';
import type { AgentNodeState, TeamRunObservatory } from '../orchestration/types';

export function useTeamOrchestratePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const teamsStore = useTeamsStore();
  const graphStore = useGraphStore();
  const orchestrationStore = useOrchestrationStore();

  const isDark = computed(() => $q.dark.isActive);
  const teamId = computed(() => String(route.params.teamId ?? ''));

  const loading = ref(true);
  const error = ref('');
  const teamRow = ref<Team | null>(null);
  const compiled = ref<CompileTeamGraphResult | null>(null);
  const definition = ref<TeamDefinition | null>(null);
  const readOnly = ref(false);

  const selectedNodeId = ref<string | null>(null);
  const activeRun = ref<TeamRun | null>(null);
  const observatory = ref<TeamRunObservatory | null>(null);
  const stream = ref<ReturnType<typeof useOrchestrationStream> | null>(null);

  const graphDef = reactive<GraphDefinition>({
    id: '',
    name: '',
    description: '',
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: false,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  });

  const liveConnected = computed(() => stream.value?.connected ?? false);
  const liveMode = computed(() => Boolean(activeRun.value && observatory.value));

  const nodeList = computed<AgentNodeState[]>(() => {
    if (!stream.value) return [];
    return [...stream.value.nodes.values()];
  });

  const selectedLiveState = computed(() => {
    if (!selectedNodeId.value || !stream.value) return null;
    return stream.value.nodes.get(selectedNodeId.value) ?? null;
  });

  const idleExecNodeStates = ref(new Map<string, { status: string; fineStatus?: string }>());

  watch(
    () => graphDef.nodes.map((n) => `${n.id}:${n.type}`).join('\0'),
    () => {
      const map = new Map<string, { status: string; fineStatus?: string }>();
      for (const node of graphDef.nodes) {
        if (node.type === 'agent') {
          map.set(node.id, { status: 'waiting', fineStatus: 'idle' });
        }
      }
      idleExecNodeStates.value = map;
    },
    { immediate: true },
  );

  const execNodeStates = computed(() => {
    if (stream.value && stream.value.nodes.size > 0) {
      return buildExecNodeStates(stream.value.nodes);
    }
    return idleExecNodeStates.value;
  });

  const issues = computed(() => compiled.value?.issues ?? []);

  function applyCompiled(result: CompileTeamGraphResult) {
    compiled.value = result;
    Object.assign(graphDef, compiledGraphToGraphDef(result, teamRow.value?.display_name || 'team-orchestration'));
  }

  function disconnectLiveRun() {
    stream.value?.disconnect();
    stream.value = null;
    activeRun.value = null;
    observatory.value = null;
  }

  async function connectLiveRun() {
    disconnectLiveRun();
    if (!teamRow.value?.has_active_run) return;

    try {
      const run = await findActiveTeamRun(teamId.value);
      if (!run) return;

      activeRun.value = run;
      const obs = await orchestrationStore.fetchRunObservatory(run.id);
      observatory.value = obs;

      const s = useOrchestrationStream(obs.session_id, obs.run_id);
      stream.value = s;
      s.seed(obs.nodes);
    } catch (e) {
      const message = e instanceof Error ? e.message : '连接运行观测流失败';
      $q.notify({ type: 'warning', message });
    }
  }

  async function reload() {
    loading.value = true;
    error.value = '';
    try {
      const team = await teamsStore.fetchTeam(teamId.value);
      teamRow.value = team;
      readOnly.value = Boolean(team.has_active_run);
      definition.value = parseDefinition(team);
      const result = await orchestrationStore.compileTeam(teamId.value);
      applyCompiled(result);
      if (team.has_active_run) {
        await connectLiveRun();
      } else {
        disconnectLiveRun();
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
  }

  function openObservatory() {
    if (!activeRun.value) return;
    router.push({
      name: 'team-run-observatory',
      params: { teamId: teamId.value, runId: activeRun.value.id },
    });
  }

  function goBack() {
    router.push({ name: 'team' });
  }

  // ── M53 Phase 11 F3：「在 Graph 编辑器中打开」──
  // 目标图资产解析：linked_external → linked_graph_id；preset/custom → team-owned
  // 资产按 teamId 反查（物化时 GraphDefinition.TeamID=team.ID，见 MaterializeTeamGraphDefinition）。
  async function resolveGraphEditorTargetId(): Promise<string> {
    const def = definition.value;
    if (def?.source === 'linked_external' && String(def.linked_graph_id ?? '').trim()) {
      return String(def.linked_graph_id).trim();
    }
    const findOwned = () =>
      graphStore.graphs.find((g) => g.teamId === teamId.value && g.metadata?.team_owned === true)?.id ?? '';
    const owned = findOwned();
    if (owned) return owned;
    // 图列表可能尚未加载：拉取后再查一次
    await graphStore.loadGraphs(200).catch(() => undefined);
    return findOwned();
  }

  const openingGraphEditor = ref(false);
  async function openInGraphEditor() {
    if (openingGraphEditor.value) return;
    openingGraphEditor.value = true;
    try {
      const id = await resolveGraphEditorTargetId();
      if (!id) {
        $q.notify({ type: 'warning', message: t('teamsPage.graphAssetMissingNotify') });
        return;
      }
      await router.push({ name: 'graph-editor', params: { id } });
    } finally {
      openingGraphEditor.value = false;
    }
  }

  onMounted(reload);
  watch(teamId, reload);
  onBeforeUnmount(disconnectLiveRun);

  return {
    isDark,
    teamId,
    loading,
    error,
    teamRow,
    compiled,
    definition,
    readOnly,
    selectedNodeId,
    activeRun,
    observatory,
    liveConnected,
    liveMode,
    nodeList,
    selectedLiveState,
    execNodeStates,
    graphDef,
    issues,
    reload,
    onSelectNode,
    openObservatory,
    openingGraphEditor,
    openInGraphEditor,
    goBack,
  };
}
