import { describe, it, expect, vi, beforeEach } from "vitest";
import { setActivePinia, createPinia } from "pinia";
import { useAppStore } from "../app";
import { useChatStore } from "../chat";

vi.mock("../../features/session/api", () => ({
  listSessions: vi.fn().mockResolvedValue([]),
  createSession: vi.fn().mockResolvedValue({ id: "sess-1", title: "Test Session" }),
  deleteSession: vi.fn().mockResolvedValue(undefined),
  clearAgentSessions: vi.fn().mockResolvedValue(undefined),
  updateSessionTitle: vi.fn().mockResolvedValue({ id: "sess-1", title: "Renamed" }),
  listSessionChatMessages: vi.fn().mockResolvedValue({ items: [], currentRevision: 0 }),
  listTeamSessions: vi.fn().mockResolvedValue([]),
}));

vi.mock("../../features/agents/api", () => ({
  listAgents: vi.fn().mockResolvedValue([
    { id: "agent-1", agent_key: "test-agent", display_name: "Test Agent" }
  ]),
  createAgent: vi.fn().mockResolvedValue({ id: "agent-2", agent_key: "new", display_name: "New" }),
  updateAgent: vi.fn().mockImplementation((_id, patch) => Promise.resolve(patch)),
  deleteAgent: vi.fn().mockResolvedValue(undefined)
}));

describe("useAppStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("initialises with empty state", () => {
    const store = useAppStore();
    expect(store.agents).toEqual([]);
    expect(store.selectedAgent).toBeNull();
  });

  it("loadAgents populates agents and selects the first", async () => {
    const store = useAppStore();
    await store.loadAgents();
    expect(store.agents).toHaveLength(1);
    expect(store.selectedAgent?.id).toBe("agent-1");
  });

  it("upsertAgent updates existing agent in-place", async () => {
    const store = useAppStore();
    await store.loadAgents();
    store.upsertAgent({ id: "agent-1", agent_key: "test-agent", display_name: "Updated" } as any);
    expect(store.agents[0].display_name).toBe("Updated");
  });

  it("removeAgentFromList removes and clears selection", async () => {
    const store = useAppStore();
    const chat = useChatStore();
    await store.loadAgents();
    await store.removeAgentFromList("agent-1");
    expect(store.agents).toHaveLength(0);
    expect(store.selectedAgent).toBeNull();
    expect(chat.sessions).toEqual([]);
  });

  it("addAgent creates agent and prepends to list", async () => {
    const store = useAppStore();
    await store.loadAgents();
    const created = await store.addAgent({ agent_key: "new", display_name: "New" } as any);
    expect(created?.id).toBe("agent-2");
    expect(store.agents[0].id).toBe("agent-2");
    expect(store.selectedAgent?.id).toBe("agent-2");
  });
});

describe("useChatStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("stores messages by session id", async () => {
    const chat = useChatStore();
    chat.setMessages("sess-1", [{ id: "m1" } as any]);
    expect(chat.getMessages("sess-1")).toHaveLength(1);
    (chat as any).selectedSession = { id: "sess-1" } as any;
    expect(chat.messages).toHaveLength(1);
  });

  it("tracks ws connected per session", () => {
    const chat = useChatStore();
    chat.setWsConnected("sess-1", true);
    (chat as any).selectedSession = { id: "sess-1" } as any;
    (chat as any).entityKind = "agent";
    expect(chat.wsConnected).toBe(true);
  });

  it("clearTeamSessions only removes that team session caches", () => {
    const chat = useChatStore();
    chat.teamSessions["team-1"] = [{ id: "t-s1", at: "" } as any, { id: "t-s2", at: "" } as any];
    chat.teamSessions["team-2"] = [{ id: "t-s3", at: "" } as any];
    chat.setMessages("t-s1", [{ id: "m1" } as any]);
    chat.setMessages("t-s2", [{ id: "m2" } as any]);
    chat.setMessages("t-s3", [{ id: "m3" } as any]);
    chat.setMessages("agent-s1", [{ id: "m4" } as any]);
    (chat as any).teamSelectedSessionId = "t-s1";

    chat.clearTeamSessions("team-1");

    expect(chat.teamSessions["team-1"]).toEqual([]);
    expect(chat.teamSelectedSessionId).toBeNull();
    expect(chat.getMessages("t-s1")).toEqual([]);
    expect(chat.getMessages("t-s2")).toEqual([]);
    expect(chat.getMessages("t-s3")).toHaveLength(1);
    expect(chat.getMessages("agent-s1")).toHaveLength(1);
  });

  it("clearAllAgentSessions only clears that agent session caches", async () => {
    const { clearAgentSessions } = await import("../../features/session/api");
    const chat = useChatStore();
    (chat as any).sessions = [
      { id: "agent-s1", title: "A" } as any,
      { id: "agent-s2", title: "B" } as any,
    ];
    chat.setMessages("agent-s1", [{ id: "m1" } as any]);
    chat.setMessages("agent-s2", [{ id: "m2" } as any]);
    chat.setMessages("team-s1", [{ id: "m3" } as any]);
    chat.sessionRevisionBySession["agent-s1"] = 3;
    chat.wsConnectedBySession["agent-s2"] = true;

    await chat.clearAllAgentSessions("agent-1");

    expect(clearAgentSessions).toHaveBeenCalledWith("agent-1");
    expect(chat.sessions).toEqual([]);
    expect(chat.selectedSession).toBeNull();
    expect(chat.getMessages("agent-s1")).toEqual([]);
    expect(chat.getMessages("agent-s2")).toEqual([]);
    expect(chat.sessionRevisionBySession["agent-s1"]).toBeUndefined();
    expect(chat.wsConnectedBySession["agent-s2"]).toBeUndefined();
    expect(chat.getMessages("team-s1")).toHaveLength(1);
  });

  it("removeTeamSessionLocal deletes session and clears caches", async () => {
    const { deleteSession } = await import("../../features/session/api");
    const chat = useChatStore();
    chat.teamSessions["team-1"] = [
      { id: "t-s1", at: "" } as any,
      { id: "t-s2", at: "" } as any,
    ];
    (chat as any).teamSelectedSessionId = "t-s1";
    chat.setMessages("t-s1", [{ id: "m1" } as any]);
    chat.sessionRevisionBySession["t-s1"] = 2;
    chat.wsConnectedBySession["t-s1"] = true;

    await chat.removeTeamSessionLocal("team-1", "t-s1");

    expect(deleteSession).toHaveBeenCalledWith("t-s1");
    expect(chat.teamSessions["team-1"]).toHaveLength(1);
    expect(chat.teamSessions["team-1"]![0].id).toBe("t-s2");
    expect(chat.teamSelectedSessionId).toBe("t-s2");
    expect(chat.getMessages("t-s1")).toEqual([]);
    expect(chat.sessionRevisionBySession["t-s1"]).toBeUndefined();
    expect(chat.wsConnectedBySession["t-s1"]).toBeUndefined();
  });
});
