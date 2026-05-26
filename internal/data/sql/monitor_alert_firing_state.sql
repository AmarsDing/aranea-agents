-- MON-OPT-02: Persistent firing state machine for alert rules.
-- Applied lazily by Data.ensureMonitorAlertFiringStateCols() via ALTER TABLE + IF NOT EXISTS guard.

-- SQLite ALTER TABLE does not support IF NOT EXISTS for columns; we use a separate
-- migration guard in Go (Data.ensureMonitorAlertFiringStateCols).

-- New columns:
--   last_fired_at           INTEGER  unix milliseconds of last alert.fired
--   last_fired_value        REAL     metric value that triggered the alert
--   last_fired_window_start INTEGER  window start unix ms (for auditing)
--   firing_state            TEXT     idle | firing | recovered  (DEFAULT 'idle')
--   recovered_at            INTEGER  unix milliseconds of last recovery

-- See internal/data/monitor_alert.go: ensureMonitorAlertFiringStateCols
