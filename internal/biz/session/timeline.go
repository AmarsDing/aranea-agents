package session

import "context"

// Timeline delegates to SessionTimelineUsecase (Facade pattern).
func (uc *SessionUsecase) Timeline(ctx context.Context, id string, q TimelineQuery) (SessionTimeline, error) {
	return uc.timelineUsecase.Timeline(ctx, id, q)
}
