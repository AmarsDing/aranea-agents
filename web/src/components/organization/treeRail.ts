/** 编制树（组织架构 / Team 管理 / Agent 管理三页共用）的入参形状。数字由各页自己算：岗位数、团队数或在编人数。 */
export type TreeRailEntry = { key: string; icon: string; label: string; count?: number };

export type TreeRailDept = {
  id: string;
  name: string;
  enabled: boolean;
  count?: number;
  /** 琥珀小标签，如「缺 2」 */
  tag?: string;
  tagTitle?: string;
  /** 总经理办公室之类的治理部门用盾牌图标 */
  icon?: string;
};

export type TreeRailCompany = {
  id: string;
  name: string;
  enabled: boolean;
  count?: number;
  departments: TreeRailDept[];
};
