import type { FieldScope } from './fieldGuides';
import { createAIRefineService } from '../../services';
import type {
  RefineScope,
  RefineRequest as KratosRefineRequest,
  RefineResponse as KratosRefineResponse,
} from '../../services/kratos/ai_refine/v1/index';

const fieldScopeToRefineScope: Record<FieldScope, RefineScope> = {
  'category.industry': 'REFINE_SCOPE_CATEGORY_INDUSTRY',
  'category.department': 'REFINE_SCOPE_CATEGORY_DEPT',
  'category.position': 'REFINE_SCOPE_CATEGORY_POSITION',
  'agent.description': 'REFINE_SCOPE_AGENT_DESCRIPTION',
  'agent.file': 'REFINE_SCOPE_AGENT_FILE',
};

export interface RefineRequest {
  scope: FieldScope;
  resourceId?: string;
  fileName?: string;
  originalText: string;
  userHint?: string;
  targetMode?: string;
}

export interface RefineResponse {
  refined: string;
  diff: string;
  tokensBefore: number;
  tokensAfter: number;
  provider: string;
  model: string;
  source: 'agent_model' | 'system_default' | 'catalog_first' | string;
}

export async function refinePromptField(req: RefineRequest): Promise<RefineResponse> {
  const client = createAIRefineService();
  const res: KratosRefineResponse = await client.Refine({
    scope: fieldScopeToRefineScope[req.scope],
    resourceId: req.resourceId ?? '',
    fileName: req.fileName ?? '',
    originalText: req.originalText,
    userHint: req.userHint ?? '',
    targetMode: req.targetMode ?? 'complete',
  });
  return {
    refined: res.refined ?? '',
    diff: res.diff ?? '',
    tokensBefore: res.tokensBefore ?? 0,
    tokensAfter: res.tokensAfter ?? 0,
    provider: res.provider ?? '',
    model: res.model ?? '',
    source: res.source ?? '',
  };
}
