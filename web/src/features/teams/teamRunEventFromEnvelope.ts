import type { Envelope, EnvelopeType } from "../chat/envelope";
import type { TeamRun, TeamRunEvent, TeamRunStep } from "./types";

/** Envelope types emitted on the team / monitor channels for Team run observability. */
export const TEAM_RUNTIME_ENVELOPE_TYPES: EnvelopeType[] = [
  "team_run_started",
  "team_run_finished",
  "team_run_failed",
  "team_summary",
  "team_step_started",
  "team_step_finished",
  "intent_pass",
  "transfer",
  "runner_completion",
];

function pickRun(meta: Record<string, unknown>): TeamRun | undefined {
  const run = meta.run;
  if (run && typeof run === "object") {
    return run as TeamRun;
  }
  return undefined;
}

function pickStep(meta: Record<string, unknown>): TeamRunStep | undefined {
  const step = meta.step;
  if (step && typeof step === "object") {
    return step as TeamRunStep;
  }
  return undefined;
}

/** Map a WS Envelope to {@link TeamRunEvent}; returns null for unrelated types. */
export function teamRunEventFromEnvelope(env: Envelope, defaultTeamID = ""): TeamRunEvent | null {
  const meta = (env.metadata ?? {}) as Record<string, unknown>;
  const teamId = env.team_id ?? defaultTeamID;
  const runId = String(meta.run_id ?? "");

  switch (env.type) {
    case "team_run_started":
    case "team_run_finished":
    case "team_run_failed":
      return {
        type: env.type,
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        run: pickRun(meta),
        payload: meta,
      };
    case "team_summary":
      return {
        type: "team_summary",
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        run: pickRun(meta),
        payload: meta,
      };
    case "team_step_started":
    case "team_step_started":
    case "team_step_finished":
      return {
        type: env.type,
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        step: pickStep(meta),
        payload: meta,
      };
    case "intent_pass":
      return {
        type: "intent_pass",
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        payload: meta,
      };
    case "transfer":
      return {
        type: "transfer",
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        payload: {
          from_agent: env.transfer?.from_agent,
          to_agent: env.transfer?.to_agent,
        },
      };
    case "runner_completion":
      return {
        type: "run_finished",
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        payload: meta,
      };
    case "log": {
      const eventType = String(meta.event_type ?? "");
      if (!eventType) {
        return null;
      }
      return {
        type: eventType,
        team_id: teamId,
        run_id: runId,
        session_id: env.session_id,
        payload: meta,
      };
    }
    default:
      return null;
  }
}
