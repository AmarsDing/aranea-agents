-- Version 20261019: index for the P1-3 compensation sweeper's ListPending query
-- (WHERE published_at IS NULL [AND created_at < ?] ORDER BY created_at LIMIT n).
--
-- Without it the sweep runs a full sequential scan of event_delivery_outbox
-- every 10s; the table only grows until session cascade-delete, so the scan
-- cost degrades linearly with deployment age (finding #17).
--
-- Partial index. Works on both dialects: Postgres supports WHERE predicates on
-- indexes natively; SQLite supports partial indexes since 3.8 and treats
-- '' = published_at='' legacy rows as simply not indexed (they stay
-- discoverable via the primary-key lookup in MarkPublished, and new writers
-- always store NULL for unpublished rows, so the pending set the sweeper
-- cares about is fully indexed). A predicate including OR published_at=''
-- would also be valid on both dialects, but legacy '' rows only exist on
-- SQLite test/CLI databases where the table is tiny — the IS NULL predicate
-- keeps the production index minimal.

CREATE INDEX IF NOT EXISTS idx_event_delivery_outbox_pending
  ON event_delivery_outbox(created_at)
  WHERE published_at IS NULL;
