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

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
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

// --- OnMemberMessageDelta / OnMemberMessageDone (AF-GAP-04) ---

// TestOnMemberMessageDelta_createsReplyActivityWithMemberID verifies that the
// first delta for a team member author creates a reply Activity tagged with
// meta.member_id, so the frontend can distinguish member replies from the
// coordinator's reply.
//
// Note: On first delta, OnMemberMessageDelta publishes two envelopes:
//   - activity_start (async via safego.Go in publishAndPersist)
//   - activity_delta (sync via publishActivityDelta)
//
// Because the start envelope is async and the delta is sync, arrival order is
// not guaranteed. We wait for both and locate the activity_start envelope.
func TestOnMemberMessageDelta_createsReplyActivityWithMemberID(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		AgentID:         "coordinator",
		AgentDisplayName: "Coordinator",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	p.OnMemberMessageDelta(context.Background(), "worker-a", "Hello ")

	// Expect 2 envelopes: activity_start + activity_delta (order not guaranteed)
	envs := bus.waitForPublished(t, 2)
	var startEnv *event.Envelope
	for i := range envs {
		if envs[i].Type == contract.EnvelopeTypeActivityStart {
			startEnv = &envs[i]
			break
		}
	}
	if startEnv == nil {
		t.Fatalf("no activity_start envelope found among %d envelopes", len(envs))
	}
	kind, _ := startEnv.Metadata["kind"].(string)
	if kind != string(biz.ActivityKindReply) {
		t.Errorf("metadata kind=%q want %q", kind, biz.ActivityKindReply)
	}
	meta, _ := startEnv.Metadata["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("metadata meta is nil, expected member_id tag")
	}
	memberID, _ := meta["member_id"].(string)
	if memberID != "worker-a" {
		t.Errorf("meta.member_id=%q want %q", memberID, "worker-a")
	}
	if startEnv.TeamID != "team-1" {
		t.Errorf("envelope TeamID=%q want %q (should inherit from ProjectMeta)", startEnv.TeamID, "team-1")
	}
}

// TestOnMemberMessageDelta_appendsDeltaToExistingActivity verifies that
// subsequent deltas reuse the same reply Activity instead of creating new ones.
func TestOnMemberMessageDelta_appendsDeltaToExistingActivity(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	p.OnMemberMessageDelta(context.Background(), "worker-a", "Hello ")
	// First Delta publishes 2 envelopes: activity_start (async via safego.Go)
	// and activity_delta (sync). Wait for both to drain before resetting,
	// otherwise the lingering activity_start may race with the next call's
	// async publish and be mis-attributed.
	bus.waitForPublished(t, 2)
	bus.reset()

	p.OnMemberMessageDelta(context.Background(), "worker-a", "world")

	// Expect 1 envelope: activity_delta (no new activity_start)
	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityDelta {
		t.Errorf("envelope type=%q want activity_delta", envs[0].Type)
	}
}

// TestOnMemberMessageDone_finalizesReplyActivity verifies that Done marks the
// reply Activity as completed and publishes activity_done.
func TestOnMemberMessageDone_finalizesReplyActivity(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	p.OnMemberMessageDelta(context.Background(), "worker-a", "Hello ")
	// First Delta publishes 2 envelopes: activity_start (async via safego.Go)
	// and activity_delta (sync). Wait for both to drain before resetting,
	// otherwise the lingering activity_start may race with Done's async
	// publish and be mis-attributed as the Done envelope.
	bus.waitForPublished(t, 2)
	bus.reset()

	p.OnMemberMessageDone(context.Background(), "worker-a", "Hello world")

	envs := bus.waitForPublished(t, 1)
	if envs[0].Type != contract.EnvelopeTypeActivityDone {
		t.Errorf("envelope type=%q want activity_done", envs[0].Type)
	}
	status, _ := envs[0].Metadata["status"].(string)
	if status != string(biz.ActivityStatusCompleted) {
		t.Errorf("status=%q want %q", status, biz.ActivityStatusCompleted)
	}
	content, _ := envs[0].Metadata["content"].(string)
	if content != "Hello world" {
		t.Errorf("content=%q want %q", content, "Hello world")
	}
}

