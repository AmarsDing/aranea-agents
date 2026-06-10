/**
 * useAgentBlocks — Build AgentBlock tree from chat messages.
 *
 * As of P0 (proposal-execution-progress-inline), single agent sessions are
 * also represented as an AgentBlock tree (root agent only, no sub-agents).
 * Team/spirit sessions nest sub-agent blocks recursively.
 *
 * Algorithm:
 * 1. Group messages by user turn boundaries
 * 2. Within each turn, walk messages in chronological order
 * 3. Build AgentBlock with timeline entries in STRICT execution order:
 *    thinking → tool → thinking → subagent → thinking → result
 *    (interleaved by message position, NOT grouped by type)
 * 4. Root agent is the top-level block; sub-agents are embedded as timeline entries
 *    at their chronological position
 * 5. Extract orchestration plan from plan_and_execute / subagents_spawn tool calls
 * 6. Compute team status summary from sub-agent statuses
 * 7. Merge execution_progress envelopes (if provided) as inline progress cards
 *    in the timeline, ordered by startedAt relative to other entries.
 */
import { computed, type ComputedRef } from 'vue';
import type { Message } from '../types';
import type { ToolUseEvent } from '../types';
import type { Envelope } from '../../../realtime/envelope';
import type {
  AgentBlock,
  TimelineEntry,
  ToolSection,
  AgentBlockStatus,
  OrchestrationPlan,
  PlanEntry,
  TeamStatusSummary,
  ProgressSection,
} from '../agentTreeTypes';
import { agentColorFromKey, ROOT_AGENT_KEY } from '../agentTreeTypes';
import { toolEventFromMessage } from '../envelopeToolCall';
import { resolveAssistantPresentation, type AssistantPresentation } from '../messagePlannerPresentation';
import { resolveDisplayLabel } from '../activityPresentation';
import { isTeamMemberOrigin, ensureOrigin } from '../messageOrigin';
import { useTurnBlockEnabled } from '../useTurnBlock';
import { canonicalToolStatus } from '../lib/statusMap';
import { mergeProgressEvents } from '../executionProgress';

export function useAgentBlocks(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
  /**
   * Ordered execution_progress envelopes. Optional. When provided, each turn
   * timeline gets inline progress cards merged in chronological order.
   */
  progressEnvelopes?: ComputedRef<readonly Envelope[]>;
}) {
  const useTurnBlockMode = computed(() => useTurnBlockEnabled());

  /**
   * Build AgentBlock tree from turn blocks. Active for all sessions in P0
   * (single agent also gets the tree view). Returns an empty array only when
   * turn block mode is disabled (legacy ChatMessageList path) or there are no
   * messages at all.
   */
  const agentBlocks = computed((): AgentBlock[] => {
    if (!useTurnBlockMode.value) return [];

    const allMessages = deps.messages.value;
    if (allMessages.length === 0) return [];

    // P0: drop the team-only gate. Single agent sessions still produce a root
    // AgentBlock so they can use the unified tree timeline.
    const plannerKind = deps.plannerKind?.value ?? '';
    const progressByStep = deps.progressEnvelopes?.value
      ? mergeProgressEvents(deps.progressEnvelopes.value)
      : new Map<string, ProgressSection>();
    const blocks: AgentBlock[] = [];

    // Find user messages as turn boundaries
    const userTurns = findUserTurns(allMessages);

    for (const turn of userTurns) {
      const rootBlock = buildRootAgentBlock(turn, plannerKind, progressByStep);
      if (rootBlock) blocks.push(rootBlock);
    }

    return blocks;
  });

  return {
    agentBlocks,
  };
}

// ── Turn detection ──

/** Tool names that indicate team/orchestration activity */
const TEAM_TOOL_NAMES = new Set([
  'subagents_spawn',
  'subagents_get',
  'subagents_wait',
  'subagents_send',
  'subagents_kill',
  'plan_and_execute',
  'todo_manage',
  'todo_write',
  'transfer_to_agent',
  'agent_transfer',
  'spawn_agent',
  'create_team',
  'team_run',
]);

function isTeamToolName(name: string): boolean {
  if (!name) return false;
  return TEAM_TOOL_NAMES.has(name);
}

interface UserTurn {
  userMessage: Message | null;
  /** All messages between this user message and the next (or end) */
  messages: Message[];
}

