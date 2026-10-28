export type SceneId = 1 | 2 | 3 | 4 | 5;
export type StepId = 1 | 2 | 3 | 4 | 5 | 6 | 7;
export type ShopId = 'a' | 'b' | 'c' | 'd';
export type CaseId = 'traffic' | 'launch';
export type ViewMode = 'workbench' | 'reference';
export type ReferenceFocus =
  | 'ceo-summary'
  | 'ceo-detail'
  | 'products'
  | 'funnel-overview'
  | 'funnel'
  | 'table'
  | 'report'
  | 'anomaly'
  | 'suggestion';
export interface DemoViewState {
  viewMode: ViewMode;
  scene: SceneId;
  focusStep: StepId | null;
  referenceFocus: ReferenceFocus | null;
  companyFocused: boolean;
  selectedShop: ShopId;
  selectedProduct: string;
  activeCase: CaseId;
}
export type WorkflowAction =
  | 'approved'
  | 'rejected'
  | 'revise'
  | 'advance'
  | 'publish'
  | 'verify'
  | 'simulate-verification-failure'
  | 'cancel-edit'
  | 'feedback';
export interface WorkflowActionPayload {
  action: WorkflowAction;
  version: number;
  note?: string;
}
export type Tone = 'data' | 'ai' | 'ceo' | 'owner' | 'working' | 'danger' | 'purple';
export type DemoPhase =
  | 'awaiting_decision'
  | 'rejected'
  | 'approved'
  | 'drafting'
  | 'awaiting_publish_approval'
  | 'verification_pending'
  | 'rework_required'
  | 'awaiting_feedback'
  | 'closed';

export interface DemoDraft {
  id: string;
  productId: string;
  titleKey: string;
  scentKey: string;
  published: boolean;
}

export interface DemoWorkflow {
  kind: CaseId;
  phase: DemoPhase;
  version: number;
  approvedVersion: number | null;
  publishVersion: number | null;
  note: string;
  drafts: DemoDraft[];
  history: DemoPhase[];
}

export type ShopWorkflows = Record<ShopId, Record<CaseId, DemoWorkflow>>;
export interface ProductMetric extends Record<string, unknown> {
  id: string;
  nameKey: string;
  visitors: number;
  addToCart: number;
  orders: number;
  previousCartRate?: number;
}

export interface CompanyModule {
  id: string;
  tone: Tone;
  icon: string;
  children: string[];
}
