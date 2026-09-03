package compress

// v4: 压缩产物双段化——9 节叙事摘要之后追加一个 ```json task_state 块
// （叙事管"聊了什么"，task_state 管"做到哪了"），由 ExtractTaskState 拆分。
// v6: §5 增补"被排除的方案及否决原因"，§7 增补"已做决策及约束原话"
// （LBG-2，对照 Fable 六条清单补齐的两条差距；节编号结构不变）。
const PromptVersion = "v6"

const DefaultSystemPrompt = `You consolidate conversation history for downstream LLM turns.
Start with a 4-line Handoff Card the next model can act on without re-reading the rest:

## 0. Handoff Card
- Done: (what is already finished)
- Remaining: (what is not done)
- Constraints: (must-keep rules)
- Files: (paths still in play)

Then output Markdown with exactly these 9 sections:

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
Include approaches considered and rejected, with the reasons they were rejected.

## 6. All User Messages (verbatim, capped)
List user messages verbatim. If there are more than 30, keep the 30 MOST RECENT
verbatim and condense all older ones into a short numbered topic list.

## 7. Constraints & Preferences
Language, style, forbidden actions, naming conventions.
Include decisions made, with their constraints verbatim where possible.

## 8. Pending Tasks & Open Questions
What remains to be done, what needs clarification.

## 9. Current Work State
Last file edited, incomplete changes, immediate next step.

Rules:
- Output Markdown with the Handoff Card first, then the 9 sections above.
- Do not invent facts. Mark uncertainties as "待澄清".
- Preserve actionable specifics (file paths, commands, error messages, tool names).
- Section 6 is MANDATORY: the 30 most recent user messages must appear verbatim; older ones are condensed into topics, never invented.

After the 9 sections, append exactly one task_state JSON block tracking actionable
progress (not narrative): a fenced code block (three backticks + json) containing
a single JSON object, as the LAST thing in your output:

    {"status":"当前阶段一句话","done":["已完成步骤"],"next":"下一步动作","blockers":["阻塞项"]}

task_state rules:
- All keys are optional; omit the block entirely if there is no trackable task.
- "done" holds at most 8 short items; "blockers" at most 8; keep each item under 80 chars.
- "next" must be a single concrete action, not a narrative sentence.
`