function findUserTurns(messages: Message[]): UserTurn[] {
  if (messages.length === 0) return [];

  const ensured = messages.map((m) => ensureOrigin(m));
  const turns: UserTurn[] = [];
  let current: UserTurn = { userMessage: null, messages: [] };

  for (const msg of ensured) {
    if (msg.role === 'user') {
      // Start a new turn
      if (current.messages.length > 0 || current.userMessage) {
        turns.push(current);
      }
      current = { userMessage: msg, messages: [msg] };
    } else {
      current.messages.push(msg);
    }
  }

  if (current.messages.length > 0 || current.userMessage) {
    turns.push(current);
  }

  return turns;
}

// ── Chronological timeline builder ──

/**
 * Represents a timestamped event that can be placed on the timeline.
 * We walk all messages in order and classify each into one of these categories,
 * then sort by position to get strict chronological interleaving.
 */
interface TimestampedEvent {
  /** Original message position for sorting */
  position: number;
  /** Message reference */
  message: Message;
  /** Classification */
  type: 'root-thinking' | 'root-tool' | 'team-tool' | 'member-thinking' | 'member-tool' | 'member-content';
  /** For tool events */
  toolEvent: ToolUseEvent | null;
  /** Sub-agent key for member/tool messages */
  agentKey: string | null;
}

function classifyMessages(msgs: Message[]): TimestampedEvent[] {
  const events: TimestampedEvent[] = [];

  // First pass: identify all sub-agent keys
  const subAgentKeys = new Set<string>();
  for (const m of msgs) {
    if (isTeamMemberOrigin(m.origin)) {
      const key = getMemberAgentKey(m);
      if (key) subAgentKeys.add(key);
    }
    const ev = toolEventFromMessage(m);
    if (ev && ev.agent_key && ev.agent_key !== ROOT_AGENT_KEY) {
      subAgentKeys.add(ev.agent_key);
    }
  }

  // Second pass: classify each message
  for (let i = 0; i < msgs.length; i++) {
    const m = msgs[i];
    const ev = toolEventFromMessage(m);

    if (m.role === 'user') continue;

    if (isTeamMemberOrigin(m.origin)) {
      const key = getMemberAgentKey(m);
      if (ev) {
        events.push({ position: i, message: m, type: 'member-tool', toolEvent: ev, agentKey: key });
      } else if (m.reasoning_markdown?.trim()) {
        events.push({ position: i, message: m, type: 'member-thinking', toolEvent: null, agentKey: key });
      } else if (m.content_markdown?.trim()) {
        events.push({ position: i, message: m, type: 'member-content', toolEvent: null, agentKey: key });
      }
    } else if (ev) {
      // Tool event from root agent or unknown agent
      if (ev.agent_key && subAgentKeys.has(ev.agent_key)) {
        events.push({ position: i, message: m, type: 'member-tool', toolEvent: ev, agentKey: ev.agent_key });
      } else if (isTeamToolName(ev.tool_name)) {
        events.push({ position: i, message: m, type: 'team-tool', toolEvent: ev, agentKey: null });
      } else {
        events.push({ position: i, message: m, type: 'root-tool', toolEvent: ev, agentKey: null });
      }
    } else if (m.role === 'assistant') {
      events.push({ position: i, message: m, type: 'root-thinking', toolEvent: null, agentKey: null });
    }
  }

  return events;
}

// ── Agent block building ──

