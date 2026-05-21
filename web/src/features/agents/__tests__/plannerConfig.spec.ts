import { describe, expect, it } from "vitest";
import {
  defaultPlannerForm,
  plannerFormFromSettings,
  serializePlannerForm,
  validatePlannerForm,
} from "../plannerConfig";

describe("plannerConfig", () => {
  it("round-trips builtin config", () => {
    const form = plannerFormFromSettings(
      "builtin",
      '{"reasoning_effort":"high","thinking_enabled":true,"thinking_tokens":8192}'
    );
    expect(form.kind).toBe("builtin");
    expect(form.builtin.reasoning_effort).toBe("high");
    const out = serializePlannerForm(form);
    expect(out.planner_kind).toBe("builtin");
    const parsed = JSON.parse(out.planner_config_json);
    expect(parsed.reasoning_effort).toBe("high");
    expect(parsed.thinking_enabled).toBe(true);
    expect(parsed.thinking_tokens).toBe(8192);
  });

  it("react yields empty config object", () => {
    const form = defaultPlannerForm();
    form.kind = "react";
    expect(serializePlannerForm(form).planner_config_json).toBe("{}");
  });

  it("drops invalid reasoning_effort when hydrating from settings", () => {
    const form = plannerFormFromSettings("builtin", '{"reasoning_effort":"turbo"}');
    expect(form.builtin.reasoning_effort).toBe("");
  });

  it("rejects invalid reasoning_effort", () => {
    const form = defaultPlannerForm();
    form.kind = "builtin";
    form.builtin.reasoning_effort = "turbo";
    expect(validatePlannerForm(form)).toMatch(/reasoning_effort/);
  });

  it("rejects invalid a2ui schema json", () => {
    const form = defaultPlannerForm();
    form.kind = "a2ui";
    form.a2ui.client_to_server_schema_json = "[]";
    expect(validatePlannerForm(form)).toMatch(/JSON/);
  });
});
