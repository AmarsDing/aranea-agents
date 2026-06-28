package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

type EventBridge struct {
	activityBus     biz.ActivityEventBus
	sessionID       string
	spiritSessionID string
	graphID         string
	execID          string
	summary         *ExecutionSummaryTracker
	lg              loggateway.Logger
}

func NewEventBridge(activityBus biz.ActivityEventBus, sessionID, spiritSessionID, graphID, execID string, lg loggateway.Logger) *EventBridge {
	return &EventBridge{
		activityBus:     activityBus,
		sessionID:       sessionID,
		spiritSessionID: spiritSessionID,
		graphID:         graphID,
		execID:          execID,
		summary:         NewExecutionSummaryTracker(execID, graphID),
		lg:              lg,
	}
}

func (b *EventBridge) ActivityBus() biz.ActivityEventBus {
	return b.activityBus
}

func (b *EventBridge) Consume(ctx context.Context, eventCh <-chan *trpcevent.Event) {
	for e := range eventCh {
		ev := b.ConvertEvent(e)
		if ev != nil && b.activityBus != nil {
			b.activityBus.Publish(ctx, *ev)
		} else if ev == nil {
			b.lg.Warn("unhandled event dropped",
				loggateway.StepID("graph.event_bridge"),
				loggateway.Any("object", fmt.Sprintf("%v", e.Object)),
			)
		}
	}
}

// ConvertEvent converts a trpc-agent-go framework Event into a biz.ActivityEvent
// suitable for publishing on biz.ActivityEventBus. Returns nil when the framework
// event is not mapped (unknown object type or non-terminal GraphExecution event).
func (b *EventBridge) ConvertEvent(e *trpcevent.Event) *biz.ActivityEvent {
	return b.convertEvent(e)
}

