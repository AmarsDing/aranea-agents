import { computed, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { compiledGraphToGraphDef } from "../orchestration/compileApi";
import type { CompileTeamGraphResult } from "../orchestration/compileApi";
import type { GraphDefinition } from "../graph/types";
import type { Team, TeamDefinition } from "./types";
import { parseDefinition, withGraph } from "../../components/teams/teamUtils";
import { useTeamsStore } from "../../stores/teams";
import { useOrchestrationStore } from "../../stores/orchestration";

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

  const execNodeStates = computed(() => new Map<string, { status: string }>());

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
    createdAt: "",
    updatedAt: ""
  });

  const issues = computed(() => compiled.value?.issues ?? []);

  function markDirty() {
    if (readOnly.value) return;
    dirty.value = true;
  }

  function applyCompiled(result: CompileTeamGraphResult) {
    compiled.value = result;
    Object.assign(graphDef, compiledGraphToGraphDef(result, teamRow.value?.display_name || "team-orchestration"));
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
    return {
      version: 1,
      layout: def.mode,
      nodes: graphDef.nodes.map((n, index) => ({
        id: n.id,
        type: n.type,
        label: n.agentName || n.id,
        agent_id: def.members[index]?.agent_id,
        role: n.requiredRole,
        x: 160 + index * 150,
        y: 80
      })),
      edges: graphDef.edges.map((e, index) => ({
        id: `e-${index}`,
        source: e.from,
        target: e.to
      }))
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
        definition_json: JSON.stringify(nextDef),
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

  function onSelectNode(_nodeId: string | null) {}

  function goBack() {
    router.push({ name: "team" });
  }

  onMounted(reload);
  watch(teamId, reload);

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
    execNodeStates,
    graphDef,
    issues,
    markDirty,
    reload,
    saveGraph,
    onSelectNode,
    goBack
  };
}
