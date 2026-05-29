package session

import (
	"context"
	"strings"
	"time"
)

const (
	// SessionBatchPageSize is the page size when scanning scope candidates.
	SessionBatchPageSize = 1000
	// SessionBatchMaxScan caps total sessions scanned in retention mode (100 pages).
	SessionBatchMaxScan = 100000
)

// SessionBatchScope filters sessions for batch archive/delete (same fields as search).
type SessionBatchScope struct {
	OwnerType     string
	AgentID       string
	TeamID        string
	Status        string
	ContextStatus string
	Keyword       string
	UserID        string
}

// SessionBatchPreview is dry-run output for batch operations.
type SessionBatchPreview struct {
	Matched         int
	SkippedRunning  int
	SkippedNotFound int
	Truncated       bool
	SampleIDs       []string
}

// SessionBatchResult is the outcome of batch archive/delete.
type SessionBatchResult struct {
	Matched         int
	Processed       int
	SkippedRunning  int
	SkippedNotFound int
	Truncated       bool
	FailedIDs       []string
}

func effectiveActivityAt(s Session) time.Time {
	for _, raw := range []string{s.LastMessageAt, s.UpdatedAt, s.CreatedAt} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil && !t.IsZero() {
			return t.UTC()
		}
	}
	return time.Time{}
}

func resolveBatchTargets(sessions []Session, mode string, olderThanDays int, includeArchived bool) (matched []string, skippedRunning int) {
	var cutoff time.Time
	if olderThanDays > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -olderThanDays)
	}
	for _, s := range sessions {
		if strings.TrimSpace(s.DeletedAt) != "" {
			continue
		}
		if s.Status == "running" {
			skippedRunning++
			continue
		}
		if mode == "archive" && s.Status == "archived" {
			continue
		}
		if mode == "delete" && s.Status == "archived" && !includeArchived {
			continue
		}
		if olderThanDays > 0 {
			at := effectiveActivityAt(s)
			if at.IsZero() || !at.Before(cutoff) {
				continue
			}
		}
		matched = append(matched, s.ID)
	}
	return matched, skippedRunning
}

func sampleIDs(ids []string, n int) []string {
	if n <= 0 || len(ids) == 0 {
		return nil
	}
	if len(ids) <= n {
		out := make([]string, len(ids))
		copy(out, ids)
		return out
	}
	return append([]string(nil), ids[:n]...)
}

func scopeToSearchQuery(scope SessionBatchScope, limit, offset int) SessionSearchQuery {
	if limit <= 0 {
		limit = SessionBatchPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return SessionSearchQuery{
		OwnerType:     scope.OwnerType,
		AgentID:       scope.AgentID,
		TeamID:        scope.TeamID,
		Status:        scope.Status,
		ContextStatus: scope.ContextStatus,
		Keyword:       scope.Keyword,
		UserID:        scope.UserID,
		Limit:         limit,
		Offset:        offset,
	}
}

func validateBatchParams(idCount int, olderThanDays int) error {
	if idCount == 0 && olderThanDays < 1 {
		return validationErr("ids or older_than_days >= 1 is required")
	}
	if olderThanDays < 0 {
		return validationErr("older_than_days must be >= 0")
	}
	return nil
}

type batchLoadResult struct {
	sessions    []Session
	notFoundIDs []string
	truncated   bool
}

func (uc *SessionUsecase) loadBatchCandidates(ctx context.Context, ids []string, scope SessionBatchScope) (batchLoadResult, error) {
	if len(ids) > 0 {
		trimmed := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				trimmed = append(trimmed, id)
			}
		}
		out, err := uc.sessionReader.ListSessionsByIDs(ctx, trimmed)
		if err != nil {
			return batchLoadResult{}, err
		}
		found := make(map[string]struct{}, len(out))
		for _, s := range out {
			found[s.ID] = struct{}{}
		}
		var notFound []string
		for _, id := range trimmed {
			if _, ok := found[id]; !ok {
				notFound = append(notFound, id)
			}
		}
		return batchLoadResult{sessions: out, notFoundIDs: notFound}, nil
	}
	sessions, truncated, err := uc.loadBatchCandidatesByScope(ctx, scope)
	if err != nil {
		return batchLoadResult{}, err
	}
	return batchLoadResult{sessions: sessions, truncated: truncated}, nil
}

