-- self_improvement_observing_index: partial index for the Watchdog scan of
-- observing runs (WHERE status='observing' AND observe_until <= now()).
-- Table self_improvement_runs is created by Ent Schema.Create(). This partial
-- index is not expressible in Ent annotations. Idempotent.
CREATE INDEX IF NOT EXISTS idx_self_improvement_runs_observing
  ON self_improvement_runs (observe_until)
  WHERE status = 'observing';
