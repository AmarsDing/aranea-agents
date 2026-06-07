import { createOrganizationService } from '../../services';
import type { OrganizationNode } from '../../services/kratos/organization/v1/index';
import type { Company, Department, Position, PositionPromptResult, VariantInfo } from './types';
import { pickStr } from '../../shared/wireJson';

/** 从 OrganizationNode wire 数据中安全取值 */
function asRecord(raw: unknown): Record<string, unknown> {
  return raw && typeof raw === 'object' && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {};
}

/** 将 OrganizationNode 映射为 Company 类型（level=company） */
function orgNodeToCompany(node: OrganizationNode): Company {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'orgKey', 'orgKey'),
    name: pickStr(r, 'name', 'name'),
    icon: '', // OrganizationNode 无 icon 字段，由前端默认处理
    description: pickStr(r, 'description', 'description'),
    scenario_key: '',
    enabled: Boolean(r.enabled ?? r.Enabled ?? true),
    sort_order: Number(r.sortOrder ?? r.sort_order ?? 0),
  };
}

/** 将 OrganizationNode 映射为 Department 类型（level=department） */
function orgNodeToDepartment(node: OrganizationNode): Department {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'orgKey', 'orgKey'),
    name: pickStr(r, 'name', 'name'),
    company_key: pickStr(r, 'parentId', 'parentId'), // parentId 即所属 company 的 id
    description: pickStr(r, 'description', 'description'),
    responsibilities_json: pickStr(r, 'metadataJson', 'metadataJson') || '',
    sort_order: Number(r.sortOrder ?? r.sort_order ?? 0),
  };
}

/** 将 OrganizationNode 映射为 Position 类型（level=position） */
function orgNodeToPosition(node: OrganizationNode): Position {
  const r = asRecord(node);
  return {
    id: pickStr(r, 'id', 'id'),
    key: pickStr(r, 'orgKey', 'orgKey'),
    name: pickStr(r, 'name', 'name'),
    department_key: pickStr(r, 'parentId', 'parentId'), // parentId 即所属 department 的 id
    description: pickStr(r, 'description', 'description'),
    responsibilities_json: pickStr(r, 'metadataJson', 'metadataJson') || '',
    skills_required_json: '',
    seniority_level: '',
    sort_order: Number(r.sortOrder ?? r.sort_order ?? 0),
  };
}

/** 获取所有 organization 节点（内部缓存） */
let _allNodesCache: OrganizationNode[] | null = null;

async function fetchAllNodes(): Promise<OrganizationNode[]> {
  if (_allNodesCache) return _allNodesCache;
  const svc = createOrganizationService();
  const res = await svc.ListOrganization({});
  const items = (res.items ?? []) as unknown as OrganizationNode[];
  _allNodesCache = items;
  return items;
}

/** 清除缓存（用于刷新场景） */
export function invalidateCache(): void {
  _allNodesCache = null;
}

export async function listCompanies(): Promise<{ items: Company[]; total: number }> {
  const items = await fetchAllNodes();
  const companies = items
    .filter((n) => {
      const r = asRecord(n);
      const level = pickStr(r, 'level', 'level');
      return level === 'company';
    })
    .map(orgNodeToCompany);
  return { items: companies, total: companies.length };
}

export async function getCompany(key: string): Promise<Company> {
  const items = await fetchAllNodes();
  const node = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'orgKey', 'orgKey') === key && pickStr(r, 'level', 'level') === 'company';
  });
  if (!node) throw new Error(`Company not found: ${key}`);
  return orgNodeToCompany(node);
}

export async function listDepartments(companyKey: string): Promise<{ items: Department[]; total: number }> {
  const items = await fetchAllNodes();
  // 先找到 company 节点的 id
  const companyNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'orgKey', 'orgKey') === companyKey && pickStr(r, 'level', 'level') === 'company';
  });
  if (!companyNode) return { items: [], total: 0 };
  const companyId = pickStr(asRecord(companyNode), 'id', 'id');
  const departments = items
    .filter((n) => {
      const r = asRecord(n);
      return pickStr(r, 'parentId', 'parentId') === companyId && pickStr(r, 'level', 'level') === 'department';
    })
    .map(orgNodeToDepartment);
  return { items: departments, total: departments.length };
}

