import { reactive } from 'vue';
import {
  defaultPlannerForm,
  plannerFormFromSettings,
  serializePlannerForm,
  validatePlannerForm,
  type PlannerFormState,
} from './plannerConfig';

/** Planner slice for Agent settings — form state, hydrate, validate, serialize. */
export function useAgentPlannerForm() {
  const form = reactive<PlannerFormState>(defaultPlannerForm());

  function hydrateFromSettings(plannerKind?: string, plannerConfigJson?: string) {
    Object.assign(form, plannerFormFromSettings(plannerKind, plannerConfigJson));
  }

  function validate(): string | null {
    return validatePlannerForm(form);
  }

  function serialize() {
    return serializePlannerForm(form);
  }

  return { form, hydrateFromSettings, validate, serialize };
}
