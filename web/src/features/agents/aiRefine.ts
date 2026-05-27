/**
 * PGO-3: API client for POST /v1/ai/refine.
 * Provides a typed wrapper around the unified AI refinement endpoint.
 */
import type { FieldScope } from './fieldGuides'

export interface RefineRequest {
  scope: FieldScope
  resourceId?: string   // agent_id or category_id
  fileName?: string     // only for scope='agent.file'
  originalText: string
  userHint?: string     // optional user instruction
  targetMode?: string   // complete | task | minimized
}

export interface RefineResponse {
  refined: string
  diff: string
  tokensBefore: number
  tokensAfter: number
  provider: string
  model: string
  source: 'agent_model' | 'system_default' | 'catalog_first' | string
}

/** POST /v1/ai/refine */
export async function refinePromptField(req: RefineRequest): Promise<RefineResponse> {
  const body = {
    scope: req.scope,
    resource_id: req.resourceId ?? '',
    file_name: req.fileName ?? '',
    original_text: req.originalText,
    user_hint: req.userHint ?? '',
    target_mode: req.targetMode ?? 'complete',
  }

  const response = await fetch('/v1/ai/refine', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })

  if (!response.ok) {
    const errBody = await response.json().catch(() => ({}))
    const msg = (errBody as { message?: string }).message ?? `AI refine failed (${response.status})`
    throw new Error(msg)
  }

  const data = await response.json()
  return {
    refined: data.refined ?? '',
    diff: data.diff ?? '',
    tokensBefore: data.tokens_before ?? 0,
    tokensAfter: data.tokens_after ?? 0,
    provider: data.provider ?? '',
    model: data.model ?? '',
    source: data.source ?? '',
  }
}
