import { computed, ref, watch } from "vue";
import { isAgentEvolving } from "../../components/agents/agentUi";
import type { Agent } from "./types";
import type { AgentRuntimeConfigForm } from "./agentRuntimeConfig";

/** Evolution tab range + header chip + self_evolve bidirectional sync with runtime config. */
export function useAgentEvolutionSettings(form: Agent, config: AgentRuntimeConfigForm) {
  const evolutionRange = ref("30d");
  const showEvolving = computed(() => isAgentEvolving(form));

  watch(
    () => form.system_prompt_mode,
    () => {
      config.evolution.self_evolve = config.self_evolve;
    },
  );

  watch(
    () => config.evolution.self_evolve,
    (value) => {
      config.self_evolve = value;
    },
  );

  return { evolutionRange, showEvolving };
}