function buildRootAgentBlock(
  turn: UserTurn,
  plannerKind: string,
  progressByStep: Map<string, ProgressSection>,
): AgentBlock | null {
  const msgs = turn.messages;
  if (msgs.length === 0 && !turn.userMessage) return null;

  const assistantMsgs = msgs.filter(
    (m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin) && !toolEventFromMessage(m),
  );
  const assistant = assistantMsgs[0] || null;

  const agentKey = assistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = assistant?.agent_ref?.name || '精灵助手';
  const agentIcon = assistant?.agent_ref?.icon || '精';

  // Classify all messages into timestamped events for chronological interleaving
  const events = classifyMessages(msgs);

  // Build timeline entries in STRICT chronological order
  const timeline: TimelineEntry[] = [];
  let sortCounter = 0;

  // Track sub-agent blocks being built (keyed by agentKey)
  const subAgentBuilders = new Map<string, SubAgentBuilder>();

  // Track team tool events for orchestration plan extraction
  const planEntries: PlanEntry[] = [];
  let planStatus: OrchestrationPlan['status'] = 'planning';

  // Process events in chronological order
  for (const event of events) {
    switch (event.type) {
      case 'root-thinking': {
        // P1 (reply-chronological): each assistant message emits (a) thinking
        // entries from ReAct steps / reasoning_markdown AND (b) one
        // chronological reply entry from the message's final answer. Reply
        // entries are pushed at the message's chronological position so the
        // timeline becomes thinking → tool → reply → thinking → reply rather
        // than 1:1-pairing paragraphs of a single collapsed result.
        const messagePresentation = resolveAssistantPresentation(plannerKind, event.message);
        const steps = messagePresentation.reactSteps?.steps;

        if (steps?.length) {
          // ReAct mode: emit one thinking entry per thinking-kind step
          const seenIds = new Set<string>();
          for (const step of steps) {
            const isThinking = step.kind === 'planning' || step.kind === 'reasoning' || step.kind === 'replanning';
            if (isThinking && step.body?.trim()) {
              const bodyHash = step.body.slice(0, 32).replace(/\s+/g, '');
              const stableId = `root-think-${event.message.id}-${step.kind}-${bodyHash}`;
              if (seenIds.has(stableId)) continue;
              seenIds.add(stableId);

              timeline.push({
                kind: 'thinking',
                section: {
                  id: stableId,
                  content: step.body,
                  durationMs: 0,
                  collapsed: true,
                  streaming: event.message.status === 'streaming',
                },
                sortKey: sortCounter++,
              });
            }
          }
        } else if (messagePresentation.reasoning?.trim()) {
          // Non-react mode: use reasoning_markdown as the thinking section
          timeline.push({
            kind: 'thinking',
            section: {
              id: `root-think-${event.message.id}`,
              content: messagePresentation.reasoning,
              durationMs: 0,
              collapsed: true,
              streaming: event.message.status === 'streaming',
            },
            sortKey: sortCounter++,
          });
        }

        // Emit a reply entry from this message's final answer (in-place at
        // this message's chronological position). Skip if the reply would
        // duplicate the last thinking entry (avoids "thinking then echo the
        // same content as reply" artifacts when the LLM only has reasoning
        // and no final-answer tag).
        const replyContent = resolveReplyContent(messagePresentation);
        if (replyContent) {
          const lastThinking = [...timeline].reverse().find((e) => e.kind === 'thinking');
          const lastThinkingContent = lastThinking?.kind === 'thinking' ? lastThinking.section.content.trim() : '';
          if (replyContent !== lastThinkingContent) {
            timeline.push({
              kind: 'reply',
              section: {
                id: `root-reply-${event.message.id}`,
                content: replyContent,
                durationMs: 0,
                streaming: event.message.status === 'streaming',
              },
              sortKey: sortCounter++,
            });
          }
        }
        break;
      }

      case 'root-tool': {
        if (event.toolEvent) {
          timeline.push({
            kind: 'tool',
            section: buildToolSection(event.toolEvent, 'root-tool'),
            sortKey: sortCounter++,
          });
        }
        break;
      }

      case 'team-tool': {
        if (!event.toolEvent) break;

        // Team orchestration tools: extract plan and/or create subagent entries
        const toolEv = event.toolEvent;

        if (toolEv.tool_name === 'plan_and_execute') {
          // Extract orchestration plan from plan_and_execute arguments
          const args = asObject(toolEv.arguments);
          const tasks = Array.isArray(args.tasks) ? args.tasks : Array.isArray(args.steps) ? args.steps : [];
          for (let i = 0; i < tasks.length; i++) {
            const t = asObject(tasks[i]);
            const task = String(t.task || t.description || t.prompt || `步骤 ${i + 1}`);
            const agentName = String(t.agent_name || t.agent || t.assignee || '');
            planEntries.push({
              id: `plan-${i}`,
              task,
              agentName: agentName || null,
              agentIcon: agentName ? agentName.charAt(0) : null,
              agentColor: agentName ? agentColorFromKey(agentName) : null,
              status: 'pending',
            });
          }
          planStatus = 'executing';

          // Also add as a tool entry in timeline
          timeline.push({
            kind: 'tool',
            section: buildToolSection(toolEv, 'root-plan'),
            sortKey: sortCounter++,
          });
        } else if (
          isTeamToolName(toolEv.tool_name) &&
          toolEv.tool_name !== 'subagents_get' &&
          toolEv.tool_name !== 'subagents_wait'
        ) {
          // Spawn-like tool: create subagent entry at this chronological position
          const subBlock = buildSubAgentFromSpawn(toolEv, msgs);
          if (subBlock) {
            // Add to plan entries if not already there
            if (!planEntries.some((p) => p.task === subBlock.task)) {
              planEntries.push({
                id: `plan-${planEntries.length}`,
                task: subBlock.task || subBlock.agentName,
                agentName: subBlock.agentName,
                agentIcon: subBlock.agentIcon,
                agentColor: subBlock.agentColor,
                status:
                  subBlock.status === 'completed' ? 'completed' : subBlock.status === 'failed' ? 'failed' : 'running',
              });
            }

            timeline.push({
              kind: 'subagent',
              block: subBlock,
              sortKey: sortCounter++,
            });
          } else {
            // Fallback: show as tool entry
            timeline.push({
              kind: 'tool',
              section: buildToolSection(toolEv, 'root-team-tool'),
              sortKey: sortCounter++,
            });
          }
        } else {
          // subagents_get / subagents_wait: show as tool entry
          timeline.push({
            kind: 'tool',
            section: buildToolSection(toolEv, 'root-team-tool'),
            sortKey: sortCounter++,
          });
        }
        break;
      }

      case 'member-thinking': {
        // Add thinking to the sub-agent builder
        if (event.agentKey) {
          const builder = getOrCreateBuilder(subAgentBuilders, event.agentKey, event.message, event.position);
          builder.addThinking(event.message);
        }
        break;
      }

      case 'member-tool': {
        if (event.agentKey && event.toolEvent) {
          const builder = getOrCreateBuilder(subAgentBuilders, event.agentKey, event.message, event.position);
          builder.addTool(event.toolEvent);
        }
        break;
      }

      case 'member-content': {
        if (event.agentKey) {
          const builder = getOrCreateBuilder(subAgentBuilders, event.agentKey, event.message, event.position);
          builder.addContent(event.message);
        }
        break;
      }
    }
  }

  // Now insert sub-agent blocks into the timeline at their first appearance position
  // Sub-agents that weren't created by a spawn tool need to be inserted
  const spawnBasedKeys = new Set<string>();
  for (const entry of timeline) {
    if (entry.kind === 'subagent') {
      spawnBasedKeys.add(entry.block.agentKey);
    }
  }

  // Build remaining sub-agent blocks from builders and insert at correct position
  for (const [agentKey, builder] of subAgentBuilders) {
    if (spawnBasedKeys.has(agentKey)) {
      // Update existing sub-agent block with additional timeline data
      const existingEntry = timeline.find((e) => e.kind === 'subagent' && e.block.agentKey === agentKey);
      if (existingEntry && existingEntry.kind === 'subagent') {
        // Merge builder data into existing block
        const builtBlock = builder.build();
        existingEntry.block.timeline = builtBlock.timeline;
        existingEntry.block.result = builtBlock.result || existingEntry.block.result;
        existingEntry.block.status = builtBlock.status;
        existingEntry.block.durationMs = builtBlock.durationMs;
        existingEntry.block.collapsed = builtBlock.status === 'completed';
        existingEntry.block.finishedAt = builtBlock.finishedAt;
      }
      continue;
    }

    // New sub-agent: insert at the position of its first event
    const block = builder.build();

    // Add to plan entries
    planEntries.push({
      id: `plan-${planEntries.length}`,
      task: block.task || block.agentName,
      agentName: block.agentName,
      agentIcon: block.agentIcon,
      agentColor: block.agentColor,
      status: block.status === 'completed' ? 'completed' : block.status === 'failed' ? 'failed' : 'running',
    });

    timeline.push({
      kind: 'subagent',
      block,
      sortKey: builder.firstPosition + 0.5, // Insert between existing entries
    });
  }

  // Sort timeline by sortKey to ensure correct order
  timeline.sort((a, b) => a.sortKey - b.sortKey);

  // Merge execution_progress sections. Each ProgressSection is assigned a
  // sortKey relative to its startedAt timestamp: convert epoch ms into a
  // fractional sort key that lands before any tool/thinking at the same
  // position. The mapping uses a reference start of the turn; since the
  // backend emits progress in real time before any tool_call/thinking
  // arrives, this naturally places progress nodes at the start of the
  // turn's timeline when they pre-date the LLM streaming.
  if (progressByStep.size > 0) {
    const turnStartTs = turn.userMessage?.created_at
      ? new Date(turn.userMessage.created_at).getTime()
      : Date.now();
    // Use a fractional sort key based on (startedAt - turnStartTs).
    // Negative values land before existing entries; positive values land
    // after. We bucket by millisecond and offset by 0.5 to keep
    // stability between equal timestamps.
    let progressSortBase = 0;
    for (const section of progressByStep.values()) {
      const offset = (section.startedAt - turnStartTs) / 1000; // seconds offset
      const key = offset - 0.5 + progressSortBase * 1e-6;
      progressSortBase += 1;
      timeline.push({
        kind: 'progress',
        section,
        sortKey: key,
      });
    }
    timeline.sort((a, b) => a.sortKey - b.sortKey);
  }

  // Determine status
  const isCompleted = !msgs.some(
    (m) => m.status === 'streaming' || m.status === 'tool_running' || m.status === 'tool_blocked',
  );
  const status = computeAgentStatus(
    assistant,
    msgs.filter((m) => toolEventFromMessage(m)),
    msgs.filter((m) => isTeamMemberOrigin(m.origin)),
    isCompleted,
  );

  // Calculate duration
  const durationMs = computeTurnDuration(turn);

  // Root agent result (final summary)
  // P1 (reply-chronological): each round emits its own reply entry in the
  // timeline. The legacy `result` field is now the content of the LAST reply
  // entry — it preserves the historical collapsed "root answer" affordance
  // (e.g. for screenshots, accessibility readers, or code that still reads
  // `block.result`) while no longer driving 1:1 pairing inside the timeline.
  // If no reply entry was emitted, fall back to the last assistant's
  // content_markdown so we never return null for a turn that did produce
  // a non-empty assistant message.
  let result: string | null = null;
  for (let i = timeline.length - 1; i >= 0; i--) {
    const entry = timeline[i];
    if (entry && entry.kind === 'reply') {
      result = entry.section.content?.trim() || null;
      break;
    }
  }
  if (!result) {
    result = assistant?.content_markdown?.trim() || null;
  }

  // Build orchestration plan
  const plan: OrchestrationPlan | null =
    planEntries.length > 0
      ? {
          entries: updatePlanEntryStatuses(planEntries, timeline),
          status: resolvePlanStatus(planStatus, status),
        }
      : null;

  // Build team status summary
  const teamStatus = computeTeamStatus(timeline);

  return {
    id: `root-${turn.userMessage?.id || 'no-user'}`,
    agentKey,
    agentName,
    agentIcon,
    agentColor: agentColorFromKey(agentKey),
    status,
    durationMs,
    collapsed: status === 'completed',
    task: turn.userMessage?.content_markdown || null,
    timeline,
    result,
    plan,
    teamStatus,
    startedAt: turn.userMessage?.created_at || assistant?.created_at || '',
    finishedAt: isCompleted ? assistant?.created_at || '' : null,
  };
}

