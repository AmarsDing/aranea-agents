import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { compiledGraphToGraphDef } from "../orchestration/compileApi";
import type { CompileTeamGraphResult } from "../orchestration/compileApi";
import type { GraphDefinition } from "../graph/types";
import type { Team, TeamDefinition, TeamRun } from "./types";
import { findActiveTeamRun } from "./api";
import { parseDefinition, definitionToJSON, withGraph } from "../../components/teams/teamUtils";
import { useTeamsStore } from "../../stores/teams";
import { useOrchestrationStore } from "../../stores/orchestration";
import { useOrchestrationStream } from "../orchestration/useOrchestrationStream";
import { buildExecNodeStates } from "../orchestration/teamGraphAdapter";
import type { AgentNodeState, TeamRunObservatory } from "../orchestration/types";

export function useTeamOrchestratePage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const teamsStore = useTeamsStore();
  const orchestrationStore = useOrchestrationStore();

  const isDark = computed(() => $q.dark.isActive);
  const teamId = computed(() => String(route.params.teamId ?? ""));

  const loading = ref(true);
  const saving = ref(false);
  const dirty = ref(false);
  const error = ref("");
  const teamRow = ref<Team | null>(null);
  const compiled = ref<CompileTeamGraphResult | null>(null);
  const linkedGraphId = ref("");
  const definition = ref<TeamDefinition | null>(null);
  const readOnly = ref(false);

  const selectedNodeId = ref<string | null>(null);
  const activeRun = ref<TeamRun | null>(null);
  const observatory = ref<TeamRunObservatory | null>(null);
  const stream = ref<ReturnType<typeof useOrchestrationStream> | null>(null);

  const graphDef = reactive<GraphDefinition>({
    id: "",
    name: "",
    description: "",
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: "",
    finishPoint: "",
    enableCheckpoint: false,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    createdAt: "",
    updatedAt: ""
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
    () => graphDef.nodes.map((n) => `${n.id}:${n.type}`).join("\0"),
    () => {
      const map = new Map<string, { status: string; fineStatus?: string }>();
      for (const node of graphDef.nodes) {
        if (node.type === "agent") {
          map.set(node.id, { status: "waiting", fineStatus: "idle" });
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

  function markDirty() {
    if (readOnly.value) return;
    dirty.value = true;
  }

  function patchDefinition(patch: Partial<TeamDefinition>) {
    if (!definition.value || readOnly.value) return;
    definition.value = { ...definition.value, ...patch };
    if (patch.failure_policy) {
      definition.value.failure_policy = {
        ...(definition.value.failure_policy ?? {}),
        ...patch.failure_policy,
      };
    }
    markDirty();
  }

  function applyCompiled(result: CompileTeamGraphResult) {
    compiled.value = result;
    Object.assign(graphDef, compiledGraphToGraphDef(result, teamRow.value?.display_name || "team-orchestration"));
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
      const message = e instanceof Error ? e.message : "连接运行观测流失败";
      $q.notify({ type: "warning", message });
    }
  }

  async function reload() {
    loading.value = true;
    error.value = "";
    try {
      const team = await teamsStore.fetchTeam(teamId.value);
      teamRow.value = team;
      readOnly.value = Boolean(team.has_active_run);
      definition.value = parseDefinition(team);
      linkedGraphId.value = team.linked_graph_id || readLinkedGraphId(definition.value);
      const result = await orchestrationStore.compileTeam(teamId.value);
      applyCompiled(result);
      dirty.value = false;
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

  function readLinkedGraphId(def: TeamDefinition & { linked_graph_id?: string }) {
    return String((def as { linked_graph_id?: string }).linked_graph_id ?? "");
  }

  function graphToDefinitionGraph() {
    const def = definition.value;
    if (!def) return undefined;
    const priorById = new Map((def.graph?.nodes ?? []).map((node) => [node.id, node]));
    return {
      version: 1,
      layout: def.mode,
      nodes: graphDef.nodes.map((n) => {
        const prior = priorById.get(n.id);
        return {
          id: n.id,
          type: n.type,
          label: n.agentName || n.id,
          agent_id: prior?.agent_id || n.agentName || "",
          role: n.requiredRole || prior?.role,
          x: prior?.x,
          y: prior?.y,
        };
      }),
      edges: graphDef.edges.map((e, index) => ({
        id: priorById.has(e.from) ? `e-${e.from}-${e.to}` : `e-${index}`,
        source: e.from,
        target: e.to,
      })),
    };
  }

  async function saveGraph() {
    if (!teamRow.value || !definition.value || readOnly.value) return;
    saving.value = true;
    try {
      const nextDef = withGraph({
        ...definition.value,
        graph: graphToDefinitionGraph(),
        linked_graph_id: linkedGraphId.value.trim() || undefined
      } as TeamDefinition & { linked_graph_id?: string });
      const updated = await teamsStore.editTeam(teamId.value, {
        definition_json: definitionToJSON(nextDef),
        linked_graph_id: linkedGraphId.value.trim()
      });
      teamRow.value = updated;
      definition.value = parseDefinition(updated);
      dirty.value = false;
      $q.notify({ type: "positive", message: "编排 graph 已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    } finally {
      saving.value = false;
    }
  }

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
  }

  function openObservatory() {
    if (!activeRun.value) return;
    router.push({
      name: "team-run-observatory",
      params: { teamId: teamId.value, runId: activeRun.value.id },
    });
  }

  function goBack() {
    router.push({ name: "team" });
  }

  onMounted(reload);
  watch(teamId, reload);
  onBeforeUnmount(disconnectLiveRun);

  return {
    isDark,
    teamId,
    loading,
    saving,
    dirty,
    error,
    teamRow,
    compiled,
    linkedGraphId,
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
    markDirty,
    patchDefinition,
    reload,
    saveGraph,
    onSelectNode,
    openObservatory,
    goBack
  };
}
