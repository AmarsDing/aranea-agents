package deliverable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// AckKeyPrefix is the top-level key prefix for delivery acknowledgments in
// the deliverable map ("ack/<topic>"). Top-level keys merge independently
// under MergeReducer, so parallel acks on distinct topics never clobber
// each other. Ack keys are intra-team signals: the bridge
// (WriteDeliverablesToSession) excludes them from the DeliverableRef
// StructuredJSON so they never leak into inter-team envelopes.
const AckKeyPrefix = "ack/"

// Ack statuses accepted by ack_deliverable.
const (
	AckStatusAccepted = "accepted"
	AckStatusRejected = "rejected"
)

// AckKeyForTopic renders the top-level deliverable key carrying the ack for
// a topic. Exported for the bridge exclusion filter and readers.
func AckKeyForTopic(topic string) string { return AckKeyPrefix + topic }

// ackDeliverableInput is the ack_deliverable tool input.
type ackDeliverableInput struct {
	Topic   string `json:"topic" jsonschema:"description=The topic being acknowledged (the topic used in set_deliverable).,required"`
	Status  string `json:"status" jsonschema:"description=accepted or rejected.,required,enum=accepted,enum=rejected"`
	Comment string `json:"comment" jsonschema:"description=Optional comment. Strongly recommended when status=rejected so the producer/coordinator knows what to fix."`
}

// ackDeliverableOutput is the ack_deliverable tool output. Record carries the
// full ack payload so StateDelta (which has no ctx) can rebuild the state
// delta deterministically from the serialized result.
type ackDeliverableOutput struct {
	Acked  bool           `json:"acked"`
	Topic  string         `json:"topic"`
	Status string         `json:"status"`
	Record map[string]any `json:"record,omitempty"`
}

// AckDeliverableTool lets a team member formally accept or reject a topic
// deliverable produced by another member. The ack is an advisory signal
// (never gates the run): coordinators/synthesizers read acks via
// get_deliverable(key="ack/<topic>") to decide whether rework is needed.
type AckDeliverableTool struct{}

// NewAckDeliverableTool creates the ack_deliverable tool.
func NewAckDeliverableTool() *AckDeliverableTool { return &AckDeliverableTool{} }

// Declaration returns the tool metadata.
func (t *AckDeliverableTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "ack_deliverable",
		Description: "Acknowledge a topic deliverable after reviewing it via get_deliverable. " +
			"Use status=accepted when the deliverable satisfies your needs, or status=rejected " +
			"with a comment explaining what is missing/wrong so the producer or coordinator can fix it. " +
			"Acks are advisory: they inform the team but do not block execution.",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"topic", "status"},
			Properties: map[string]*trpctool.Schema{
				"topic": {
					Type:        "string",
					Description: "The topic being acknowledged (same slug as in set_deliverable).",
				},
				"status": {
					Type:        "string",
					Description: "accepted or rejected.",
					Enum:        []any{AckStatusAccepted, AckStatusRejected},
				},
				"comment": {
					Type:        "string",
					Description: "Optional comment; strongly recommended when rejecting.",
				},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Confirmation that the ack was recorded.",
			Required:    []string{"acked", "topic", "status"},
			Properties: map[string]*trpctool.Schema{
				"acked":  {Type: "boolean", Description: "Whether the ack was recorded."},
				"topic":  {Type: "string", Description: "The acknowledged topic."},
				"status": {Type: "string", Description: "The recorded status."},
				"record": {Type: "object", Description: "The full ack record (status/by/comment/at)."},
			},
		},
	}
}

// Call validates the input, writes the ack record into the node-local
// read-your-writes view, and returns it. The authoritative graph state write
// flows via StateDelta: a single top-level "ack/<topic>" key so concurrent
// acks on distinct topics merge safely under MergeReducer.
func (t *AckDeliverableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil {
		return nil, errors.New("ack_deliverable is not configured")
	}
	var in ackDeliverableInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, err
	}
	topic := strings.TrimSpace(in.Topic)
	if topic == "" {
		return nil, errors.New("topic is required")
	}
	if err := validateTopicName(topic); err != nil {
		return nil, err
	}
	status := strings.TrimSpace(in.Status)
	if status != AckStatusAccepted && status != AckStatusRejected {
		return nil, fmt.Errorf("invalid status %q: must be %q or %q", in.Status, AckStatusAccepted, AckStatusRejected)
	}

	record := map[string]any{
		"status":  status,
		"by":      ackAuthorFromCtx(ctx),
		"comment": strings.TrimSpace(in.Comment),
		"at":      time.Now().UTC().Format(time.RFC3339),
	}
	deltaKey := AckKeyForTopic(topic)
	base, _ := readDeliverableMap(ctx)
	merged := make(map[string]any, len(base)+1)
	for k, v := range base {
		merged[k] = v
	}
	merged[deltaKey] = record
	storeLocalDeliverable(ctx, merged)
	return ackDeliverableOutput{Acked: true, Topic: topic, Status: status, Record: record}, nil
}

// StateDelta returns the graph state delta after a successful Call: only the
// top-level ack key — MergeReducer unions it into the deliverable map without
// touching sibling topics or other acks.
func (t *AckDeliverableTool) StateDelta(toolCallID string, _ []byte, resultJSON []byte) map[string][]byte {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" || len(resultJSON) == 0 {
		return nil
	}
	var out ackDeliverableOutput
	if err := json.Unmarshal(resultJSON, &out); err != nil {
		return nil
	}
	if !out.Acked || len(out.Record) == 0 {
		return nil
	}
	b, err := json.Marshal(map[string]any{AckKeyForTopic(out.Topic): out.Record})
	if err != nil {
		return nil
	}
	return map[string][]byte{biz.DeliverableStateKey: b}
}

// ackAuthorFromCtx attributes the ack to the calling agent (invocation agent
// name), falling back to "unknown" outside agent runs.
func ackAuthorFromCtx(ctx context.Context) string {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return "unknown"
	}
	if name := strings.TrimSpace(inv.AgentName); name != "" {
		return name
	}
	return "unknown"
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*AckDeliverableTool)(nil)
	_ trpctool.CallableTool = (*AckDeliverableTool)(nil)
)
