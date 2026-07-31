// 商城 UI 共享常量：资产类型元数据（图标/配色/文案 key）。
// 展示组件从这里取类型展示信息，避免各组件重复维护映射表。
import type { MarketAssetType, PriceModel } from './types';

export interface AssetTypeMeta {
  icon: string;
  /** Quasar 色名或 hex，用于图标底色 */
  color: string;
  /** i18n key 后缀（shopPage.type.<key>） */
  labelKey: string;
}

export const ASSET_TYPE_META: Record<MarketAssetType, AssetTypeMeta> = {
  skill: { icon: 'psychology', color: '#E9A23B', labelKey: 'skill' },
  mcp_server: { icon: 'extension', color: '#4CAF7C', labelKey: 'mcpServer' },
  tool: { icon: 'handyman', color: '#5E8BF0', labelKey: 'tool' },
  plugin: { icon: 'tune', color: '#8B6FF0', labelKey: 'plugin' },
  agent: { icon: 'smart_toy', color: '#E55C8A', labelKey: 'agent' },
  team: { icon: 'groups', color: '#2BB3A3', labelKey: 'team' },
  channel_template: { icon: 'hub', color: '#F09B54', labelKey: 'channelTemplate' },
  knowledge_pack: { icon: 'menu_book', color: '#C9A227', labelKey: 'knowledgePack' },
  workflow: { icon: 'account_tree', color: '#6C7BD9', labelKey: 'workflow' },
  company_bundle: { icon: 'inventory_2', color: '#D48C1A', labelKey: 'companyBundle' },
  org_bundle: { icon: 'corporate_fare', color: '#7C5CE0', labelKey: 'orgBundle' },
};

export const ALL_ASSET_TYPES = Object.keys(ASSET_TYPE_META) as MarketAssetType[];

export const PRICE_MODELS: PriceModel[] = ['free', 'one_time', 'subscription', 'enterprise'];

/** 分转人民币显示（保留两位去尾零） */
export function formatCents(cents: number): string {
  const yuan = cents / 100;
  return yuan % 1 === 0 ? String(yuan) : yuan.toFixed(2);
}

/** 千分位格式化安装数，>1万显示 x.x万 */
export function formatInstalls(count: number): string {
  if (count >= 10000) return `${(count / 10000).toFixed(1)}w`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
  return String(count);
}
