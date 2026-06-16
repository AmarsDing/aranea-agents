package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// syncCaptureBus is an event.Bus that captures envelopes with
// channel-based signaling for testing async publishers (safego.Go).
type syncCaptureBus struct {
	mu        sync.Mutex
	published []event.Envelope
	notify    chan struct{} // signaled on each Publish
}

func newSyncCaptureBus() *syncCaptureBus {
	return &syncCaptureBus{notify: make(chan struct{}, 64)}
}

func (b *syncCaptureBus) Publish(_ context.Context, env event.Envelope) {
	b.mu.Lock()
	b.published = append(b.published, env)
	b.mu.Unlock()
	// Non-blocking signal
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *syncCaptureBus) Subscribe(_ contract.SubscribeOptions) (<-chan event.Envelope, func()) {
	return nil, func() {}
}

func (b *syncCaptureBus) DropCount() uint64 { return 0 }

// waitForPublished waits until at least n envelopes have been published,
// then returns them. Times out after 2 seconds.
func (b *syncCaptureBus) waitForPublished(t *testing.T, n int) []event.Envelope {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		b.mu.Lock()
		count := len(b.published)
		b.mu.Unlock()
		if count >= n {
			b.mu.Lock()
			result := make([]event.Envelope, len(b.published))
			copy(result, b.published)
			b.mu.Unlock()
			return result
		}
		select {
		case <-b.notify:
		case <-timeout:
			b.mu.Lock()
			got := len(b.published)
			b.mu.Unlock()
			t.Fatalf("timed out waiting for %d published envelopes (got %d)", n, got)
			return nil
		}
	}
}

// reset clears captured envelopes.
func (b *syncCaptureBus) reset() {
	b.mu.Lock()
	b.published = nil
	b.mu.Unlock()
}

// mockActivityWriter implements biz.ActivityWriter for testing.
type mockActivityWriter struct {
	mu         sync.Mutex
	activities map[string]biz.Activity
}

func newMockActivityWriter() *mockActivityWriter {
	return &mockActivityWriter{activities: make(map[string]biz.Activity)}
}

func (m *mockActivityWriter) CreateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activities[a.ID] = a
	return a, nil
}

func (m *mockActivityWriter) UpdateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activities[a.ID] = a
	return a, nil
}

func (m *mockActivityWriter) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activities[a.ID] = a
	return a, nil
}

// newTestProjector creates an ActivityProjector with a syncCaptureBus and mock repo.
func newTestProjector() (*ActivityProjector, *syncCaptureBus, *mockActivityWriter) {
	bus := newSyncCaptureBus()
	repo := newMockActivityWriter()
	p := NewActivityProjector(bus, repo, loggateway.NewNoop())
	p.Reset() // initialize internal maps
	return p, bus, repo
}

// --- OnNotice ---

func TestOnNotice_createsActivityWithKindNotice(t *testing.T) {
	p, bus, _ := newTestProjector()

	act, err := p.OnNotice(context.Background(), "turn-1", "sess-1", "hello world", "info")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Kind != biz.ActivityKindNotice {
		t.Errorf("kind=%q want %q", act.Kind, biz.ActivityKindNotice)
	}
	if act.Content != "hello world" {
		t.Errorf("content=%q want %q", act.Content, "hello world")
	}
	if act.Status != biz.ActivityStatusCompleted {
		t.Errorf("status=%q want %q (pending→completed)", act.Status, biz.ActivityStatusCompleted)
	}
	if act.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", act.SessionID, "sess-1")
	}
	if act.TurnID != "turn-1" {
		t.Errorf("turnID=%q want %q", act.TurnID, "turn-1")
	}
	if nt, _ := act.Meta["noticeType"].(string); nt != "info" {
		t.Errorf("meta.noticeType=%q want %q", nt, "info")
	}

	// Expect 2 envelopes: activity_start (pending) + activity_done (completed)
	envs := bus.waitForPublished(t, 2)
	if envs[0].Type != contract.EnvelopeTypeActivityStart {
		t.Errorf("first envelope type=%q want activity_start", envs[0].Type)
	}
	if envs[1].Type != contract.EnvelopeTypeActivityDone {
		t.Errorf("second envelope type=%q want activity_done", envs[1].Type)
	}
}

// --- OnConfirmRequest ---