/** Resolve the overall plan status based on planStatus and agent status.
 *  Fixes: planStatus stuck at 'planning' when subagents_spawn (not plan_and_execute)
 *  is used — in that case planEntries are added but planStatus never transitions
 *  to 'executing', so PlanCard keeps spinning forever.
 */
function resolvePlanStatus(
  planStatus: OrchestrationPlan['status'],
  agentStatus: AgentBlockStatus,
): OrchestrationPlan['status'] {
  // Explicit transitions from executing
  if (planStatus === 'executing') {
    if (agentStatus === 'completed') return 'completed';
    if (agentStatus === 'failed') return 'failed';
    return 'executing';
  }
  // If planStatus is still 'planning' but agent is done, the plan must be done too.
  // This happens when subagents_spawn creates plan entries but never sets planStatus
  // to 'executing'.
  if (planStatus === 'planning' && (agentStatus === 'completed' || agentStatus === 'failed')) {
    return agentStatus;
  }
  return planStatus;
}

/** Update plan entry statuses based on sub-agent block statuses in timeline */
function updatePlanEntryStatuses(entries: PlanEntry[], timeline: TimelineEntry[]): PlanEntry[] {
  return entries.map((entry) => {
    // Find matching sub-agent block in timeline
    const matchingBlock = timeline.find(
      (t) => t.kind === 'subagent' && (t.block.agentName === entry.agentName || t.block.task === entry.task),
    );
    if (matchingBlock && matchingBlock.kind === 'subagent') {
      const blockStatus = matchingBlock.block.status;
      return {
        ...entry,
        status:
          blockStatus === 'completed'
            ? ('completed' as const)
            : blockStatus === 'failed'
              ? ('failed' as const)
              : ('running' as const),
      };
    }
    return entry;
  });
}

