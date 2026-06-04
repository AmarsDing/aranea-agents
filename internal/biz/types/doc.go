// Package types defines cross-module shared type foundations for the biz layer.
//
// This package serves as the single source of truth for NEW types that are
// shared across multiple biz sub-packages (monitor, session, tools, etc.).
// Existing canonical types that already live in the parent biz package
// (e.g., SkillHealth, ToolWeightReport, HealRecord, SessionStatus) remain
// in their current locations — they cannot be moved here due to Go's
// circular import restriction (biz/types is a sub-package of biz).
//
// Rules for adding types to this package:
//   - Only types that are genuinely shared across 2+ biz sub-packages belong here.
//   - Module-internal types stay in their own package.
//   - This package MUST NOT import the parent biz package (circular import).
//   - This package MUST NOT import trpc-agent-go or api proto packages.
package types
