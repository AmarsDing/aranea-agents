// 生态产品领域类型（M30 真实 RPC 对应的精简类型）。
// M57 公网商城骨架（Market*/My*/Studio* 模型、mock 数据、浏览/详情/创作者页面）
// 已随商城域下线移除；后端实现后可参照 docs/development/57-marketplace-platform.design.md 重建。
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
