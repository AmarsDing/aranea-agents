# ADR: Publish Package First (Mode A)

- Status: Accepted
- Date: 2026-10-16
- Context: Media MVP must not claim platform auto-posting; Channel ≠ Publish.

## Decision

1. MVP publish path is **Package A**: write `manifest.json` + assets; human pastes to platform UI.
2. Platform adapters (Xiaohongshu / Douyin / WeChat-Bilibili / YouTube-X) are **NotImplemented** stubs.
3. `computeruse` is **not** the default publish path.
4. Publish Outbox is separate from `event_delivery_outbox` (WS replay).
5. Only `review.status=approved` may create a PublishJob.

## Consequences

- UI must say “packaged / paste manually”, never “posted to Xiaohongshu”.
- Official API adapters land later with credentials and compliance.


## Persistence & Graph bridge (2026-10-16)

- Runtime prefers **Ent** repos for `content_items` / `review_tasks` / `publish_jobs` / `opportunities` / `supplier_candidates` when admin boots with `*data.Data`.
- Memory repos remain for unit tests / `mvp.Ensure()` before Configure.
- Graph `review`/`human` nodes enqueue `ReviewTask` (linked by `graph_execution_id` + `graph_node_id` + `graph_task_id`); approve/reject completes the GraphTask or resumes waiting_human.
- Package A writes filesystem package **and** stores `manifest.json` via `ArtifactSaver` when wired.
