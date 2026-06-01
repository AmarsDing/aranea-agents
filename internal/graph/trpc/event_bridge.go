package graph

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

type EventBridge struct {
	eventBus  event.Bus
	sessionID string
	graphID   string
	execID    string
	summary   *ExecutionSummaryTracker
	lg        loggateway.Logger
}

func NewEventBridge(eventBus event.Bus, sessionID, graphID, execID string, lg loggateway.Logger) *EventBridge {
	return &EventBridge{
		eventBus:  eventBus,
		sessionID: sessionID,
		graphID:   graphID,
		execID:    execID,
		summary:   NewExecutionSummaryTracker(execID, graphID),
		lg:        lg,
	}
}

func (b *EventBridge) Consume(ctx context.Context, eventCh <-chan *trpcevent.Event) {
	for e := range eventCh {
		env := b.convertEvent(e)
		if env != nil {
			b.eventBus.Publish(ctx, *env)
		} else {
			b.lg.Warn("unhandled event dropped",
				loggateway.StepID("graph.event_bridge"),
				loggateway.Any("object", fmt.Sprintf("%v", e.Object)),
			)
		}
	}
}

func (b *EventBridge) ConvertEvent(e *trpcevent.Event) *event.Envelope {
	return b.convertEvent(e)
}

func (b *EventBridge) convertEvent(e *trpcevent.Event) *event.Envelope {
	envType := b.mapType(e)
	if envType == "" {
		return nil
	}

	env := event.NewEnvelope(envType, "graph", b.sessionID)
	env.Channel = "graph"
	env.FilterKey = fmt.Sprintf("graph/%s/%s", b.graphID, b.execID)

	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		meta := b.extractNodeMeta(e)
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"node_id":      meta.NodeID,
			"node_type":    string(meta.NodeType),
			"step_number":  meta.StepNumber,
			"phase":        string(meta.Phase),
		}
		if meta.StartTime.IsZero() == false {
			env.Metadata["start_time"] = meta.StartTime.Format("2006-01-02T15:04:05.000Z07:00")
		}

	case trpcgraph.ObjectTypeGraphNodeComplete:
		meta := b.extractNodeMeta(e)
		if b.summary != nil {
			b.summary.RecordNodeComplete(meta)
		}
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"node_id":      meta.NodeID,
			"node_type":    string(meta.NodeType),
			"step_number":  meta.StepNumber,
			"phase":        string(meta.Phase),
		}
		if meta.Duration > 0 {
			env.Metadata["duration_ns"] = meta.Duration.Nanoseconds()
		}
		if meta.EndTime.IsZero() == false {
			env.Metadata["end_time"] = meta.EndTime.Format("2006-01-02T15:04:05.000Z07:00")
		}
		if len(meta.OutputKeys) > 0 {
			env.Metadata["output_keys"] = meta.OutputKeys
			for _, key := range meta.OutputKeys {
				if key == biz.SkippedNodeOutputKey {
					env.Metadata["skipped"] = true
					break
				}
			}
		}

	case trpcgraph.ObjectTypeGraphNodeError:
		meta := b.extractNodeMeta(e)
		if b.summary != nil {
			b.summary.RecordNodeComplete(meta)
		}
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"node_id":      meta.NodeID,
			"node_type":    string(meta.NodeType),
			"error":        meta.Error,
			"retrying":     meta.Retrying,
			"attempt":      meta.Attempt,
			"max_attempts": meta.MaxAttempts,
		}
		env.Error = &event.EnvelopeError{
			Type:    "graph_node_error",
			Message: meta.Error,
		}

	case trpcgraph.ObjectTypeGraphNodeCustom:
		meta := b.extractNodeCustomMeta(e)
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"node_id":      meta.NodeID,
			"event_type":   meta.EventType,
			"category":     string(meta.Category),
			"step_number":  meta.StepNumber,
			"message":      meta.Message,
			"progress":     meta.Progress,
		}
		if meta.Payload != nil {
			env.Metadata["payload"] = meta.Payload
		}

	case trpcgraph.ObjectTypeGraphPregelStep:
		meta := b.extractPregelMeta(e)
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"step_number":  meta.StepNumber,
			"phase":        string(meta.Phase),
			"task_count":   meta.TaskCount,
			"active_nodes": meta.ActiveNodes,
		}
		if meta.Duration > 0 {
			env.Metadata["duration_ns"] = meta.Duration.Nanoseconds()
		}

	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		meta := b.extractPregelMeta(e)
		env.Metadata = map[string]any{
			"execution_id":  b.execID,
			"graph_id":      b.graphID,
			"checkpoint_id": meta.CheckpointID,
			"lineage_id":    meta.LineageID,
			"interrupt_key": meta.InterruptKey,
			"node_id":       meta.NodeID,
			"step_number":   meta.StepNumber,
		}
		if meta.InterruptValue != nil {
			env.Metadata["interrupt_value"] = meta.InterruptValue
		}

	case trpcgraph.ObjectTypeGraphCheckpointCreated:
		meta := b.extractPregelMeta(e)
		env.Metadata = map[string]any{
			"execution_id":  b.execID,
			"graph_id":      b.graphID,
			"checkpoint_id": meta.CheckpointID,
			"lineage_id":    meta.LineageID,
			"step_number":   meta.StepNumber,
		}

	case trpcgraph.ObjectTypeGraphStateUpdate:
		meta := b.extractStateUpdateMeta(e)
		env.Metadata = map[string]any{
			"execution_id": b.execID,
			"graph_id":     b.graphID,
			"updated_keys": meta.UpdatedKeys,
			"removed_keys": meta.RemovedKeys,
		}

	case trpcgraph.ObjectTypeGraphExecution:
		if e.Done {
			meta := b.extractCompletionMeta(e)
			env.Metadata = map[string]any{
				"execution_id": b.execID,
				"graph_id":     b.graphID,
				"total_steps":  meta.TotalSteps,
			}
			if meta.TotalDuration > 0 {
				env.Metadata["duration_ns"] = meta.TotalDuration.Nanoseconds()
			}
			if b.summary != nil {
				summary := b.summary.Snapshot(meta)
				env.Metadata["execution_summary"] = summary
			}
		} else {
			return nil
		}

	default:
		return nil
	}

	return &env
}

func (b *EventBridge) mapType(e *trpcevent.Event) event.EnvelopeType {
	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		return event.EnvelopeTypeGraphNodeStart
	case trpcgraph.ObjectTypeGraphNodeComplete:
		return event.EnvelopeTypeGraphNodeEnd
	case trpcgraph.ObjectTypeGraphNodeError:
		return event.EnvelopeTypeGraphNodeError
	case trpcgraph.ObjectTypeGraphNodeCustom:
		return event.EnvelopeTypeGraphNodeCustom
	case trpcgraph.ObjectTypeGraphPregelStep:
		return event.EnvelopeTypeGraphStep
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		return event.EnvelopeTypeCheckpoint
	case trpcgraph.ObjectTypeGraphCheckpointCreated:
		return event.EnvelopeTypeCheckpoint
	case trpcgraph.ObjectTypeGraphStateUpdate:
		return event.EnvelopeTypeStateDelta
	case trpcgraph.ObjectTypeGraphExecution:
		if e.Done {
			return event.EnvelopeTypeGraphExecutionDone
		}
		return ""
	default:
		return ""
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