export async function listPositions(
  companyKey: string,
  departmentKey?: string,
): Promise<{ items: Position[]; total: number }> {
  const items = await fetchAllNodes();

  // 如果指定了 departmentKey，直接按 department 的 parentId 过滤
  if (departmentKey) {
    const deptNode = items.find((n) => {
      const r = asRecord(n);
      return pickStr(r, 'orgKey', 'orgKey') === departmentKey && pickStr(r, 'level', 'level') === 'department';
    });
    if (!deptNode) return { items: [], total: 0 };
    const deptId = pickStr(asRecord(deptNode), 'id', 'id');
    const positions = items
      .filter((n) => {
        const r = asRecord(n);
        return pickStr(r, 'parentId', 'parentId') === deptId && pickStr(r, 'level', 'level') === 'position';
      })
      .map(orgNodeToPosition);
    return { items: positions, total: positions.length };
  }

  // 否则返回该 company 下所有 positions（跨所有 departments）
  const companyNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'orgKey', 'orgKey') === companyKey && pickStr(r, 'level', 'level') === 'company';
  });
  if (!companyNode) return { items: [], total: 0 };
  const companyId = pickStr(asRecord(companyNode), 'id', 'id');

  // 找到该 company 下所有 department ids
  const deptIds = new Set(
    items
      .filter((n) => {
        const r = asRecord(n);
        return pickStr(r, 'parentId', 'parentId') === companyId && pickStr(r, 'level', 'level') === 'department';
      })
      .map((n) => pickStr(asRecord(n), 'id', 'id')),
  );

  const positions = items
    .filter((n) => {
      const r = asRecord(n);
      return deptIds.has(pickStr(r, 'parentId', 'parentId')) && pickStr(r, 'level', 'level') === 'position';
    })
    .map(orgNodeToPosition);
  return { items: positions, total: positions.length };
}

/**
 * 获取岗位 Prompt 内容
 *
 * 注意：后端 OrganizationService proto 暂未暴露 prompt/variants 端点，
 * 此函数从 organization 节点数据中提取描述信息构建 PositionPromptResult。
 * 待后端新增对应 HTTP 端点后切换为 API 调用。
 */
export async function getPositionPrompt(
  companyKey: string,
  positionKey: string,
  _variant?: string,
): Promise<PositionPromptResult> {
  const items = await fetchAllNodes();

  const posNode = items.find((n) => {
    const r = asRecord(n);
    return pickStr(r, 'orgKey', 'orgKey') === positionKey && pickStr(r, 'level', 'level') === 'position';
  });
  if (!posNode) {
    return {
      promptContent: '',
      variant: _variant || 'general',
      positionName: '',
      departmentName: '',
      companyName: '',
      companyDescription: '',
      departmentDescription: '',
      positionDescription: '',
      responsibilitiesJson: '',
      variantDescription: '',
    };
  }

  const pos = orgNodeToPosition(posNode);
  const deptId = pickStr(asRecord(posNode), 'parentId', 'parentId');

  const deptNode = deptId ? items.find((n) => pickStr(asRecord(n), 'id', 'id') === deptId) : undefined;
  const dept = deptNode ? orgNodeToDepartment(deptNode) : null;

  const compId = deptNode ? pickStr(asRecord(deptNode), 'parentId', 'parentId') : '';
  const compNode = compId ? items.find((n) => pickStr(asRecord(n), 'id', 'id') === compId) : undefined;
  const comp = compNode ? orgNodeToCompany(compNode) : null;

  return {
    promptContent: '',
    variant: _variant || 'general',
    positionName: pos.name,
    departmentName: dept?.name ?? '',
    companyName: comp?.name ?? '',
    companyDescription: comp?.description ?? '',
    departmentDescription: dept?.description ?? '',
    positionDescription: pos.description,
    responsibilitiesJson: pos.responsibilities_json,
    variantDescription: '',
  };
}

/**
 * 列出岗位的变体
 *
 * 注意：后端 OrganizationService proto 暂未暴露 variants 端点，
 * 当前返回默认的 general 变体。待后端新增对应 HTTP 端点后切换为 API 调用。
 */
export async function listPositionVariants(_companyKey: string, _positionKey: string): Promise<VariantInfo[]> {
  return [{ key: 'general', label: '通用' }];
}
