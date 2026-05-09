package compress

// PromptVersion labels the built-in system prompt for logs and regression.
const PromptVersion = "v1"

// DefaultSystemPrompt is the default summarization instruction (Markdown out).
const DefaultSystemPrompt = `You consolidate conversation history for downstream LLM turns.
Rules:
- Output Markdown only. Use clear headings: User intent / Confirmed facts / Constraints / Open tasks / Important details (paths, numbers, APIs).
- Do not invent facts. Mark uncertainties as "待澄清".
- Preserve actionable specifics (file paths, commands, error messages, tool names).
- The transcript may include USER and ASSISTANT lines only; summarize faithfully.
`