/** Compute team status summary from sub-agent blocks in timeline */
function computeTeamStatus(timeline: TimelineEntry[]): TeamStatusSummary | null {
  const subAgents = timeline.filter((t) => t.kind === 'subagent');
  if (subAgents.length === 0) return null;

  return {
    total: subAgents.length,
    running: subAgents.filter((t) => t.kind === 'subagent' && t.block.status === 'running').length,
    completed: subAgents.filter((t) => t.kind === 'subagent' && t.block.status === 'completed').length,
    failed: subAgents.filter((t) => t.kind === 'subagent' && t.block.status === 'failed').length,
  };
}

// ── Sub-agent builder (for member-based sub-agents) ──

class SubAgentBuilder {
  private agentKey: string;
  private agentName: string;
  private agentIcon: string;
  private firstMsg: Message;
  public firstPosition: number;
  private entries: TimelineEntry[] = [];
  private sortCounter = 0;
  private lastContentMsg: Message | null = null;
  private allMemberMsgs: Message[] = [];
  private allToolMsgs: Message[] = [];

  constructor(agentKey: string, firstMsg: Message, position: number) {
    this.agentKey = agentKey;
    this.firstMsg = firstMsg;
    this.firstPosition = position;
    this.agentName = firstMsg.team_member?.name || firstMsg.agent_ref?.name || agentKey;
    this.agentIcon = firstMsg.team_member?.icon || firstMsg.agent_ref?.icon || this.agentName.charAt(0);
    this.allMemberMsgs.push(firstMsg);
  }

