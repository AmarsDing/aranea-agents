package compress

// v3: Section 6 设上限（最近 30 条逐字，更早的压缩为主题列表）——无上限时
// 长会话中 Section 6 自身就会撑爆摘要，使压缩失去意义。
const PromptVersion = "v3"

const DefaultSystemPrompt = `You consolidate conversation history for downstream LLM turns.
Output Markdown with exactly these 9 sections:

## 1. User Intent & Goals
What the user wants to accomplish.

## 2. Key Technical Concepts
Technologies, frameworks, patterns discussed.

## 3. Files & Code Involved
File paths, function names, key variables, API endpoints.

## 4. Errors & Fixes
Error messages, root causes, solutions applied.

## 5. Problem-Solving Process
Steps taken, approaches tried, what worked and what didn't.

## 6. All User Messages (verbatim, capped)
List user messages verbatim. If there are more than 30, keep the 30 MOST RECENT
verbatim and condense all older ones into a short numbered topic list.

## 7. Constraints & Preferences
Language, style, forbidden actions, naming conventions.

## 8. Pending Tasks & Open Questions
What remains to be done, what needs clarification.

## 9. Current Work State
Last file edited, incomplete changes, immediate next step.

Rules:
- Output Markdown only with the 9 sections above.
- Do not invent facts. Mark uncertainties as "待澄清".
- Preserve actionable specifics (file paths, commands, error messages, tool names).
- Section 6 is MANDATORY: the 30 most recent user messages must appear verbatim; older ones are condensed into topics, never invented.
`
