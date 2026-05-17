import { describe, it, expect, vi, beforeEach } from "vitest";
import { setActivePinia, createPinia } from "pinia";
import { useAppStore } from "../app";

// Mock all external API calls so no real HTTP happens.
vi.mock("../../features/session/api", () => ({
  listSessions: vi.fn().mockResolvedValue([]),
  createSession: vi.fn().mockResolvedValue({ id: "sess-1", title: "Test Session" }),
  deleteSession: vi.fn().mockResolvedValue(undefined),
  clearAgentSessions: vi.fn().mockResolvedValue(undefined),
  updateSessionTitle: vi.fn().mockResolvedValue({ id: "sess-1", title: "Renamed" }),
  listSessionChatMessages: vi.fn().mockResolvedValue([])
}));

vi.mock("../../features/chat/api", () => ({
  sendMessage: vi.fn().mockResolvedValue({ user_message: null, agent_message: null })
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
    expect(store.sessions).toEqual([]);
    expect(store.messages).toEqual([]);
    expect(store.selectedAgent).toBeNull();
    expect(store.selectedSession).toBeNull();
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

  it("upsertAgent inserts new agent at front", async () => {
    const store = useAppStore();
    await store.loadAgents();
    store.upsertAgent({ id: "agent-99", agent_key: "z", display_name: "Z Agent" } as any);
    expect(store.agents).toHaveLength(2);
    expect(store.agents[0].id).toBe("agent-99");
  });

  it("removeAgentFromList removes and clears selection", async () => {
    const store = useAppStore();
    await store.loadAgents();
    await store.removeAgentFromList("agent-1");
    expect(store.agents).toHaveLength(0);
    expect(store.selectedAgent).toBeNull();
  });

  it("addAgent creates agent and prepends to list", async () => {
    const store = useAppStore();
    await store.loadAgents();
    const created = await store.addAgent({ agent_key: "new", display_name: "New" } as any);
    expect(created?.id).toBe("agent-2");
    expect(store.agents[0].id).toBe("agent-2");
    expect(store.selectedAgent?.id).toBe("agent-2");
  });

  it("addSession creates a session and prepends", async () => {
    const store = useAppStore();
    await store.loadAgents();
    const sess = await store.addSession("My Session");
    expect(sess?.id).toBe("sess-1");
    expect(store.sessions[0].id).toBe("sess-1");
    expect(store.selectedSession?.id).toBe("sess-1");
  });

  it("clearAllSessions resets sessions and selection", async () => {
    const store = useAppStore();
    await store.loadAgents();
    await store.addSession("S1");
    await store.clearAllSessions();
    expect(store.sessions).toHaveLength(0);
    expect(store.selectedSession).toBeNull();
  });
});
