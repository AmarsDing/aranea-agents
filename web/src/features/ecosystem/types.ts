// 生态商城领域类型。
// 说明：M57 公网商城后端尚未实现，本文件同时包含
//  1) 现有 M30 真实 RPC 对应的精简类型（EcosystemProduct，保留兼容）
//  2) UI 骨架使用的完整商城领域模型（Market* / My* / Studio*），字段对齐
//     docs/development/57-marketplace-platform.design.md §四 领域模型。

// ── M30 既有 RPC 类型（真实后端） ──────────────────────────────
export type EcosystemProduct = {
  id: string;
  name: string;
  display_name: string;
  description: string;
  type: string;
  version: string;
  install_count: number;
  installed: boolean;
};

// ── M57 商城领域模型（骨架期 mock 驱动） ───────────────────────

/** 资产类型：M57 10 类 + org_bundle（组织架构整包：部门/岗位/Agent 打包） */
export type MarketAssetType =
  | 'skill'
  | 'mcp_server'
  | 'tool'
  | 'plugin'
  | 'agent'
  | 'team'
  | 'channel_template'
  | 'knowledge_pack'
  | 'workflow'
  | 'company_bundle'
  | 'org_bundle';

export type PriceModel = 'free' | 'one_time' | 'subscription' | 'enterprise';

export type ReviewStatus = 'published' | 'scanning' | 'manual' | 'needs_fix' | 'rejected';

export interface MarketCreator {
  handle: string;
  name: string;
  verified: boolean;
  /** 骨架期头像色块（无真实头像图） */
  avatarColor: string;
  bio: string;
  assetCount: number;
  totalInstalls: number;
  avgRating: number;
  followers: number;
}

export type PermissionRisk = 'low' | 'medium' | 'high';

export interface MarketPermission {
  /** model | tool | credential | network | fs | command | mcp */
  kind: string;
  value: string;
  note?: string;
  risk: PermissionRisk;
}

export interface MarketReview {
  id: string;
  author: string;
  rating: number;
  content: string;
  createdAt: string;
  likes: number;
  reply?: { author: string; content: string; createdAt: string };
}

export interface MarketDep {
  id: string;
  name: string;
  range: string;
  kind: MarketAssetType;
}

export interface MarketVersion {
  version: string;
  date: string;
  note: string;
}

/** 组织架构整包预览：部门 → 岗位 → Agent 三层 */
export interface OrgBundlePreview {
  departments: {
    key: string;
    name: string;
    positions: { key: string; name: string; agents: string[] }[];
  }[];
  totals: { departments: number; positions: number; agents: number };
}

export interface MarketAsset {
  id: string;
  slug: string;
  name: string;
  type: MarketAssetType;
  /** 一句话卖点（卡片副标题） */
  tagline: string;
  description: string;
  /** Markdown 全文 */
  readme: string;
  creator: MarketCreator;
  /** 三级分类路径，如 "研发/编程/代码审查" */
  categories: string[];
  tags: string[];
  version: string;
  compatibility: string;
  priceModel: PriceModel;
  priceCents: number;
  rating: number;
  ratingCount: number;
  /** 5★→1★ 分布 */
  ratingDist: [number, number, number, number, number];
  installCount: number;
  activity30d: number;
  health7d: number;
  permissions: MarketPermission[];
  /** 截图数量（骨架期按 id+序号生成配图） */
  screenshotCount: number;
  deps: MarketDep[];
  status: 'published' | 'deprecated';
  publishedAt: string;
  updatedAt: string;
  installed: boolean;
  versions: MarketVersion[];
  reviews: MarketReview[];
  orgBundle?: OrgBundlePreview;
}

/** 三级分类树节点 */
export interface CategoryNode {
  key: string;
  label: string;
  /** 一级分类图标（数据侧提供，组件不硬编码） */
  icon?: string;
  children?: CategoryNode[];
}

export type InstallHealth = 'healthy' | 'degraded' | 'failed';

export type MyInstall = {
  assetId: string;
  name: string;
  type: MarketAssetType;
  version: string;
  installedAt: string;
  health7d: number;
  /** 有可用更新时为最新版本号 */
  updateAvailable?: string;
  status: InstallHealth;
};

export type OrderStatus = 'paid' | 'refunding' | 'refunded';

export type MyOrder = {
  id: string;
  assetId: string;
  name: string;
  type: MarketAssetType;
  priceModel: PriceModel;
  amountCents: number;
  status: OrderStatus;
  createdAt: string;
};

export interface StudioStats {
  totalInstalls: number;
  revenueCents: number;
  avgRating: number;
  activeAssets: number;
  installTrend: number[];
  revenueTrend: number[];
}

export type StudioAsset = {
  id: string;
  name: string;
  type: MarketAssetType;
  version: string;
  reviewStatus: ReviewStatus;
  installs: number;
  rating: number;
  revenueCents: number;
  updatedAt: string;
};

export interface StudioInboxItem {
  id: string;
  assetName: string;
  author: string;
  rating: number;
  content: string;
  createdAt: string;
  replied: boolean;
}

/** 发布向导：组织整包可选节点（部门 → 岗位 → Agent 名） */
export interface OrgPickNode {
  key: string;
  label: string;
  children?: OrgPickNode[];
  /** 仅岗位节点有值：该岗位下可选 Agent */
  agents?: string[];
}

// ── 浏览页过滤/排序 ────────────────────────────────────────────

export type BrowseSort = 'hot' | 'new' | 'rating' | 'installs' | 'activity' | 'price';

export interface BrowseFilter {
  search: string;
  type: MarketAssetType | '';
  category: string;
  priceModel: PriceModel | '';
  sort: BrowseSort;
}
