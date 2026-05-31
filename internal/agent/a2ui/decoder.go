package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
)

type Decoder struct{}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) DecodeUserAction(ctx context.Context, payload []byte) (*UserAction, error) {
	var wrapper struct {
		UserAction *UserAction `json:"userAction"`
		Error      any         `json:"error"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, fmt.Errorf("a2ui: decode user payload: %w", err)
	}
	if wrapper.UserAction != nil {
		return wrapper.UserAction, nil
	}
	if wrapper.Error != nil {
		errBytes, _ := json.Marshal(wrapper.Error)
		return nil, fmt.Errorf("a2ui: client error: %s", string(errBytes))
	}
	return nil, fmt.Errorf("a2ui: payload has neither userAction nor error")
}

func (d *Decoder) DecodeServerMessage(ctx context.Context, line []byte) (any, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("a2ui: decode server message: %w", err)
	}

	if v, ok := raw["beginRendering"]; ok {
		var msg BeginRendering
		if err := json.Unmarshal(v, &msg); err != nil {
			return nil, fmt.Errorf("a2ui: decode beginRendering: %w", err)
		}
		return &msg, nil
	}

	if v, ok := raw["surfaceUpdate"]; ok {
		var msg SurfaceUpdate
		if err := json.Unmarshal(v, &msg); err != nil {
			return nil, fmt.Errorf("a2ui: decode surfaceUpdate: %w", err)
		}
		return &msg, nil
	}

	if v, ok := raw["dataModelUpdate"]; ok {
		var msg DataModelUpdate
		if err := json.Unmarshal(v, &msg); err != nil {
			return nil, fmt.Errorf("a2ui: decode dataModelUpdate: %w", err)
		}
		return &msg, nil
	}

	if v, ok := raw["deleteSurface"]; ok {
		var msg DeleteSurface
		if err := json.Unmarshal(v, &msg); err != nil {
			return nil, fmt.Errorf("a2ui: decode deleteSurface: %w", err)
		}
		return &msg, nil
	}

	return nil, fmt.Errorf("a2ui: unknown server message keys: %v", keys(raw))
}

func (d *Decoder) IsApproval(action *UserAction) bool {
	return action.Name == "approve"
}

func (d *Decoder) IsRejection(action *UserAction) bool {
	return action.Name == "reject"
}

func (d *Decoder) ActionPlanID(action *UserAction) string {
	if action.Context == nil {
		return ""
	}
	v, _ := action.Context["planId"].(string)
	return v
}

func keys(m map[string]json.RawMessage) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
