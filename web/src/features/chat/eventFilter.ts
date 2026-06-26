/**
 * Minimal event shape consumed by the session event inspector.
 *
 * The inspector displays raw real-time events from the WS envelope stream
 * (the GET /v1/events API also returns this format). This local type
 * decouples the inspector from the global Envelope type, capturing only
 * the fields the inspector actually accesses. Full Envelope objects are
 * structurally compatible and can be assigned directly without conversion.
 */
export type InspectorEvent = {
  id: string;
  type: string;
  author: string;
  timestamp: string;
  channel?: string;
  branch?: string;
  filter_key?: string;
  tag?: string;
  invocation_id?: string;
  parent_invocation_id?: string;
  content?: { text: string };
  tool_call?: {
    name: string;
    status: string;
    is_long_running?: boolean;
  };
  state_delta?: {
    operation: string;
    path: string;
    value_json: string;
  };
  transfer?: {
    from_agent: string;
    to_agent: string;
  };
};

export type EventFilterState = {
  typeFilter: string;
  branchPrefix: string;
  tag: string;
  keyword: string;
  filterKey: string;
};

export const defaultEventFilterState = (): EventFilterState => ({
  typeFilter: 'all',
  branchPrefix: '',
  tag: '',
  keyword: '',
  filterKey: '',
});

export type BranchNode = {
  id: string;
  label: string;
  author: string;
  type: string;
  timestamp: string;
  branch?: string;
  children: BranchNode[];
};

function envelopeContainsTag(tagField: string | undefined, tag: string): boolean {
  if (!tag || !tagField) return false;
  if (tagField === tag) return true;
  return tagField.split(',').some((part) => part.trim() === tag);
}

function matchFilterKeyPrefix(subscriberKey: string, eventKey: string): boolean {
  if (!subscriberKey || !eventKey) return true;
  const sk = subscriberKey + '/';
  const ek = eventKey + '/';
  return sk.startsWith(ek) || ek.startsWith(sk);
}

export function filterEnvelopes(events: InspectorEvent[], filters: EventFilterState): InspectorEvent[] {
  const kw = filters.keyword.trim().toLowerCase();
  return events.filter((env) => {
    if (filters.typeFilter !== 'all' && env.type !== filters.typeFilter) return false;
    if (filters.branchPrefix.trim()) {
      const branch = env.branch ?? '';
      if (!branch.startsWith(filters.branchPrefix.trim())) return false;
    }
    if (filters.tag.trim() && !envelopeContainsTag(env.tag, filters.tag.trim())) return false;
    if (filters.filterKey.trim() && !matchFilterKeyPrefix(filters.filterKey.trim(), env.filter_key ?? '')) {
      return false;
    }
    if (kw) {
      const hay = [env.type, env.author, env.branch, env.filter_key, env.tag, env.content?.text, env.tool_call?.name]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      if (!hay.includes(kw)) return false;
    }
    return true;
  });
}

export function buildBranchTree(events: InspectorEvent[]): BranchNode[] {
  const nodes = new Map<string, BranchNode>();
  const roots: BranchNode[] = [];
  const childOf = new Map<string, string>();

  for (const env of events) {
    const id = env.invocation_id?.trim();
    if (!id) continue;
    if (!nodes.has(id)) {
      nodes.set(id, {
        id,
        label: env.type,
        author: env.author,
        type: env.type,
        timestamp: env.timestamp,
        branch: env.branch,
        children: [],
      });
    } else {
      const n = nodes.get(id)!;
      n.type = env.type;
      n.label = env.type;
      n.timestamp = env.timestamp;
    }
    const parent = env.parent_invocation_id?.trim();
    if (parent) childOf.set(id, parent);
  }

  for (const [id, node] of nodes) {
    const parentId = childOf.get(id);
    if (parentId && nodes.has(parentId)) {
      nodes.get(parentId)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  const sortNodes = (list: BranchNode[]) => {
    list.sort((a, b) => a.timestamp.localeCompare(b.timestamp));
    for (const n of list) sortNodes(n.children);
  };
  sortNodes(roots);
  return roots;
}
