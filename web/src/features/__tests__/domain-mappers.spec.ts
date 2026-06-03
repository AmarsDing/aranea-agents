/**
 * Mapper compatibility tests for Knowledge / Artifact / Evaluation / A2A domains.
 *
 * These tests verify that the wireJson pick helpers correctly handle both
 * snake_case (legacy proto-JSON style) and camelCase (generated TS client style).
 *
 * If the backend JSON naming convention changes, these tests will catch mapper regressions
 * before they cause page crashes.
 */

import { describe, it, expect } from 'vitest';
import { asRecord, pickBool, pickI32, pickNum, pickStr } from '../../shared/wireJson';
import { mapCollection } from '../knowledge/mappers';
import { mapDataset, mapRun } from '../evaluation/mappers';

// ── Knowledge collection mapper ──────────────────────────────────────────────

describe('knowledge mapCollection', () => {
  it('maps snake_case fields correctly', () => {
    const raw = {
      id: 'col-1',
      name: 'test',
      embedding_model: 'text-embedding-3-small',
      dim: 1536,
      status: 'ready',
      document_count: 5,
      chunk_count: 42,
      created_at: '2026-01-01T00:00:00Z',
    };
    const c = mapCollection(raw);
    expect(c.id).toBe('col-1');
    expect(c.embedding_model).toBe('text-embedding-3-small');
    expect(c.dim).toBe(1536);
    expect(c.document_count).toBe(5);
    expect(c.chunk_count).toBe(42);
  });

  it('maps camelCase fields correctly', () => {
    const raw = {
      id: 'col-2',
      name: 'test2',
      embeddingModel: 'text-embedding-ada-002',
      dim: 1024,
      status: 'indexing',
      documentCount: 3,
      chunkCount: 10,
      createdAt: '2026-01-02T00:00:00Z',
    };
    const c = mapCollection(raw);
    expect(c.embedding_model).toBe('text-embedding-ada-002');
    expect(c.document_count).toBe(3);
    expect(c.created_at).toBe('2026-01-02T00:00:00Z');
  });

  it('returns empty strings and zeros for missing fields', () => {
    const c = mapCollection({});
    expect(c.id).toBe('');
    expect(c.dim).toBe(0);
    expect(c.document_count).toBe(0);
  });
});

// ── Artifact meta mapper ─────────────────────────────────────────────────────