func TestOnConfirmRequest_createsActivityWithKindConfirm(t *testing.T) {
	p, bus, _ := newTestProjector()

	params := ConfirmRequestParams{
		ToolName:      "delete_file",
		ToolArguments: `{"path":"/tmp/x"}`,
		Content:       "Are you sure?",
	}
	act, err := p.OnConfirmRequest(context.Background(), "turn-1", "sess-1", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Kind != biz.ActivityKindConfirm {
		t.Errorf("kind=%q want %q", act.Kind, biz.ActivityKindConfirm)
	}
	if act.Status != biz.ActivityStatusToolBlocked {
		t.Errorf("status=%q want %q", act.Status, biz.ActivityStatusToolBlocked)
	}
	if act.Content != "Are you sure?" {
		t.Errorf("content=%q want %q", act.Content, "Are you sure?")
	}
	if tn, _ := act.Meta["toolName"].(string); tn != "delete_file" {
		t.Errorf("meta.toolName=%q want %q", tn, "delete_file")
	}
	if ta, _ := act.Meta["toolArguments"].(string); ta != `{"path":"/tmp/x"}` {
		t.Errorf("meta.toolArguments=%q want %q", ta, `{"path":"/tmp/x"}`)
	}

	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityStart {
		t.Errorf("envelope type=%q want activity_start", envs[0].Type)
	}
}

// --- OnConfirmResult ---

func TestOnConfirmResult_notFoundWhenActivityMissing(t *testing.T) {
	p, _, _ := newTestProjector()

	_, err := p.OnConfirmResult(context.Background(), "nonexistent-id", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeNotFound {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeNotFound)
	}
}

func TestOnConfirmResult_badRequestWhenKindNotConfirm(t *testing.T) {
	p, _, _ := newTestProjector()

	// Create a notice activity (not confirm) in the projector's internal map
	p.mu.Lock()
	p.activities["act-notice"] = &biz.Activity{
		ID:     "act-notice",
		Kind:   biz.ActivityKindNotice,
		Status: biz.ActivityStatusCompleted,
	}
	p.mu.Unlock()

	_, err := p.OnConfirmResult(context.Background(), "act-notice", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
}

func TestOnConfirmResult_completedWhenApproved(t *testing.T) {
	p, bus, _ := newTestProjector()

	// Create a confirm activity first
	params := ConfirmRequestParams{ToolName: "rm", Content: "OK?"}
	act, _ := p.OnConfirmRequest(context.Background(), "turn-1", "sess-1", params)

	// Wait for the confirm request envelope, then reset
	bus.waitForPublished(t, 1)
	bus.reset()

	result, err := p.OnConfirmResult(context.Background(), act.ID, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != biz.ActivityStatusCompleted {
		t.Errorf("status=%q want %q", result.Status, biz.ActivityStatusCompleted)
	}

	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityDone {
		t.Errorf("envelope type=%q want activity_done", envs[0].Type)
	}
}

func TestOnConfirmResult_cancelledWhenNotApproved(t *testing.T) {
	p, bus, _ := newTestProjector()

	params := ConfirmRequestParams{ToolName: "rm", Content: "OK?"}
	act, _ := p.OnConfirmRequest(context.Background(), "turn-1", "sess-1", params)

	// Wait for the confirm request envelope, then reset
	bus.waitForPublished(t, 1)
	bus.reset()

	result, err := p.OnConfirmResult(context.Background(), act.ID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != biz.ActivityStatusCancelled {
		t.Errorf("status=%q want %q", result.Status, biz.ActivityStatusCancelled)
	}

	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityDone {
		t.Errorf("envelope type=%q want activity_done", envs[0].Type)
	}
}

// --- OnPlanStart ---

func TestOnPlanStart_createsActivityWithKindPlan(t *testing.T) {
	p, bus, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
		{ID: "s2", Label: "Step 2", Status: biz.ActivityStatusPending},
	}
	act, err := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "My Plan", steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Kind != biz.ActivityKindPlan {
		t.Errorf("kind=%q want %q", act.Kind, biz.ActivityKindPlan)
	}
	if act.Status != biz.ActivityStatusPending {
		t.Errorf("status=%q want %q", act.Status, biz.ActivityStatusPending)
	}
	if act.Content != "My Plan" {
		t.Errorf("content=%q want %q", act.Content, "My Plan")
	}
	if act.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", act.SessionID, "sess-1")
	}
	if act.TurnID != "turn-1" {
		t.Errorf("turnID=%q want %q", act.TurnID, "turn-1")
	}

	metaSteps, ok := act.Meta["steps"].([]biz.ActivityPlanStep)
	if !ok {
		t.Fatal("meta.steps is not []biz.ActivityPlanStep")
	}
	if len(metaSteps) != 2 {
		t.Fatalf("meta.steps len=%d want 2", len(metaSteps))
	}
	if metaSteps[0].ID != "s1" {
		t.Errorf("meta.steps[0].id=%q want %q", metaSteps[0].ID, "s1")
	}
	if metaSteps[1].ID != "s2" {
		t.Errorf("meta.steps[1].id=%q want %q", metaSteps[1].ID, "s2")
	}

	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityStart {
		t.Errorf("envelope type=%q want activity_start", envs[0].Type)
	}
}

// --- OnPlanStepUpdate ---

func TestOnPlanStepUpdate_notFoundWhenActivityMissing(t *testing.T) {
	p, _, _ := newTestProjector()

	_, err := p.OnPlanStepUpdate(context.Background(), "nonexistent-id", "s1", biz.ActivityStatusCompleted)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeNotFound {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeNotFound)
	}
}

func TestOnPlanStepUpdate_badRequestWhenKindNotPlan(t *testing.T) {
	p, _, _ := newTestProjector()

	// Insert a non-plan activity
	p.mu.Lock()
	p.activities["act-notice"] = &biz.Activity{
		ID:     "act-notice",
		Kind:   biz.ActivityKindNotice,
		Status: biz.ActivityStatusCompleted,
	}
	p.mu.Unlock()

	_, err := p.OnPlanStepUpdate(context.Background(), "act-notice", "s1", biz.ActivityStatusCompleted)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
}

func TestOnPlanStepUpdate_badRequestWhenStepsMetaInvalid(t *testing.T) {
	p, _, _ := newTestProjector()

	// Insert a plan activity with invalid steps metadata
	p.mu.Lock()
	p.activities["act-plan"] = &biz.Activity{
		ID:     "act-plan",
		Kind:   biz.ActivityKindPlan,
		Status: biz.ActivityStatusPending,
		Meta:   map[string]any{"steps": "not-a-slice"},
	}
	p.mu.Unlock()

	_, err := p.OnPlanStepUpdate(context.Background(), "act-plan", "s1", biz.ActivityStatusCompleted)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
}

func TestOnPlanStepUpdate_notFoundWhenStepIDMissing(t *testing.T) {
	p, _, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
	}
	act, _ := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "Plan", steps)

	_, err := p.OnPlanStepUpdate(context.Background(), act.ID, "s999", biz.ActivityStatusCompleted)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("error type=%T want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeNotFound {
		t.Errorf("code=%q want %q", apiErr.Code, apierror.CodeNotFound)
	}
}

