-- Version 20260620: Session revision column patch
ALTER TABLE sessions ADD COLUMN session_revision INTEGER NOT NULL DEFAULT 0;
