package a2a

import (
	"encoding/json"
	"strings"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// MessageMetadataStateDeltaKey is the A2A message metadata key for graph state_delta.
const MessageMetadataStateDeltaKey = "state_delta"

// Metadata keys for Admin / remote invoke capability routing.
const (
	MetadataKeyCapability = "aranea_capability"
)

// GraphResumeInput carries checkpoint + resume payload for A2A graph interrupt/resume.
type GraphResumeInput struct {
	LineageID    string
	CheckpointID string
	CheckpointNS string
	Resume       any
	ResumeMap    map[string]any
}

// BuildGraphResumeMetadata encodes graph resume fields for A2A message metadata.
// Uses flattened keys compatible with trpc-agent-go GraphResumeStateFromMetadata.
func BuildGraphResumeMetadata(in GraphResumeInput) map[string]any {
	meta := make(map[string]any, 8)
	if s := strings.TrimSpace(in.LineageID); s != "" {
		meta[trpcgraph.CfgKeyLineageID] = s
	}
	if s := strings.TrimSpace(in.CheckpointID); s != "" {
		meta[trpcgraph.CfgKeyCheckpointID] = s
	}
	if s := strings.TrimSpace(in.CheckpointNS); s != "" {
		meta[trpcgraph.CfgKeyCheckpointNS] = s
	}
	if in.Resume != nil {
		meta["resume"] = in.Resume
	}
	if len(in.ResumeMap) > 0 {
		meta[trpcgraph.CfgKeyResumeMap] = cloneAnyMap(in.ResumeMap)
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// EncodeGraphResumeStateDeltaJSON returns JSON for state_delta envelope payloads.
func EncodeGraphResumeStateDeltaJSON(in GraphResumeInput) (string, error) {
	delta := encodeGraphStateDeltaEnvelope(in)
	if len(delta) == 0 {
		return "", nil
	}
	b, err := json.Marshal(delta)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func encodeGraphStateDeltaEnvelope(in GraphResumeInput) map[string]any {
	flat := BuildGraphResumeMetadata(in)
	if len(flat) == 0 {
		return nil
	}
	return map[string]any{MessageMetadataStateDeltaKey: flat}
}