func TestOnPlanStepUpdate_updatesStepStatusCorrectly(t *testing.T) {
	p, _, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
		{ID: "s2", Label: "Step 2", Status: biz.ActivityStatusPending},
	}
	act, _ := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "Plan", steps)

	result, err := p.OnPlanStepUpdate(context.Background(), act.ID, "s1", biz.ActivityStatusRunning)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metaSteps, ok := result.Meta["steps"].([]biz.ActivityPlanStep)
	if !ok {
		t.Fatal("meta.steps is not []biz.ActivityPlanStep")
	}
	if metaSteps[0].Status != biz.ActivityStatusRunning {
		t.Errorf("step s1 status=%q want %q", metaSteps[0].Status, biz.ActivityStatusRunning)
	}
	if metaSteps[1].Status != biz.ActivityStatusPending {
		t.Errorf("step s2 status=%q want %q (unchanged)", metaSteps[1].Status, biz.ActivityStatusPending)
	}
}

func TestOnPlanStepUpdate_planRunningWhenSomeStepsPending(t *testing.T) {
	p, _, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
		{ID: "s2", Label: "Step 2", Status: biz.ActivityStatusPending},
	}
	act, _ := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "Plan", steps)

	// Complete s1, s2 still pending → plan should be running
	result, err := p.OnPlanStepUpdate(context.Background(), act.ID, "s1", biz.ActivityStatusCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != biz.ActivityStatusRunning {
		t.Errorf("plan status=%q want %q", result.Status, biz.ActivityStatusRunning)
	}
}

func TestOnPlanStepUpdate_planCompletedWhenAllStepsCompleted(t *testing.T) {
	p, _, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
		{ID: "s2", Label: "Step 2", Status: biz.ActivityStatusPending},
	}
	act, _ := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "Plan", steps)

	// Complete both steps
	p.OnPlanStepUpdate(context.Background(), act.ID, "s1", biz.ActivityStatusCompleted)
	result, err := p.OnPlanStepUpdate(context.Background(), act.ID, "s2", biz.ActivityStatusCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != biz.ActivityStatusCompleted {
		t.Errorf("plan status=%q want %q", result.Status, biz.ActivityStatusCompleted)
	}
}

func TestOnPlanStepUpdate_planPartialFailureWhenAllDoneButSomeFailed(t *testing.T) {
	p, _, _ := newTestProjector()

	steps := []biz.ActivityPlanStep{
		{ID: "s1", Label: "Step 1", Status: biz.ActivityStatusPending},
		{ID: "s2", Label: "Step 2", Status: biz.ActivityStatusPending},
	}
	act, _ := p.OnPlanStart(context.Background(), "turn-1", "sess-1", "Plan", steps)

	// s1 completed, s2 failed → partial_failure
	p.OnPlanStepUpdate(context.Background(), act.ID, "s1", biz.ActivityStatusCompleted)
	result, err := p.OnPlanStepUpdate(context.Background(), act.ID, "s2", biz.ActivityStatusFailed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != biz.ActivityStatusPartialFailure {
		t.Errorf("plan status=%q want %q", result.Status, biz.ActivityStatusPartialFailure)
	}
}
