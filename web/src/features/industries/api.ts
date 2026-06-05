import { createTaxonomyService } from '../../services';
import type { TaxonomyNode } from '../../services/kratos/taxonomy/v1/index';
import type { Industry, Department, Position, PositionPromptResult, VariantInfo } from './types';
import { pickStr } from '../../shared/wireJson';

/** 从 TaxonomyNode wire 数据中安全取值 */
function asRecord(raw: unknown): Record<string, unknown> {
  return (raw && typeof raw === 'object' && !Array.isArray(raw)) ? raw as Record<string, unknown> : {};
}

/** 将 TaxonomyNode 映射为 Industry 类型（level=industry） */
function taxonomyNodeToIndustry(node: TaxonomyNode): Industry {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    icon: '', // TaxonomyNode 无 icon 字段，由前端默认处理
    description: pickStr(r, 'description', 'description'),
    scenario_key: '',
    enabled: Boolean(r.enabled ?? r.Enabled ?? true),
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
  };
}

/** 将 TaxonomyNode 映射为 Department 类型（level=department） */
function taxonomyNodeToDepartment(node: TaxonomyNode): Department {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    industry_key: pickStr(r, 'parent_id', 'parentId'), // parent_id 即所属 industry 的 id
    description: pickStr(r, 'description', 'description'),
    responsibilities_json: pickStr(r, 'metadata_json', 'metadataJson') || '',
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
  };
}

/** 将 TaxonomyNode 映射为 Position 类型（level=position） */
function taxonomyNodeToPosition(node: TaxonomyNode): Position {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'key', 'key'),
    name: pickStr(r, 'name', 'name'),
    department_key: pickStr(r, 'parent_id', 'parentId'), // parent_id 即所属 department 的 id
    description: pickStr(r, 'description', 'description'),
    responsibilities_json: pickStr(r, 'metadata_json', 'metadataJson') || '',
    skills_required_json: '',
    seniority_level: '',
    sort_order: Number(r.sort_order ?? r.sortOrder ?? 0),
  };
}

/** 获取所有 taxonomy 节点（内部缓存） */
let _allNodesCache: TaxonomyNode[] | null = null;

async function fetchAllNodes(): Promise<TaxonomyNode[]> {
  if (_allNodesCache) return _allNodesCache;
  const svc = createTaxonomyService();
  const res = await svc.ListTaxonomy({});
  const items = (res.items ?? []) as unknown as TaxonomyNode[];
  _allNodesCache = items;
  return items;
}

/** 清除缓存（用于刷新场景） */
export function invalidateCache(): void {
  _allNodesCache = null;
}

export async function listIndustries(): Promise<{ items: Industry[]; total: number }> {
  const items = await fetchAllNodes();
  const industries = items
    .filter((n) => {
      const r = asRecord(n);
      const level = pickStr(r, 'level', 'level');
      return level === 'industry';
    })
    .map(taxonomyNodeToIndustry);
  return { items: industries, total: industries.length };
}

export async function getIndustry(key: string): Promise<Industry> {
  const items = await fetchAllNodes();
  const node = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'key', 'key') === key && pickStr(r, 'level', 'level') === 'industry';
  });
  if (!node) throw new Error(`Industry not found: ${key}`);
  return taxonomyNodeToIndustry(node);
}

export async function listDepartments(industryKey: string): Promise<{ items: Department[]; total: number }> {
  const items = await fetchAllNodes();
  // 先找到 industry 节点的 id
  const industryNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'key', 'key') === industryKey && pickStr(r, 'level', 'level') === 'industry';
  });
  if (!industryNode) return { items: [], total: 0 };
  const industryId = pickStr(asRecord(industryNode), 'id', 'id');
  const departments = items
    .filter((n) => {
      const r = asRecord(n);
      return pickStr(r, 'parent_id', 'parentId') === industryId && pickStr(r, 'level', 'level') === 'department';
    })
    .map(taxonomyNodeToDepartment);
  return { items: departments, total: departments.length };
}