func (b *EventBridge) convertEvent(e *trpcevent.Event) *biz.ActivityEvent {
	stage, kind, status, eventType, deltaField := b.mapType(e)
	if stage == "" && kind == "" {
		return nil
	}

	now := time.Now().UTC()
	if !e.Timestamp.IsZero() {
		now = e.Timestamp.UTC()
	}

	ev := biz.ActivityEvent{
		Event: eventType,
		Activity: biz.Activity{
			ID:              uuid.NewString(),
			Kind:            kind,
			Status:          status,
			SessionID:       b.sessionID,
			SpiritSessionID: b.spiritSessionID,
			Timestamp:       now,
			Stage:           stage,
			Meta: map[string]any{
				"execution_id": b.execID,
				"graph_id":     b.graphID,
				"filter_key":   fmt.Sprintf("graph/%s/%s", b.graphID, b.execID),
				"channel":      "graph",
				"author":       e.Author,
			},
		},
		DeltaField: deltaField,
		Domain:     biz.ActivityDomainChat,
	}
	if e.ID != "" {
		ev.Activity.ID = e.ID
	}
	if e.RequestID != "" {
		ev.Activity.TurnID = e.RequestID
	}
	if e.InvocationID != "" {
		ev.Activity.Meta["invocation_id"] = e.InvocationID
	}
	if e.Branch != "" {
		ev.Activity.Meta["branch"] = e.Branch
	}
	if e.Tag != "" {
		ev.Activity.Meta["tag"] = e.Tag
	}

	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		meta := b.extractNodeMeta(e)
		ev.Activity.Meta["node_id"] = meta.NodeID
		ev.Activity.Meta["node_type"] = string(meta.NodeType)
		ev.Activity.Meta["step_number"] = meta.StepNumber
		ev.Activity.Meta["phase"] = string(meta.Phase)
		if !meta.StartTime.IsZero() {
			ev.Activity.Meta["start_time"] = meta.StartTime.Format("2006-01-02T15:04:05.000Z07:00")
		}

	case trpcgraph.ObjectTypeGraphNodeComplete:
		meta := b.extractNodeMeta(e)
		if b.summary != nil {
			b.summary.RecordNodeComplete(meta)
		}
		ev.Activity.Meta["node_id"] = meta.NodeID
		ev.Activity.Meta["node_type"] = string(meta.NodeType)
		ev.Activity.Meta["step_number"] = meta.StepNumber
		ev.Activity.Meta["phase"] = string(meta.Phase)
		if meta.Duration > 0 {
			ev.Activity.Meta["duration_ns"] = meta.Duration.Nanoseconds()
		}
		if !meta.EndTime.IsZero() {
			ev.Activity.Meta["end_time"] = meta.EndTime.Format("2006-01-02T15:04:05.000Z07:00")
		}
		if len(meta.OutputKeys) > 0 {
			ev.Activity.Meta["output_keys"] = meta.OutputKeys
			for _, key := range meta.OutputKeys {
				if key == biz.SkippedNodeOutputKey {
					ev.Activity.Meta["skipped"] = true
					break
				}
			}
		}

	case trpcgraph.ObjectTypeGraphNodeError:
		meta := b.extractNodeMeta(e)
		if b.summary != nil {
			b.summary.RecordNodeComplete(meta)
		}
		ev.Activity.Meta["node_id"] = meta.NodeID
		ev.Activity.Meta["node_type"] = string(meta.NodeType)
		ev.Activity.Meta["error"] = meta.Error
		ev.Activity.Meta["retrying"] = meta.Retrying
		ev.Activity.Meta["attempt"] = meta.Attempt
		ev.Activity.Meta["max_attempts"] = meta.MaxAttempts
		ev.Activity.Meta["error_type"] = "graph_node_error"
		if meta.Error != "" {
			ev.Activity.Content = meta.Error
		}

	case trpcgraph.ObjectTypeGraphNodeCustom:
		meta := b.extractNodeCustomMeta(e)
		ev.Activity.Meta["node_id"] = meta.NodeID
		ev.Activity.Meta["event_type"] = meta.EventType
		ev.Activity.Meta["category"] = string(meta.Category)
		ev.Activity.Meta["step_number"] = meta.StepNumber
		ev.Activity.Meta["message"] = meta.Message
		ev.Activity.Meta["progress"] = meta.Progress
		if meta.Payload != nil {
			ev.Activity.Meta["payload"] = meta.Payload
		}

	case trpcgraph.ObjectTypeGraphPregelStep:
		meta := b.extractPregelMeta(e)
		ev.Activity.Meta["step_number"] = meta.StepNumber
		ev.Activity.Meta["phase"] = string(meta.Phase)
		ev.Activity.Meta["task_count"] = meta.TaskCount
		ev.Activity.Meta["active_nodes"] = meta.ActiveNodes
		if meta.Duration > 0 {
			ev.Activity.Meta["duration_ns"] = meta.Duration.Nanoseconds()
		}

	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		meta := b.extractPregelMeta(e)
		ev.Activity.Meta["checkpoint_id"] = meta.CheckpointID
		ev.Activity.Meta["lineage_id"] = meta.LineageID
		ev.Activity.Meta["interrupt_key"] = meta.InterruptKey
		ev.Activity.Meta["node_id"] = meta.NodeID
		ev.Activity.Meta["step_number"] = meta.StepNumber
		if meta.InterruptValue != nil {
			ev.Activity.Meta["interrupt_value"] = meta.InterruptValue
		}

	case trpcgraph.ObjectTypeGraphCheckpointCreated:
		meta := b.extractPregelMeta(e)
		ev.Activity.Meta["checkpoint_id"] = meta.CheckpointID
		ev.Activity.Meta["lineage_id"] = meta.LineageID
		ev.Activity.Meta["step_number"] = meta.StepNumber

	case trpcgraph.ObjectTypeGraphStateUpdate:
		meta := b.extractStateUpdateMeta(e)
		ev.Activity.Meta["updated_keys"] = meta.UpdatedKeys
		ev.Activity.Meta["removed_keys"] = meta.RemovedKeys

	case trpcgraph.ObjectTypeGraphExecution:
		if e.Done {
			meta := b.extractCompletionMeta(e)
			ev.Activity.Meta["total_steps"] = meta.TotalSteps
			if meta.TotalDuration > 0 {
				ev.Activity.Meta["duration_ns"] = meta.TotalDuration.Nanoseconds()
			}
			if b.summary != nil {
				summary := b.summary.Snapshot(meta)
				ev.Activity.Meta["execution_summary"] = summary
			}
		} else {
			return nil
		}

	default:
		return nil
	}

	return &ev
}