  addThinking(msg: Message): void {
    this.allMemberMsgs.push(msg);
    if (msg.reasoning_markdown?.trim()) {
      this.entries.push({
        kind: 'thinking',
        section: {
          id: `sub-think-${msg.id}`,
          content: msg.reasoning_markdown,
          durationMs: 0,
          collapsed: true,
          streaming: msg.status === 'streaming',
        },
        sortKey: this.sortCounter++,
      });
    }
  }

  addTool(toolEv: ToolUseEvent): void {
    this.entries.push({
      kind: 'tool',
      section: buildToolSection(toolEv, `sub-${this.agentKey}-tool`),
      sortKey: this.sortCounter++,
    });
  }

  addContent(msg: Message): void {
    this.allMemberMsgs.push(msg);
    this.lastContentMsg = msg;
  }

  build(): AgentBlock {
    // Determine status
    const isStreaming = this.allMemberMsgs.some((m) => m.status === 'streaming' || m.status === 'tool_running');
    const allToolsDone = this.allToolMsgs.every((t) => {
      const ev = toolEventFromMessage(t);
      return !ev || ev.status === 'success' || ev.status === 'failed' || ev.status === 'cancelled';
    });
    const hasMemberContent = this.allMemberMsgs.some((m) => m.content_markdown?.trim());
    const status: AgentBlockStatus = isStreaming
      ? 'running'
      : allToolsDone && hasMemberContent
        ? 'completed'
        : 'running';

    // Result: last member message content
    const result =
      [...this.allMemberMsgs].reverse().find((m: Message) => m.content_markdown?.trim())?.content_markdown || null;

    // Task: first member message content
    const task = this.allMemberMsgs[0]?.content_markdown || null;

    // Duration
    const durationMs = computeSubAgentDuration(this.allMemberMsgs, this.allToolMsgs);

    return {
      id: `sub-${this.agentKey}`,
      agentKey: this.agentKey,
      agentName: this.agentName,
      agentIcon: typeof this.agentIcon === 'string' ? this.agentIcon : this.agentName.charAt(0),
      agentColor: agentColorFromKey(this.agentKey),
      status,
      durationMs,
      collapsed: status === 'completed',
      task,
      timeline: this.entries,
      result,
      plan: null,
      teamStatus: null,
      startedAt: this.firstMsg.created_at || '',
      finishedAt: status === 'completed' ? this.allMemberMsgs.at(-1)?.created_at || '' : null,
    };
  }
}

function getOrCreateBuilder(
  builders: Map<string, SubAgentBuilder>,
  agentKey: string,
  msg: Message,
  position: number,
): SubAgentBuilder {
  let builder = builders.get(agentKey);
  if (!builder) {
    builder = new SubAgentBuilder(agentKey, msg, position);
    builders.set(agentKey, builder);
  }
  return builder;
}

