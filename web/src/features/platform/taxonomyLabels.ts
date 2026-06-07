/**
 * PGO-1-UI: Category description label & placeholder helpers.
 * Switches UI copy based on category level (1=company, 2=dept, 3=position).
 */
import { categoryDescriptionLabel, categoryDescriptionPlaceholder, type FieldScope } from '../agents/fieldGuides';

/** Level to FieldScope mapping. */
const levelToScope: Record<1 | 2 | 3, FieldScope> = {
  1: 'category.company',
  2: 'category.department',
  3: 'category.position',
};

/**
 * Returns the form label for the description field based on category level.
 * 1 → "公司说明", 2 → "部门职责", 3 → "岗位职责"
 */
export function descriptionLabel(level: 1 | 2 | 3): string {
  return categoryDescriptionLabel(level);
}

/**
 * Returns the textarea placeholder for the description field based on level.
 */
export function descriptionPlaceholder(level: 1 | 2 | 3): string {
  return categoryDescriptionPlaceholder(level);
}

/**
 * Returns the FieldScope for a category level.
 */
export function levelScope(level: 1 | 2 | 3): FieldScope {
  return levelToScope[level];
}

/**
 * Parses a raw level string (e.g. "company" | "department" | "position")
 * to the numeric 1 | 2 | 3 representation.
 */
export function parseLevelNumber(level: string): 1 | 2 | 3 {
  switch (level) {
    case 'company':
      return 1;
    case 'department':
      return 2;
    case 'position':
      return 3;
    default:
      return 1;
  }
}
