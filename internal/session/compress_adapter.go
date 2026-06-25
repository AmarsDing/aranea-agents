package session

import (
	"context"

	"aranea-agents/internal/biz"
	bizsession "aranea-agents/internal/biz/session"
)

// compressReadDepsAdapter composes SessionReader + MessageReader + SummaryReader
// to satisfy CompressReadDeps after the messages table was removed (Phase 1c-3).
//
// SessionRepo provides SessionReader + SummaryReader; an Activity-backed
// MessageReader (biz/session.ActivityMessageReader) provides MessageReader.
type compressReadDepsAdapter struct {
	biz.SessionReader
	biz.MessageReader
	biz.SummaryReader
}

// ProvideCompressReadDepsAdapter constructs a CompressReadDeps from its parts.
// Returns nil if sessions is nil; Wire treats nil as a missing binding.
//
// If lister is nil (e.g., tests without ActivityReader wired), an empty
// ActivityLister is used so MessageReader calls return empty results instead
// of panicking.
func ProvideCompressReadDepsAdapter(
	sessions biz.SessionRepo,
	lister bizsession.ActivityLister,
) CompressReadDeps {
	if sessions == nil {
		return nil
	}
	if lister == nil {
		lister = emptyActivityLister{}
	}
	msgReader := bizsession.NewActivityMessageReader(lister)
	if msgReader == nil {
		msgReader = bizsession.NewActivityMessageReader(emptyActivityLister{})
	}
	return &compressReadDepsAdapter{
		SessionReader:  sessions,
		MessageReader:  msgReader,
		SummaryReader:  sessions,
	}
}

// compressWriteDepsAdapter composes MessageWriter + SummaryWriter + ContextUpdater
// to satisfy CompressWriteDeps after the messages table was removed (Phase 1c-3).
//
// SessionRepo provides SummaryWriter + ContextUpdater; NoopMessageWriter stubs
// MessageWriter (the ActivityProjector now persists all chat-shaped content).
type compressWriteDepsAdapter struct {
	biz.MessageWriter
	biz.SummaryWriter
	biz.ContextUpdater
}

// ProvideCompressWriteDepsAdapter constructs a CompressWriteDeps from its parts.
// Returns nil if sessions is nil; Wire treats nil as a missing binding.
func ProvideCompressWriteDepsAdapter(
	sessions biz.SessionRepo,
) CompressWriteDeps {
	if sessions == nil {
		return nil
	}
	return &compressWriteDepsAdapter{
		MessageWriter:  bizsession.NewNoopMessageWriter(),
		SummaryWriter:  sessions,
		ContextUpdater: sessions,
	}
}

// emptyActivityLister is a local noop ActivityLister used as a fallback when
// biz/session.ActivityLister is nil (e.g., tests/CLI without ActivityReader
// wired). It returns empty results for all calls.
type emptyActivityLister struct{}

func (emptyActivityLister) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]bizsession.ActivityEntry, error) {
	return nil, nil
}

func (emptyActivityLister) ListBySession(ctx context.Context, sessionID string) ([]bizsession.ActivityEntry, error) {
	return nil, nil
}

// Compile-time interface checks.
var (
	_ CompressReadDeps           = (*compressReadDepsAdapter)(nil)
	_ CompressWriteDeps          = (*compressWriteDepsAdapter)(nil)
	_ bizsession.ActivityLister  = emptyActivityLister{}
)
