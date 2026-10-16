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
