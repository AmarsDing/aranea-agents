import { reactive } from "vue";
import type { AgentRuntimeSettings } from "./types";
import {
  defaultRalphLoopForm,
  ralphLoopFormFromSettings,
  serializeRalphLoopForm,
  validateRalphLoopForm,
  type RalphLoopFormState,
} from "./ralphLoopConfig";

/** Ralph Loop slice for Agent settings — form state, hydrate, validate, serialize. */
export function useAgentRalphLoopForm() {
  const form = reactive<RalphLoopFormState>(defaultRalphLoopForm());

  function hydrateFromSettings(s?: AgentRuntimeSettings | null) {
    Object.assign(form, ralphLoopFormFromSettings(s));
  }

  function validate(): string | null {
    return validateRalphLoopForm(form);
  }

  function serialize() {
    return serializeRalphLoopForm(form);
  }

  return { form, hydrateFromSettings, validate, serialize };
}