/** Build a sub-agent block from a spawn/plan tool call */
function buildSubAgentFromSpawn(toolEv: ToolUseEvent, allToolMsgs: Message[]): AgentBlock | null {
  const args = asObject(toolEv.arguments);
  const agentName = String(args.name || args.agent_name || args.title || args.task || '子任务');
  const task = String(args.task || args.prompt || args.query || args.instruction || '');
  const icon = typeof agentName === 'string' ? agentName.charAt(0) : '子';

  const spawnKey = `spawn-${toolEv.id || toolEv.tool_name}`;

  // Build timeline: the spawn call itself is a tool entry
  const timeline: TimelineEntry[] = [];
  const spawnToolSection = buildToolSection(toolEv, `sub-${spawnKey}-tool`);
  timeline.push({ kind: 'tool', section: spawnToolSection, sortKey: 0 });

  // Find the corresponding result message
  if (toolEv.tool_name === 'subagents_spawn') {
    const resultMsg = findSubagentResult(allToolMsgs, toolEv);
    if (resultMsg) {
      const resultEv = toolEventFromMessage(resultMsg);
      if (resultEv) {
        timeline.push({ kind: 'tool', section: buildToolSection(resultEv, `sub-${spawnKey}-result`), sortKey: 1 });
      }
    }
  }

  const status: AgentBlockStatus =
    toolEv.status === 'success' || toolEv.status === 'failed'
      ? toolEv.status === 'failed'
        ? 'failed'
        : 'completed'
      : 'running';

  return {
    id: `sub-${spawnKey}`,
    agentKey: spawnKey,
    agentName,
    agentIcon: icon,
    agentColor: agentColorFromKey(spawnKey),
    status,
    durationMs: toolEv.duration_ms ?? null,
    collapsed: status === 'completed',
    task,
    timeline,
    result: toolEv.result
      ? typeof toolEv.result === 'object'
        ? JSON.stringify(toolEv.result)
        : String(toolEv.result)
      : null,
    plan: null,
    teamStatus: null,
    startedAt: toolEv.started_at || toolEv.occurred_at || '',
    finishedAt: status === 'completed' ? toolEv.finished_at || toolEv.occurred_at || '' : null,
  };
}