function mapArtifactMeta(raw: unknown) {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    session_id: pickStr(r, 'session_id', 'sessionId'),
    name: pickStr(r, 'name', 'name'),
    mime_type: pickStr(r, 'mime_type', 'mimeType'),
    size: pickNum(r, 'size', 'size'),
    sha256: pickStr(r, 'sha256', 'sha256'),
    storage_kind: pickStr(r, 'storage_kind', 'storageKind'),
    version: pickI32(r, 'version', 'version'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

describe('artifact mapMeta', () => {
  it('maps snake_case fields correctly', () => {
    const raw = {
      id: 'art-1',
      session_id: 'sess-1',
      name: 'report.pdf',
      mime_type: 'application/pdf',
      size: 1024,
      sha256: 'abc123',
      storage_kind: 's3',
      version: 1,
      created_at: '2026-01-01T00:00:00Z',
    };
    const m = mapArtifactMeta(raw);
    expect(m.session_id).toBe('sess-1');
    expect(m.mime_type).toBe('application/pdf');
    expect(m.storage_kind).toBe('s3');
    expect(m.size).toBe(1024);
  });

  it('maps camelCase fields correctly', () => {
    const raw = {
      id: 'art-2',
      sessionId: 'sess-2',
      name: 'data.csv',
      mimeType: 'text/csv',
      size: 512,
      sha256: 'def456',
      storageKind: 'local',
      version: 2,
      createdAt: '2026-01-02T00:00:00Z',
    };
    const m = mapArtifactMeta(raw);
    expect(m.session_id).toBe('sess-2');
    expect(m.mime_type).toBe('text/csv');
    expect(m.storage_kind).toBe('local');
    expect(m.version).toBe(2);
  });
});

// ── Evaluation dataset/run mapper ────────────────────────────────────────────

describe('evaluation mapDataset', () => {
  it('handles snake_case case_count', () => {
    const d = mapDataset({ id: 'ds-1', name: 'bench', case_count: 100, created_at: '2026-01-01T00:00:00Z' });
    expect(d.case_count).toBe(100);
  });

  it('handles camelCase caseCount', () => {
    const d = mapDataset({ id: 'ds-2', name: 'bench2', caseCount: 200, createdAt: '2026-01-02T00:00:00Z' });
    expect(d.case_count).toBe(200);
  });
});

describe('evaluation mapRun', () => {
  it('maps snake_case run fields', () => {
    const r = mapRun({
      id: 'run-1',
      dataset_id: 'ds-1',
      agent_id: 'ag-1',
      status: 'completed',
      total_cases: 50,
      completed_cases: 50,
      exact_match_score: 0.92,
    });
    expect(r.dataset_id).toBe('ds-1');
    expect(r.total_cases).toBe(50);
    expect(r.exact_match_score).toBeCloseTo(0.92);
  });

  it('maps camelCase run fields', () => {
    const r = mapRun({
      id: 'run-2',
      datasetId: 'ds-2',
      agentId: 'ag-2',
      status: 'running',
      totalCases: 30,
      completedCases: 15,
      exactMatchScore: 0.5,
    });
    expect(r.dataset_id).toBe('ds-2');
    expect(r.total_cases).toBe(30);
    expect(r.completed_cases).toBe(15);
    expect(r.exact_match_score).toBeCloseTo(0.5);
  });
});

// ── A2A agent card mapper ─────────────────────────────────────────────────────

function mapAgentCard(raw: unknown) {
  const r = asRecord(raw);
  const capsRaw = r.capabilities ?? r.Capabilities;
  const capabilities = Array.isArray(capsRaw)
    ? (capsRaw as unknown[]).map((c) => {
        const cr = asRecord(c);
        return {
          name: pickStr(cr, 'name', 'name'),
          input_schema_json: pickStr(cr, 'input_schema_json', 'inputSchemaJson'),
        };
      })
    : [];
  return {
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    display_name: pickStr(r, 'display_name', 'displayName'),
    enabled: pickBool(r, 'enabled', 'enabled'),
    capabilities,
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

describe('a2a mapAgentCard', () => {
  it('maps snake_case fields and capabilities', () => {
    const card = mapAgentCard({
      agent_id: 'ag-1',
      display_name: 'My Agent',
      enabled: true,
      capabilities: [{ name: 'summarize', input_schema_json: '{}' }],
      updated_at: '2026-01-01T00:00:00Z',
    });
    expect(card.agent_id).toBe('ag-1');
    expect(card.display_name).toBe('My Agent');
    expect(card.capabilities).toHaveLength(1);
    expect(card.capabilities[0].input_schema_json).toBe('{}');
  });

  it('maps camelCase fields and capabilities', () => {
    const card = mapAgentCard({
      agentId: 'ag-2',
      displayName: 'Agent Two',
      enabled: false,
      capabilities: [{ name: 'translate', inputSchemaJson: '{"lang":"string"}' }],
      updatedAt: '2026-01-02T00:00:00Z',
    });
    expect(card.agent_id).toBe('ag-2');
    expect(card.display_name).toBe('Agent Two');
    expect(card.enabled).toBe(false);
    expect(card.capabilities[0].input_schema_json).toBe('{"lang":"string"}');
    expect(card.updated_at).toBe('2026-01-02T00:00:00Z');
  });

  it('returns empty capabilities for missing field', () => {
    const card = mapAgentCard({ agentId: 'ag-3' });
    expect(card.capabilities).toEqual([]);
  });
});
