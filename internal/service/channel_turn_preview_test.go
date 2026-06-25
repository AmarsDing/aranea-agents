package service

import (
	"context"
)

type mockPreviewUpdater struct {
	calls []string
	force []bool
	msgID string
}

func (m *mockPreviewUpdater) Update(_ context.Context, _, text string, force bool) error {
	m.calls = append(m.calls, text)
	m.force = append(m.force, force)
	if m.msgID == "" {
		m.msgID = "om_preview_1"
	}
	return nil
}

func (m *mockPreviewUpdater) PreviewMessageID() string { return m.msgID }

// Phase 1c-5: TestTurnPreviewCoordinatorTextThenTool removed — tested deleted
// EnvelopeType TextDelta/ToolCall behavior in consume() (now a no-op).
// Phase 1c-5: TestTurnPreviewCoordinator_heartbeatPreservesTranscript removed —
// tested deleted EnvelopeType TextDone behavior.
