package service

import (
	"context"
)

// streamPreviewUpdater patches in-place IM preview messages during a channel turn.
type streamPreviewUpdater interface {
	Update(ctx context.Context, recipient, text string, force bool) error
}
