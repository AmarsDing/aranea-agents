import { describe, expect, it } from "vitest";
import type { Envelope } from "../envelope";
import {
  envelopeSessionRevision,
  envelopeSource,
  isSessionRevisionSyncEnvelope,
  isTurnCompleteEnvelope,
} from "../inboundSyncEnvelope";
import { SESSION_RUN_STATUS } from "../sessionRunStatus";

function env(partial: Partial<Envelope>): Envelope {
  return {
    id: "e1",
    type: "run_status",
    author: "test",
    session_id: "sess-1",
    timestamp: "",
    version: 1,
    ...partial,
  };
}

describe("inboundSyncEnvelope (DECO-01 / M55-SYNC)", () => {
  it("reads session_revision from envelope root or metadata", () => {
    expect(envelopeSessionRevision(env({ session_revision: 3 }))).toBe(3);
    expect(
      envelopeSessionRevision(env({ metadata: { session_revision: 5 } }))
    ).toBe(5);
    expect(envelopeSessionRevision(env({}))).toBe(0);
  });

  it("reads channel source for cross-plane routing", () => {
    expect(envelopeSource(env({ source: "channel" }))).toBe("channel");
    expect(envelopeSource(env({ metadata: { source: "channel" } }))).toBe(
      "channel"
    );
  });

  it("treats sync run_status as hydrate-only, not turn complete", () => {
    const syncEnv = env({
      source: "channel",
      metadata: { status: SESSION_RUN_STATUS.SYNC, run_id: "run-1" },
      session_revision: 2,
    });
    expect(isSessionRevisionSyncEnvelope(syncEnv)).toBe(true);
    expect(isTurnCompleteEnvelope(syncEnv)).toBe(false);
  });

  it("treats completed channel turn as turn complete", () => {
    const done = env({
      source: "channel",
      metadata: { status: SESSION_RUN_STATUS.COMPLETED, run_id: "run-1" },
      session_revision: 3,
    });
    expect(isSessionRevisionSyncEnvelope(done)).toBe(false);
    expect(isTurnCompleteEnvelope(done)).toBe(true);
  });

  it("runner_completion always completes turn", () => {
    expect(
      isTurnCompleteEnvelope(env({ type: "runner_completion" }))
    ).toBe(true);
  });
});