function buildToolSection(toolEv: ToolUseEvent, idPrefix: string): ToolSection {
  const status = normalizeToolSectionStatus(toolEv.status);
  const stableId = (toolEv.id || '').trim();
  return {
    id: stableId ? `${idPrefix}-${stableId}` : `${idPrefix}-missing-${toolEv.tool_name || 'tool'}`,
    toolName: toolEv.tool_name,
    toolLabel: resolveDisplayLabel(toolEv),
    status,
    durationMs: toolEv.duration_ms ?? null,
    arguments: toolEv.arguments != null ? safeStringify(toolEv.arguments) : null,
    result: toolEv.result != null ? safeStringify(toolEv.result) : toolEv.error || null,
    error: toolEv.error || null,
    collapsed: status === 'success' || status === 'failed' || status === 'cancelled',
    // Pass-through wire hints so AgentToolSection.vue can show a category
    // icon and a long-running pill instead of a generic bolt.
    iconKey: toolEv.icon_key,
    isLongRunning: toolEv.is_long_running === true,
  };
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

// ── Reply content resolution ──

/**
 * Extract the user-visible "reply" content for a single assistant message.
 *
 * Rules (mirrors the legacy `result` field logic so the timeline reply entry
 * matches the historical final answer):
 * - ReAct mode: use the explicit `finalAnswer` only when the model wrote a
 *   `FINAL_ANSWER` tag (i.e. `hasExplicitFinalAnswer` is true). Falling
 *   back to the last step body would duplicate the last thinking entry and
 *   produce a "thinking then echo the same content as reply" artifact.
 * - A2UI / userAction / default mode: use `bodyMarkdown` (the assistant's
 *   `content_markdown`).
 *
 * Returns null when there is no meaningful reply text (caller skips the
 * timeline entry rather than rendering an empty card).
 */
function resolveReplyContent(presentation: AssistantPresentation): string | null {
  if (presentation.mode === 'react' && presentation.reactSteps) {
    if (!presentation.reactSteps.hasExplicitFinalAnswer) return null;
    return presentation.reactSteps.finalAnswer?.trim() || null;
  }
  // a2ui / userAction / default: assistant content_markdown is the reply.
  return presentation.bodyMarkdown?.trim() || null;
}

/** Coerce an unknown value (LLM-emitted JSON shape) into a plain object for
 *  safe property access. Returns {} for arrays, primitives, or null. */
function asObject(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return {};
}

function normalizeToolSectionStatus(status: string): ToolSection['status'] {
  // Delegate to the shared statusMap. ToolSection.status is a closed union
  // (5 values), which is exactly the CanonicalToolStatus set.
  return canonicalToolStatus(status);
}

// Exported for unit tests; not part of the public composable contract.
export const __test__ = { buildToolSection, normalizeToolSectionStatus, computeAgentStatus };

function getMemberAgentKey(msg: Message): string | null {
  if (msg.origin?.kind === 'team_member') return msg.origin.agentKey;
  if (msg.team_member?.agent_id) return msg.team_member.agent_id;
  if (msg.agent_ref?.agent_key) return msg.agent_ref.agent_key;
  return null;
}

/**
 * Find the subagents_get result message that corresponds to a spawn call.
 */
function findSubagentResult(toolMsgs: Message[], spawnEv: ToolUseEvent): Message | null {
  const spawnArgs = asObject(spawnEv.arguments);
  const spawnName = String(spawnArgs.name || spawnArgs.agent_name || '');
  const spawnId = String(spawnArgs.id || spawnArgs.agent_id || spawnEv.id || '');

  for (const msg of toolMsgs) {
    const ev = toolEventFromMessage(msg);
    if (!ev || ev.tool_name !== 'subagents_get') continue;
    const args = asObject(ev.arguments);
    const getName = String(args.name || args.agent_name || args.id || args.agent_id || '');
    if (spawnName && getName === spawnName) return msg;
    if (
      spawnId &&
      (getName === spawnId ||
        (ev.result && typeof ev.result === 'object' && (ev.result as Record<string, unknown>).id === spawnId))
    )
      return msg;
  }
  return null;
}

function computeAgentStatus(
  assistant: Message | null,
  toolMsgs: Message[],
  memberMsgs: Message[],
  isCompleted: boolean,
): AgentBlockStatus {
  // Even when the assistant has stopped streaming, tools in non-terminal states
  // (running, blocked) mean the overall turn is NOT completed. Without this
  // check, a tool waiting for user confirmation (`tool_blocked`) would mark the
  // root agent as completed.
  const hasLiveTool = toolMsgs.some((t) => {
    const ev = toolEventFromMessage(t);
    if (!ev) return false;
    return ev.status === 'running' || ev.status === 'blocked';
  });
  if (isCompleted && !hasLiveTool) {
    // Determine final status based on the assistant message, not individual tools.
    // A team can have partial tool failures but still produce a successful result.
    if (assistant?.status === 'failed') return 'failed';
    // If there's a successful result (content_markdown), consider it completed
    // even if some tools failed along the way.
    const hasSuccessfulResult = assistant?.content_markdown?.trim() || assistant?.reasoning_markdown?.trim();
    if (hasSuccessfulResult) return 'completed';
    // No result at all — check if any tool failed
    const hasFailedTool = toolMsgs.some((t) => {
      const ev = toolEventFromMessage(t);
      return ev?.status === 'failed';
    });
    return hasFailedTool ? 'failed' : 'completed';
  }
  if (hasLiveTool) return 'running';
  if (assistant?.status === 'streaming') return 'running';
  if (memberMsgs.some((m) => m.status === 'streaming' || m.status === 'tool_running')) return 'running';
  return 'running';
}

function computeTurnDuration(turn: UserTurn): number | null {
  const start = turn.userMessage?.created_at;
  const end = [...turn.messages].reverse().find((m) => m.created_at)?.created_at;
  if (!start || !end) return null;
  const ms = new Date(end).getTime() - new Date(start).getTime();
  return ms > 0 ? ms : null;
}

function computeSubAgentDuration(memberMsgs: Message[], toolMsgs: Message[]): number | null {
  const allMsgs = [...memberMsgs, ...toolMsgs];
  if (allMsgs.length === 0) return null;
  const start = allMsgs[0]?.created_at;
  const end = allMsgs.at(-1)?.created_at;
  if (!start || !end) return null;
  const ms = new Date(end).getTime() - new Date(start).getTime();
  return ms > 0 ? ms : null;
}
