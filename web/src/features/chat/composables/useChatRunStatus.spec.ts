import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { useChatRunStatus } from "./useChatRunStatus";

vi.mock("../api", () => ({
  getRunStatus: vi.fn(),
}));

import { getRunStatus } from "../api";

describe("useChatRunStatus", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(getRunStatus).mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("prefers WS envelope over delayed HTTP hydrate", async () => {
    vi.mocked(getRunStatus).mockResolvedValue({
      status: "running",
      runId: "http-run",
      errorMessage: "",
      updatedAt: "",
    });

    const applyAwaitRunStatus = vi.fn();
    const { runStatus, applyFromEnvelope, onSessionSwitch } = useChatRunStatus({
      applyAwaitRunStatus,
    });

    onSessionSwitch("sess-1");
    applyFromEnvelope({
      id: "e1",
      type: "run_status",
      author: "test",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      metadata: { status: "completed", run_id: "ws-run" },
    });

    expect(runStatus.value).toBe("completed");
    vi.advanceTimersByTime(500);
    await Promise.resolve();
    expect(getRunStatus).not.toHaveBeenCalled();
  });

  it("hydrates from HTTP when WS silent after session switch", async () => {
    vi.mocked(getRunStatus).mockResolvedValue({
      status: "awaiting_user",
      runId: "run-1",
      errorMessage: "",
      updatedAt: "",
      awaitKind: "reply",
    });

    const applyAwaitRunStatus = vi.fn();
    const { runStatus, onSessionSwitch } = useChatRunStatus({ applyAwaitRunStatus });

    onSessionSwitch("sess-2");
    vi.advanceTimersByTime(400);
    await Promise.resolve();
    await Promise.resolve();

    expect(getRunStatus).toHaveBeenCalledWith("sess-2");
    expect(runStatus.value).toBe("awaiting_user");
    expect(applyAwaitRunStatus).toHaveBeenCalled();
  });
});
