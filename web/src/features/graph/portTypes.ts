import type { NodeDef, StateFieldDef } from './types';

// ---------------------------------------------------------------------------
// Port direction
// ---------------------------------------------------------------------------

export type PortDirection = 'reads' | 'writes';

// ---------------------------------------------------------------------------
// Port info
// ---------------------------------------------------------------------------

export type PortInfo = {
  direction: PortDirection;
  field: string; // State field name
  fieldType: string; // State field type (string/integer/float/boolean/array/object)
  nodeId: string;
};

// ---------------------------------------------------------------------------
// Handle ID encoding / decoding
// Format: r:fieldName:nodeId  or  w:fieldName:nodeId
// Field names may contain colons, so we only split on the first two colons.
// ---------------------------------------------------------------------------

export function encodeHandleId(port: PortInfo): string {
  const prefix = port.direction === 'reads' ? 'r' : 'w';
  return `${prefix}:${port.field}:${port.nodeId}`;
}

export function decodeHandleId(id: string): PortInfo {
  const firstColon = id.indexOf(':');
  if (firstColon < 0) throw new Error(`Invalid handle ID: ${id}`);
  const lastColon = id.lastIndexOf(':');
  if (lastColon === firstColon) throw new Error(`Invalid handle ID: ${id}`);
  const prefix = id.slice(0, firstColon);
  const field = id.slice(firstColon + 1, lastColon);
  const nodeId = id.slice(lastColon + 1);
  return {
    direction: prefix === 'r' ? 'reads' : 'writes',
    field,
    fieldType: '', // not encoded in handle ID; caller should look up from stateFields
    nodeId,
  };
}

// ---------------------------------------------------------------------------
// Connection validation (quick structural checks only)
// ---------------------------------------------------------------------------

export type ConnectionValidationResult = {
  valid: boolean;
  reason?: string;
  warning?: string; // non-blocking warning (e.g. field name mismatch)
};

export function isValidConnectionQuick(
  sourceNodeId: string,
  sourceHandleId: string | null,
  targetNodeId: string,
  targetHandleId: string | null,
  existingEdges: Array<{ from: string; to: string }>,
): ConnectionValidationResult {
  // Reject self-connections
  if (sourceNodeId === targetNodeId) {
    return { valid: false, reason: 'Cannot connect a node to itself' };
  }

  // Reject duplicate edges
  const isDuplicate = existingEdges.some((e) => e.from === sourceNodeId && e.to === targetNodeId);
  if (isDuplicate) {
    return { valid: false, reason: 'Duplicate edge' };
  }

  // If both handles have valid encoded IDs, check field name match (warning only)
  if (sourceHandleId && targetHandleId) {
    try {
      const sourcePort = decodeHandleId(sourceHandleId);
      const targetPort = decodeHandleId(targetHandleId);

      if (sourcePort.field !== targetPort.field) {
        return {
          valid: true,
          warning: `Field name mismatch: source writes "${sourcePort.field}", target reads "${targetPort.field}"`,
        };
      }
    } catch {
      // Handle IDs don't follow our encoding convention — skip field check
    }
  }

  return { valid: true };
}

// ---------------------------------------------------------------------------
// getNodePorts — derive port info from a NodeDef and state fields
// ---------------------------------------------------------------------------

function findFieldType(stateFields: StateFieldDef[], fieldName: string): string {
  return stateFields.find((f) => f.name === fieldName)?.type ?? 'string';
}

/** Parse ${fieldName} patterns from a template string */
function parseTemplateFields(template: string): string[] {
  const regex = /\$\{([^}]+)\}/g;
  const fields: string[] = [];
  let match: RegExpExecArray | null;
  while ((match = regex.exec(template)) !== null) {
    fields.push(match[1]);
  }
  return fields;
}

/** Safely parse JSON, returning null on failure */
function safeParseJson(raw: string): Record<string, string> | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
      return parsed as Record<string, string>;
    }
    return null;
  } catch {
    return null;
  }
}

export function getNodePorts(node: NodeDef, stateFields: StateFieldDef[]): { reads: PortInfo[]; writes: PortInfo[] } {
  switch (node.type) {
    case 'function': {
      // Can't introspect Go function signatures from frontend
      return { reads: [], writes: [] };
    }

    case 'llm': {
      const reads: PortInfo[] = [];
      const templateFields = parseTemplateFields(node.instruction);
      for (const field of templateFields) {
        reads.push({
          direction: 'reads',
          field,
          fieldType: findFieldType(stateFields, field),
          nodeId: node.id,
        });
      }
      const writes: PortInfo[] = [
        {
          direction: 'writes',
          // 与后端 trpc-agent-go graph 的 StateKeyLastResponse 保持一致
          field: 'last_response',
          fieldType: findFieldType(stateFields, 'last_response'),
          nodeId: node.id,
        },
      ];
      return { reads, writes };
    }

    case 'tool': {
      // Can't determine tool input/output fields from frontend
      return { reads: [], writes: [] };
    }

    case 'agent': {
      const reads: PortInfo[] = [];
      const writes: PortInfo[] = [];

      // inputMapperJson: {"agent_field": "state_field"} → reads state_field
      const inputMap = safeParseJson(node.inputMapperJson);
      if (inputMap) {
        for (const stateField of Object.values(inputMap)) {
          reads.push({
            direction: 'reads',
            field: stateField,
            fieldType: findFieldType(stateFields, stateField),
            nodeId: node.id,
          });
        }
      }

      // outputMapperJson: {"state_field": "agent_field"} → writes state_field
      const outputMap = safeParseJson(node.outputMapperJson);
      if (outputMap) {
        for (const stateField of Object.keys(outputMap)) {
          writes.push({
            direction: 'writes',
            field: stateField,
            fieldType: findFieldType(stateFields, stateField),
            nodeId: node.id,
          });
        }
      }

      return { reads, writes };
    }

    case 'router': {
      // Can't determine what condFuncRef reads; router passes through
      return { reads: [], writes: [] };
    }

    case 'join': {
      // Pass-through
      return { reads: [], writes: [] };
    }

    case 'hitl': {
      // Can't determine approval fields from frontend
      return { reads: [], writes: [] };
    }

    default: {
      return { reads: [], writes: [] };
    }
  }
}

// ---------------------------------------------------------------------------
// State field type → CSS variable for Handle coloring
// ---------------------------------------------------------------------------

export const STATE_FIELD_TYPE_COLORS: Record<string, string> = {
  string: 'var(--graph-port-string)',
  integer: 'var(--graph-port-integer)',
  float: 'var(--graph-port-float)',
  boolean: 'var(--graph-port-boolean)',
  array: 'var(--graph-port-array)',
  object: 'var(--graph-port-object)',
  // Extended types matching Langflow datatype colors
  Text: 'var(--graph-port-string)',
  Message: 'var(--graph-port-string)',
  number: 'var(--graph-port-integer)',
  Document: 'var(--graph-port-document)',
  Data: 'var(--graph-port-object)',
  JSON: 'var(--graph-port-object)',
  Dict: 'var(--graph-port-object)',
  List: 'var(--graph-port-array)',
  Embeddings: 'var(--graph-port-embeddings)',
  BaseLanguageModel: 'var(--graph-port-model)',
  LanguageModel: 'var(--graph-port-model)',
  Agent: 'var(--graph-port-agent)',
  Tool: 'var(--graph-port-tool)',
  Prompt: 'var(--graph-port-prompt)',
  unknown: 'var(--graph-port-unknown)',
};
