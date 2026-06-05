# skill-progressive-loading Specification

## Purpose
TBD - created by archiving change fix-progressive-loading-review. Update Purpose after archive.
## Requirements
### Requirement: Progressive mode hook does not load full guidance
In progressive mode, `newProgressiveSkillGuidanceHook` SHALL NOT call `BatchGetSkillGuidance`. It SHALL only write routed skill slugs to invocation state and let `SkillsRequestProcessor.injectOverview` handle all Skill overview display.

#### Scenario: Progressive hook writes state without loading body
- **WHEN** an agent has `skill_load_mode = "progressive"` and a user message triggers skill routing
- **THEN** the progressive hook writes `result.Slugs` to `RoutedSkillsStateKey` and `result.Reasons` to `skillSelectionReasonStateKey` in invocation state
- **AND** the hook does NOT call `BatchGetSkillGuidance`
- **AND** the hook does NOT inject any system message containing Skill Name/Description lists

#### Scenario: Non-progressive mode unchanged
- **WHEN** an agent has `skill_load_mode = "turn"` (or any non-progressive mode)
- **THEN** `newSkillGuidanceBeforeHook` behaves exactly as before, calling `BatchGetSkillGuidance` and injecting guidance into system prompt

### Requirement: IsProgressiveSkillLoad is case-insensitive
`IsProgressiveSkillLoad` SHALL use case-insensitive comparison with whitespace trimming, consistent with the framework's `normalizeSkillLoadMode`.

#### Scenario: Uppercase variant recognized
- **WHEN** `IsProgressiveSkillLoad("Progressive")` is called
- **THEN** it returns `true`

#### Scenario: Mixed case with whitespace recognized
- **WHEN** `IsProgressiveSkillLoad(" progressive ")` is called
- **THEN** it returns `true`

#### Scenario: Non-progressive mode rejected
- **WHEN** `IsProgressiveSkillLoad("turn")` is called
- **THEN** it returns `false`

### Requirement: GetSkillLoadMode default value is empty string
`AgentRuntimeSettings.GetSkillLoadMode()` SHALL return `""` when `SkillLoadMode` is empty or nil, allowing the framework layer to normalize to the default mode ("turn"). The `SkillLoadModeEager` constant SHALL be removed.

#### Scenario: Empty SkillLoadMode returns empty string
- **WHEN** `GetSkillLoadMode()` is called on an `AgentRuntimeSettings` with empty `SkillLoadMode`
- **THEN** it returns `""`

#### Scenario: Nil settings returns empty string
- **WHEN** `GetSkillLoadMode()` is called on a nil `AgentRuntimeSettings`
- **THEN** it returns `""`

#### Scenario: Explicit mode returned as-is
- **WHEN** `GetSkillLoadMode()` is called on settings with `SkillLoadMode = "progressive"`
- **THEN** it returns `"progressive"`

