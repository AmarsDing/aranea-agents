export interface VariantInfo {
  key: string;
  label: string;
}

export interface Company {
  id: string;
  key: string;
  name: string;
  icon: string;
  description: string;
  scenario_key: string;
  enabled: boolean;
  sort_order: number;
  /**
   * 客户端并行计算填充（useOrgMarket.fetchCompanies 内部）：
   * 部门 / 岗位 / Agent 数量。当前后端 list endpoint 不返回，故为可选。
   * 未来后端聚合时移除并行 fetch。
   */
  deptCount?: number;
  posCount?: number;
  agentCount?: number;
  /** 已部署实例数（运营侧统计）。当前类型未提供，固定 0。 */
  installed?: number;
}

export interface PositionPromptResult {
  promptContent: string;
  variant: string;
  positionName: string;
  departmentName: string;
  companyName: string;
  companyDescription: string;
  departmentDescription: string;
  positionDescription: string;
  responsibilitiesJson: string;
  variantDescription: string;
}

export interface Department {
  id: string;
  key: string;
  name: string;
  company_key: string;
  description: string;
  responsibilities_json: string;
  sort_order: number;
}

export interface Position {
  id: string;
  key: string;
  name: string;
  department_key: string;
  description: string;
  responsibilities_json: string;
  skills_required_json: string;
  seniority_level: string;
  sort_order: number;
}
