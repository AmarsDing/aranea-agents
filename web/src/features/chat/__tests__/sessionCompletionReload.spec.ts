import { describe, expect, it, vi } from "vitest";
import { reloadSessionAfterCompletion } from "../sessionCompletionReload";

describe("reloadSessionAfterCompletion (DECO-R-P2-02)", () => {
  it("uses dropStaleInFlight for agent turns and refreshes sessions", async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadAgentSessions = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: "agent",
        selectedTeamId: "",
        loadAgentSessions,
        loadTeamSessions: vi.fn(),
      } as never,
      messageStore: {
        loadMessages,
      } as never,
      streamingSnapshots: { clear } as never,
      sessionId: "sess-1",
      resolveAgentId: () => "agent-1",
    });

    expect(loadMessages).toHaveBeenCalledWith({
      sessionId: "sess-1",
      dropStaleInFlight: true,
    });
    expect(clear).toHaveBeenCalledWith("sess-1");
    expect(loadAgentSessions).toHaveBeenCalledWith("agent-1", { refreshOnly: true });
  });

  it("uses the same dropStaleInFlight path for team turns", async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadTeamSessions = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: "team",
        selectedTeamId: "team-1",
        loadAgentSessions: vi.fn(),
        loadTeamSessions,
      } as never,
      messageStore: {
        loadMessages,
      } as never,
      streamingSnapshots: { clear } as never,
      sessionId: "sess-team",
    });

    expect(loadMessages).toHaveBeenCalledWith({
      sessionId: "sess-team",
      dropStaleInFlight: true,
    });
    expect(clear).toHaveBeenCalledWith("sess-team");
    expect(loadTeamSessions).toHaveBeenCalledWith("team-1");
  });
});
