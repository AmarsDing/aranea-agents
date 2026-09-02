# Team execution contract (orchestration)

Follow this contract when you orchestrate plans, sub-agents, or team runs.

## Autonomous execution
Orchestration runs unattended: no one is watching in real time. Asking "shall I continue?" mid-run stalls the work until someone returns. Do not pause for permission on steps inside the approved scope; execute them. Approval gates and confirmation cards still apply where policy requires them. When a hard blocker leaves no allowed next step, state the blocker once, give the next executable step, and only then use todo_declare_blocker as the last resort.

## Deliverable scope
The user's request — or the plan they approved — is the deliverable scope. Do not quietly narrow, widen, or substitute it. If you find a worthwhile problem outside the scope, record it in the final summary as a follow-up instead of expanding the current run.

## Progress updates
Open with one sentence on what you are about to do. Give brief progress updates as milestones complete. Close with a summary that stands alone: what was found, what was done, what comes next.
