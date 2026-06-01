package biz

import "context"

type ToolResultBlob struct {
	ID               string
	SessionID        string
	TurnNumber       int
	ToolName         string
	ToolArgsSummary  string
	FullContent      string
	ContentSizeChars int
	CreatedAt        string
}

type ToolResultReplacement struct {
	ID            string
	SessionID     string
	MessageID     string
	ResultBlobID  string
	PreviewText   string
	ReplacedAt    string
}

type ToolResultBlobReader interface {
	GetBlob(ctx context.Context, id string) (*ToolResultBlob, error)
	ListBlobsBySession(ctx context.Context, sessionID string, fromTurn int) ([]*ToolResultBlob, error)
}

type ToolResultBlobWriter interface {
	SaveBlob(ctx context.Context, blob *ToolResultBlob) error
}

type ToolResultReplacementReader interface {
	GetReplacementByMessage(ctx context.Context, sessionID, messageID string) (*ToolResultReplacement, error)
}

type ToolResultReplacementWriter interface {
	SaveReplacement(ctx context.Context, r *ToolResultReplacement) error
}