func (uc *SessionUsecase) loadBatchCandidatesByScope(ctx context.Context, scope SessionBatchScope) ([]Session, bool, error) {
	var all []Session
	offset := 0
	for len(all) < SessionBatchMaxScan {
		batch, err := uc.sessionReader.ListSessionsForBatch(ctx, scopeToSearchQuery(scope, SessionBatchPageSize, offset))
		if err != nil {
			return nil, false, err
		}
		all = append(all, batch...)
		if len(batch) < SessionBatchPageSize {
			return all, false, nil
		}
		offset += SessionBatchPageSize
	}
	return all, true, nil
}

func (uc *SessionUsecase) resolveBatchOperation(ctx context.Context, mode string, ids []string, olderThanDays int, scope SessionBatchScope, includeArchived bool) ([]string, int, int, bool, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode != "archive" && mode != "delete" {
		return nil, 0, 0, false, validationErr("mode must be archive or delete")
	}
	if err := validateBatchParams(len(ids), olderThanDays); err != nil {
		return nil, 0, 0, false, err
	}
	loaded, err := uc.loadBatchCandidates(ctx, ids, scope)
	if err != nil {
		return nil, 0, 0, false, err
	}
	matched, skipped := resolveBatchTargets(loaded.sessions, mode, olderThanDays, includeArchived)
	return matched, skipped, len(loaded.notFoundIDs), loaded.truncated, nil
}

// PreviewBatch returns matched session ids for archive/delete without mutating.
func (uc *SessionUsecase) PreviewBatch(ctx context.Context, mode string, ids []string, olderThanDays int, scope SessionBatchScope, includeArchived bool) (SessionBatchPreview, error) {
	matched, skipped, notFound, truncated, err := uc.resolveBatchOperation(ctx, mode, ids, olderThanDays, scope, includeArchived)
	if err != nil {
		return SessionBatchPreview{}, err
	}
	return SessionBatchPreview{
		Matched:         len(matched),
		SkippedRunning:  skipped,
		SkippedNotFound: notFound,
		Truncated:       truncated,
		SampleIDs:       sampleIDs(matched, 5),
	}, nil
}

// BatchArchive archives sessions by ids and/or retention cutoff.
func (uc *SessionUsecase) BatchArchive(ctx context.Context, ids []string, olderThanDays int, scope SessionBatchScope) (SessionBatchResult, error) {
	matched, skipped, notFound, truncated, err := uc.resolveBatchOperation(ctx, "archive", ids, olderThanDays, scope, false)
	if err != nil {
		return SessionBatchResult{}, err
	}
	if len(matched) == 0 {
		return SessionBatchResult{
			SkippedRunning:  skipped,
			SkippedNotFound: notFound,
			Truncated:       truncated,
		}, nil
	}
	processed, failed, err := uc.sessionBatchMutator.ArchiveSessionsByIDs(ctx, matched)
	if err != nil {
		return SessionBatchResult{}, err
	}
	return SessionBatchResult{
		Matched:         len(matched),
		Processed:       processed,
		SkippedRunning:  skipped,
		SkippedNotFound: notFound,
		Truncated:       truncated,
		FailedIDs:       failed,
	}, nil
}

// BatchDelete soft-deletes sessions by ids and/or retention cutoff.
func (uc *SessionUsecase) BatchDelete(ctx context.Context, ids []string, olderThanDays int, scope SessionBatchScope, includeArchived bool) (SessionBatchResult, error) {
	matched, skipped, notFound, truncated, err := uc.resolveBatchOperation(ctx, "delete", ids, olderThanDays, scope, includeArchived)
	if err != nil {
		return SessionBatchResult{}, err
	}
	if len(matched) == 0 {
		return SessionBatchResult{
			SkippedRunning:  skipped,
			SkippedNotFound: notFound,
			Truncated:       truncated,
		}, nil
	}
	processed, failed, err := uc.sessionBatchMutator.DeleteSessionsByIDs(ctx, matched)
	if err != nil {
		return SessionBatchResult{}, err
	}
	return SessionBatchResult{
		Matched:         len(matched),
		Processed:       processed,
		SkippedRunning:  skipped,
		SkippedNotFound: notFound,
		Truncated:       truncated,
		FailedIDs:       failed,
	}, nil
}