export async function listPositions(
  industryKey: string,
  departmentKey?: string,
): Promise<{ items: Position[]; total: number }> {
  const items = await fetchAllNodes();

  // 如果指定了 departmentKey，直接按 department 的 parent_id 过滤
  if (departmentKey) {
    const deptNode = items.find((n) => {
      const r = asRecord(n);
      return pickStr(r, 'key', 'key') === departmentKey && pickStr(r, 'level', 'level') === 'department';
    });
    if (!deptNode) return { items: [], total: 0 };
    const deptId = pickStr(asRecord(deptNode), 'id', 'id');
    const positions = items
      .filter((n) => {
        const r = asRecord(n);
        return pickStr(r, 'parent_id', 'parentId') === deptId && pickStr(r, 'level', 'level') === 'position';
      })
      .map(taxonomyNodeToPosition);
    return { items: positions, total: positions.length };
  }

  // 否则返回该 industry 下所有 positions（跨所有 departments）
  const industryNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'key', 'key') === industryKey && pickStr(r, 'level', 'level') === 'industry';
  });
  if (!industryNode) return { items: [], total: 0 };
  const industryId = pickStr(asRecord(industryNode), 'id', 'id');

  // 找到该 industry 下所有 department ids
  const deptIds = new Set(
    items
      .filter((n) => {
        const r = asRecord(n);
        return pickStr(r, 'parent_id', 'parentId') === industryId && pickStr(r, 'level', 'level') === 'department';
      })
      .map((n) => pickStr(asRecord(n), 'id', 'id')),
  );

  const positions = items
    .filter((n) => {
      const r = asRecord(n);
      return deptIds.has(pickStr(r, 'parent_id', 'parentId')) && pickStr(r, 'level', 'level') === 'position';
    })
    .map(taxonomyNodeToPosition);
  return { items: positions, total: positions.length };
}

/**
 * 获取岗位 Prompt 内容
 *
 * 注意：后端 TaxonomyService proto 暂未暴露 prompt/variants 端点，
 * 此函数从 taxonomy 节点数据中提取描述信息构建 PositionPromptResult。
 * 待后端新增对应 HTTP 端点后切换为 API 调用。
 */
export async function getPositionPrompt(
  industryKey: string,
  positionKey: string,
  _variant?: string,
): Promise<PositionPromptResult> {
  const items = await fetchAllNodes();

  const posNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'key', 'key') === positionKey && pickStr(r, 'level', 'level') === 'position';
  });
  if (!posNode) {
    return {
      promptContent: '',
      variant: _variant || 'general',
      positionName: '',
      departmentName: '',
      industryName: '',
      industryDescription: '',
      departmentDescription: '',
      positionDescription: '',
      responsibilitiesJson: '',
      variantDescription: '',
    };
  }

  const pos = taxonomyNodeToPosition(posNode);
  const deptId = pickStr(asRecord(posNode), 'parent_id', 'parentId');

  const deptNode = deptId ? items.find((n) => pickStr(asRecord(n), 'id', 'id') === deptId) : undefined;
  const dept = deptNode ? taxonomyNodeToDepartment(deptNode) : null;

  const indId = deptNode ? pickStr(asRecord(deptNode), 'parent_id', 'parentId') : '';
  const indNode = indId ? items.find((n) => pickStr(asRecord(n), 'id', 'id') === indId) : undefined;
  const ind = indNode ? taxonomyNodeToIndustry(indNode) : null;

  return {
    promptContent: '',
    variant: _variant || 'general',
    positionName: pos.name,
    departmentName: dept?.name ?? '',
    industryName: ind?.name ?? '',
    industryDescription: ind?.description ?? '',
    departmentDescription: dept?.description ?? '',
    positionDescription: pos.description,
    responsibilitiesJson: pos.responsibilities_json,
    variantDescription: '',
  };
}

/**
 * 列出岗位的变体
 *
 * 注意：后端 TaxonomyService proto 暂未暴露 variants 端点，
 * 当前返回默认的 general 变体。待后端新增对应 HTTP 端点后切换为 API 调用。
 */
export async function listPositionVariants(_industryKey: string, _positionKey: string): Promise<VariantInfo[]> {
  return [{ key: 'general', label: '通用' }];
}