// TestOnMemberMessageDone_noopWhenNoActivity verifies that Done without a
// prior Delta does not publish anything (defensive guard).
func TestOnMemberMessageDone_noopWhenNoActivity(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	p.OnMemberMessageDone(context.Background(), "worker-a", "orphan text")

	// No envelopes should be published
	if envs := bus.waitForPublished(t, 0); len(envs) != 0 {
		t.Errorf("expected 0 envelopes, got %d", len(envs))
	}
}

// TestProcessEvent_routesTeamMemberToMemberMessage verifies that ProcessEvent
// detects team member authors and routes text to OnMemberMessage* instead of
// OnTextDelta/OnTextDone, so the resulting Activity carries meta.member_id.
//
// Note: First delta publishes activity_start (async) + activity_delta (sync).
// Arrival order is not guaranteed, so we wait for both and locate the start.
func TestProcessEvent_routesTeamMemberToMemberMessage(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		AgentID:         "coordinator",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	// Simulate a streaming chunk from a team member
	ev := &trpcevent.Event{
		Author: "worker-a",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{Content: "member reply"},
			}},
		},
	}

	p.ProcessEvent(context.Background(), ev)

	// Expect 2 envelopes: activity_start + activity_delta (order not guaranteed)
	envs := bus.waitForPublished(t, 2)
	var startEnv *event.Envelope
	for i := range envs {
		if envs[i].Type == contract.EnvelopeTypeActivityStart {
			startEnv = &envs[i]
			break
		}
	}
	if startEnv == nil {
		t.Fatalf("no activity_start envelope found among %d envelopes", len(envs))
	}
	kind, _ := startEnv.Metadata["kind"].(string)
	if kind != string(biz.ActivityKindReply) {
		t.Errorf("metadata kind=%q want %q", kind, biz.ActivityKindReply)
	}
	meta, _ := startEnv.Metadata["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("metadata meta is nil, expected member_id tag")
	}
	memberID, _ := meta["member_id"].(string)
	if memberID != "worker-a" {
		t.Errorf("meta.member_id=%q want %q", memberID, "worker-a")
	}
}

// TestProcessEvent_routesCoordinatorToOnTextDelta verifies that non-team-member
// authors still go through the regular OnTextDelta path (no meta.member_id).
//
// Note: First delta publishes activity_start (async) + activity_delta (sync).
// Arrival order is not guaranteed, so we wait for both and locate the start.
func TestProcessEvent_routesCoordinatorToOnTextDelta(t *testing.T) {
	p, bus, _ := newTestProjector()
	p.Configure(ProjectMeta{
		SessionID:       "sess-team",
		RequestID:       "turn-1",
		TeamID:          "team-1",
		AgentID:         "coordinator",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}},
	}, nil)

	// Simulate a streaming chunk from the coordinator (not a team member)
	ev := &trpcevent.Event{
		Author: "coordinator",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{Content: "coordinator reply"},
			}},
		},
	}

	p.ProcessEvent(context.Background(), ev)

	// Expect 2 envelopes: activity_start + activity_delta (order not guaranteed)
	envs := bus.waitForPublished(t, 2)
	var startEnv *event.Envelope
	for i := range envs {
		if envs[i].Type == contract.EnvelopeTypeActivityStart {
			startEnv = &envs[i]
			break
		}
	}
	if startEnv == nil {
		t.Fatalf("no activity_start envelope found among %d envelopes", len(envs))
	}
	kind, _ := startEnv.Metadata["kind"].(string)
	if kind != string(biz.ActivityKindReply) {
		t.Errorf("metadata kind=%q want %q", kind, biz.ActivityKindReply)
	}
	// Coordinator reply should NOT have meta.member_id
	meta, _ := startEnv.Metadata["meta"].(map[string]any)
	if meta != nil {
		if _, ok := meta["member_id"]; ok {
			t.Errorf("coordinator reply should not have meta.member_id, got %v", meta["member_id"])
		}
	}
}
