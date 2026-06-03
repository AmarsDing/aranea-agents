// TECH-DEBT: This file is deprecated. Import from "./taxonomyTreeUtils" instead.
// All exports are re-exported for backward compatibility during migration.
export {
  type CategoryLevel,
  type CategoryQTreeNode,
  levelLabel,
  parseIsSystem,
  trimmedDesc,
  flattenCategoryTree,
  findCategoryNode,
  findCategoryPath,
  formatCategoryPath,
  collectPositionIds,
  flattenCategoryPositions,
  nodeMatchesKeyword,
  departmentPositions,
  filterCategoryTree,
  collectExpandedIdsForFilter,
  categoryTreeStats,
  toQTreeNodes,
  inferCascadeFromPosition,
  patchCategoryTreeNode,
  collectDefaultExpandedIds
} from "./taxonomyTreeUtils";
