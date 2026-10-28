import type { CompanyModule, ProductMetric, SceneId, ShopId, StepId } from './types';

export const SCENES: SceneId[] = [1, 2, 3, 4, 5];
export const STEPS: StepId[] = [1, 2, 3, 4, 5, 6, 7];
export const SHOPS: ShopId[] = ['a', 'b', 'c', 'd'];
export const PRODUCT_METRICS: ProductMetric[] = [
  { id: 'citrus', nameKey: 'citrus', visitors: 380, addToCart: 34, orders: 16 },
  { id: 'sea-salt', nameKey: 'seaSalt', visitors: 420, addToCart: 13, orders: 4, previousCartRate: 8.2 },
  { id: 'lavender', nameKey: 'lavender', visitors: 260, addToCart: 18, orders: 8 },
  { id: 'other', nameKey: 'other', visitors: 180, addToCart: 9, orders: 3 },
];
export const COMPANY_MODULES: CompanyModule[] = [
  { id: 'ecommerce', tone: 'owner', icon: 'shopping_bag', children: ['shopA', 'shopB', 'shopC', 'shopD'] },
  {
    id: 'content',
    tone: 'data',
    icon: 'smart_display',
    children: ['mainAccount', 'secondaryAccount', 'otherPlatform'],
  },
  {
    id: 'opensource',
    tone: 'ai',
    icon: 'account_tree',
    children: ['repoA', 'repoB', 'repoC', 'repoD', 'repoE', 'otherRepo'],
  },
  { id: 'investment', tone: 'danger', icon: 'trending_up', children: ['quant', 'industry'] },
  { id: 'knowledge', tone: 'purple', icon: 'storage', children: ['knowledge', 'status'] },
  { id: 'assistant', tone: 'working', icon: 'emoji_events', children: ['hackathon', 'forum', 'schedule'] },
];
export const RESOURCE_TONES: Record<string, string> = {
  shopA: 'owner',
  shopB: 'danger',
  shopC: 'purple',
  shopD: 'ai',
  mainAccount: 'data',
  secondaryAccount: 'data',
  otherPlatform: 'purple',
  repoA: 'ai',
  repoB: 'data',
  repoC: 'purple',
  repoD: 'working',
  repoE: 'danger',
  otherRepo: 'muted',
  schedule: 'owner',
};
export const PROGRESS_KEYS = [
  'suggestion',
  'summary',
  'approval',
  'dispatch',
  'draft',
  'publish',
  'verify',
  'feedback',
];

export function formatRate(numerator: number | null, denominator: number | null): string {
  if (numerator === null || denominator === null || denominator <= 0) return '—';
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

export const FLOW_POSITIONS: Record<StepId, { x: number; y: number; w: number; h: number }> = {
  1: { x: 44, y: 768, w: 516, h: 261 },
  2: { x: 44, y: 460, w: 516, h: 261 },
  3: { x: 44, y: 150, w: 516, h: 261 },
  4: { x: 724, y: 150, w: 494, h: 261 },
  5: { x: 1383, y: 150, w: 517, h: 261 },
  6: { x: 1385, y: 460, w: 513, h: 261 },
  7: { x: 1383, y: 768, w: 517, h: 261 },
};

// Separate source anchors: the launch example is not the seven-step overview layout.
export const LAUNCH_POSITIONS: typeof FLOW_POSITIONS = {
  1: { x: 50, y: 768, w: 509, h: 255 },
  2: { x: 50, y: 461, w: 510, h: 257 },
  3: { x: 53, y: 155, w: 506, h: 257 },
  4: { x: 724, y: 155, w: 491, h: 257 },
  5: { x: 1377, y: 154, w: 508, h: 258 },
  6: { x: 1378, y: 463, w: 506, h: 254 },
  7: { x: 1377, y: 768, w: 507, h: 255 },
};
