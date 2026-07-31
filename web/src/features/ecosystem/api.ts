import { createEcosystemService } from '../../services';
import type { Product } from '../../services/kratos/ecosystem/v1/index';
import type {
  BrowseFilter,
  CategoryNode,
  EcosystemProduct,
  MarketAsset,
  MarketCreator,
  MyInstall,
  MyOrder,
  OrgPickNode,
  StudioAsset,
  StudioInboxItem,
  StudioStats,
} from './types';
import {
  MOCK_ASSETS,
  MOCK_CATEGORIES,
  MOCK_CREATORS,
  MOCK_MY_INSTALLS,
  MOCK_MY_ORDERS,
  MOCK_ORG_PICK_TREE,
  MOCK_STUDIO_ASSETS,
  MOCK_STUDIO_INBOX,
  MOCK_STUDIO_STATS,
} from './mock';

function mapProduct(raw: Product): EcosystemProduct {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    display_name: String(raw.displayName ?? ''),
    description: String(raw.description ?? ''),
    type: String(raw.type ?? ''),
    version: String(raw.version ?? ''),
    install_count: Number(raw.installCount ?? 0),
    installed: Boolean(raw.installed ?? false),
  };
}

// ── M30 既有真实 RPC（保留不变） ────────────────────────────────

export async function listEcosystemProducts(search = ''): Promise<EcosystemProduct[]> {
  const svc = createEcosystemService();
  const res = await svc.ListProducts({ search: search || undefined, limit: 100, type: undefined, offset: undefined });
  const items = res.items ?? [];
  return items.map(mapProduct);
}

export async function installEcosystemProduct(id: string): Promise<void> {
  const svc = createEcosystemService();
  await svc.InstallProduct({ id });
}

export async function publishEcosystemProduct(input: {
  name: string;
  display_name: string;
  description: string;
  type: string;
}): Promise<EcosystemProduct> {
  const svc = createEcosystemService();
  const res = await svc.PublishProduct({
    name: input.name,
    displayName: input.display_name,
    description: input.description,
    type: input.type,
    version: undefined,
    priceModel: undefined,
    priceCents: undefined,
    configJson: undefined,
  });
  return mapProduct(res);
}

// ── M57 商城新能力（骨架期 mock） ──────────────────────────────

/** 浏览页：分类树 */
export async function listCategories(): Promise<CategoryNode[]> {
  // MOCK 延迟 50ms，模拟网络
  await new Promise((r) => setTimeout(r, 50));
  return JSON.parse(JSON.stringify(MOCK_CATEGORIES));
}

/** 浏览页：商品列表（支持搜索/类型/分类/价格/排序） */
export async function searchAssets(filter: BrowseFilter): Promise<MarketAsset[]> {
  await new Promise((r) => setTimeout(r, 80));
  let list = MOCK_ASSETS.slice();
  if (filter.search) {
    const q = filter.search.toLowerCase();
    list = list.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        a.tagline.toLowerCase().includes(q) ||
        a.description.toLowerCase().includes(q) ||
        a.creator.name.toLowerCase().includes(q),
    );
  }
  if (filter.type) {
    list = list.filter((a) => a.type === filter.type);
  }
  if (filter.category) {
    list = list.filter((a) => a.categories.some((c) => c.includes(filter.category)));
  }
  if (filter.priceModel) {
    list = list.filter((a) => a.priceModel === filter.priceModel);
  }
  switch (filter.sort) {
    case 'new':
      list.sort((a, b) => b.publishedAt.localeCompare(a.publishedAt));
      break;
    case 'rating':
      list.sort((a, b) => b.rating - a.rating);
      break;
    case 'installs':
      list.sort((a, b) => b.installCount - a.installCount);
      break;
    case 'activity':
      list.sort((a, b) => b.activity30d - a.activity30d);
      break;
    case 'price':
      list.sort((a, b) => a.priceCents - b.priceCents);
      break;
    default:
      // hot：加权综合
      list.sort((a, b) => b.installCount * a.rating - a.installCount * b.rating);
      break;
  }
  return JSON.parse(JSON.stringify(list));
}

/** 详情页：按 slug 获取资产 */
export async function getAsset(slug: string): Promise<MarketAsset | null> {
  await new Promise((r) => setTimeout(r, 50));
  const a = MOCK_ASSETS.find((x) => x.slug === slug);
  return a ? JSON.parse(JSON.stringify(a)) : null;
}

/** 创作者主页：按 handle 获取创作者信息（含作品列表） */
export async function getCreator(handle: string): Promise<{ creator: MarketCreator | null; assets: MarketAsset[] }> {
  await new Promise((r) => setTimeout(r, 50));
  const c = MOCK_CREATORS.find((x) => x.handle === handle);
  const assets = c ? MOCK_ASSETS.filter((a) => a.creator.handle === handle) : [];
  return { creator: c ? JSON.parse(JSON.stringify(c)) : null, assets: JSON.parse(JSON.stringify(assets)) };
}

/** 创作者中心：统计数据 */
export async function getStudioStats(): Promise<StudioStats> {
  await new Promise((r) => setTimeout(r, 40));
  return JSON.parse(JSON.stringify(MOCK_STUDIO_STATS));
}

/** 创作者中心：我的资产 */
export async function listStudioAssets(): Promise<StudioAsset[]> {
  await new Promise((r) => setTimeout(r, 40));
  return JSON.parse(JSON.stringify(MOCK_STUDIO_ASSETS));
}

/** 创作者中心：评论收件箱 */
export async function listStudioInbox(): Promise<StudioInboxItem[]> {
  await new Promise((r) => setTimeout(r, 40));
  return JSON.parse(JSON.stringify(MOCK_STUDIO_INBOX));
}

/** 买家工作台：我的安装 */
export async function listMyInstalls(): Promise<MyInstall[]> {
  await new Promise((r) => setTimeout(r, 50));
  return JSON.parse(JSON.stringify(MOCK_MY_INSTALLS));
}

/** 买家工作台：我的订单 */
export async function listMyOrders(): Promise<MyOrder[]> {
  await new Promise((r) => setTimeout(r, 50));
  return JSON.parse(JSON.stringify(MOCK_MY_ORDERS));
}

/** 发布向导：组织架构可选节点 */
export async function getOrgPickTree(): Promise<OrgPickNode[]> {
  await new Promise((r) => setTimeout(r, 50));
  return JSON.parse(JSON.stringify(MOCK_ORG_PICK_TREE));
}

/** 详情页：提交评分 */
export async function rateAsset(_assetId: string, _rating: number, _content: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 200));
}

/** 详情页：安装/卸载资产 */
export async function installAsset(_assetId: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 300));
}

export async function uninstallAsset(_assetId: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 300));
}

/** 创作者中心：发送评论回复 */
export async function replyReview(_reviewId: string, _content: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 200));
}