// mapType maps a trpc-agent-go framework event to the ActivityEvent payload
// fields: Stage (graph-specific sub-stage), Kind (ActivityKind), Status
// (ActivityStatus), Event (ActivityEventType), and DeltaField (for streaming).
//
// Mapping table (per migration spec):
//   - graph_node_start      → Kind=graph_stage, Stage="node_start",   Status=running,    Event=created
//   - graph_node_end        → Kind=graph_stage, Stage="node_end",     Status=completed,  Event=completed
//   - graph_node_error      → Kind=graph_stage, Stage="node_error",   Status=failed,     Event=failed
//   - graph_node_custom     → Kind=graph_stage, Stage="node_custom",  Status=running,    Event=updated
//   - graph_step            → Kind=graph_stage, Stage="step",         Status=running,    Event=updated
//   - checkpoint            → Kind=session,     Stage="checkpoint",   Status=running,    Event=created
//   - state_delta           → Kind=notice,      Stage="",             Status=running,    Event=streaming, DeltaField="state"
//   - graph_execution_done  → Kind=graph_stage, Stage="execution_done",Status=completed, Event=completed
func (b *EventBridge) mapType(e *trpcevent.Event) (stage string, kind biz.ActivityKind, status biz.ActivityStatus, eventType biz.ActivityEventType, deltaField string) {
	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		return "node_start", biz.ActivityKindGraphStage, biz.ActivityStatusRunning, biz.ActivityEventCreated, ""
	case trpcgraph.ObjectTypeGraphNodeComplete:
		return "node_end", biz.ActivityKindGraphStage, biz.ActivityStatusCompleted, biz.ActivityEventCompleted, ""
	case trpcgraph.ObjectTypeGraphNodeError:
		return "node_error", biz.ActivityKindGraphStage, biz.ActivityStatusFailed, biz.ActivityEventFailed, ""
	case trpcgraph.ObjectTypeGraphNodeCustom:
		return "node_custom", biz.ActivityKindGraphStage, biz.ActivityStatusRunning, biz.ActivityEventUpdated, ""
	case trpcgraph.ObjectTypeGraphPregelStep:
		return "step", biz.ActivityKindGraphStage, biz.ActivityStatusRunning, biz.ActivityEventUpdated, ""
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		return "checkpoint", biz.ActivityKindSession, biz.ActivityStatusRunning, biz.ActivityEventCreated, ""
	case trpcgraph.ObjectTypeGraphCheckpointCreated:
		return "checkpoint", biz.ActivityKindSession, biz.ActivityStatusRunning, biz.ActivityEventCreated, ""
	case trpcgraph.ObjectTypeGraphStateUpdate:
		return "", biz.ActivityKindNotice, biz.ActivityStatusRunning, biz.ActivityEventStreaming, "state"
	case trpcgraph.ObjectTypeGraphExecution:
		if e.Done {
			return "execution_done", biz.ActivityKindGraphStage, biz.ActivityStatusCompleted, biz.ActivityEventCompleted, ""
		}
		return "", "", "", "", ""
	default:
		return "", "", "", "", ""
	}
}

func (b *EventBridge) extractNodeMeta(e *trpcevent.Event) trpcgraph.NodeExecutionMetadata {
	var meta trpcgraph.NodeExecutionMetadata
	if e.StateDelta == nil {
		return meta
	}
	raw, ok := e.StateDelta[trpcgraph.MetadataKeyNode]
	if !ok || len(raw) == 0 {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		b.lg.Warn("node meta unmarshal failed",
			loggateway.StepID("graph.event_bridge.node_meta_fail"),
			loggateway.Err(err),
		)
	}
	return meta
}

func (b *EventBridge) extractPregelMeta(e *trpcevent.Event) trpcgraph.PregelStepMetadata {
	var meta trpcgraph.PregelStepMetadata
	if e.StateDelta == nil {
		return meta
	}
	raw, ok := e.StateDelta[trpcgraph.MetadataKeyPregel]
	if !ok || len(raw) == 0 {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		b.lg.Warn("pregel meta unmarshal failed",
			loggateway.StepID("graph.event_bridge.pregel_meta_fail"),
			loggateway.Err(err),
		)
	}
	return meta
}

func (b *EventBridge) extractStateUpdateMeta(e *trpcevent.Event) trpcgraph.StateUpdateMetadata {
	var meta trpcgraph.StateUpdateMetadata
	if e.StateDelta == nil {
		return meta
	}
	raw, ok := e.StateDelta[trpcgraph.MetadataKeyState]
	if !ok || len(raw) == 0 {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		b.lg.Warn("state update meta unmarshal failed",
			loggateway.StepID("graph.event_bridge.state_meta_fail"),
			loggateway.Err(err),
		)
	}
	return meta
}

func (b *EventBridge) extractCompletionMeta(e *trpcevent.Event) trpcgraph.CompletionMetadata {
	var meta trpcgraph.CompletionMetadata
	if e.StateDelta == nil {
		return meta
	}
	raw, ok := e.StateDelta[trpcgraph.MetadataKeyCompletion]
	if !ok || len(raw) == 0 {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		b.lg.Warn("completion meta unmarshal failed",
			loggateway.StepID("graph.event_bridge.completion_meta_fail"),
			loggateway.Err(err),
		)
	}
	return meta
}

func (b *EventBridge) extractNodeCustomMeta(e *trpcevent.Event) trpcgraph.NodeCustomEventMetadata {
	var meta trpcgraph.NodeCustomEventMetadata
	if e.StateDelta == nil {
		return meta
	}
	raw, ok := e.StateDelta[trpcgraph.MetadataKeyNodeCustom]
	if !ok || len(raw) == 0 {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		b.lg.Warn("node custom meta unmarshal failed",
			loggateway.StepID("graph.event_bridge.node_custom_meta_fail"),
			loggateway.Err(err),
		)
	}
	return meta
}

func ExtractNodeMeta(e *trpcevent.Event, lg loggateway.Logger) trpcgraph.NodeExecutionMetadata {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	b := &EventBridge{lg: lg}
	return b.extractNodeMeta(e)
}

func ExtractPregelMeta(e *trpcevent.Event, lg loggateway.Logger) trpcgraph.PregelStepMetadata {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	b := &EventBridge{lg: lg}
	return b.extractPregelMeta(e)
}
