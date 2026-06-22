package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
)

type Encoder struct{}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) EncodeBeginRendering(ctx context.Context, msg BeginRendering) (json.RawMessage, error) {
	wrapper := struct {
		BeginRendering BeginRendering `json:"beginRendering"`
	}{BeginRendering: msg}
	return json.Marshal(wrapper)
}

func (e *Encoder) EncodeSurfaceUpdate(ctx context.Context, msg SurfaceUpdate) (json.RawMessage, error) {
	wrapper := struct {
		SurfaceUpdate SurfaceUpdate `json:"surfaceUpdate"`
	}{SurfaceUpdate: msg}
	return json.Marshal(wrapper)
}

func (e *Encoder) EncodeDataModelUpdate(ctx context.Context, msg DataModelUpdate) (json.RawMessage, error) {
	wrapper := struct {
		DataModelUpdate DataModelUpdate `json:"dataModelUpdate"`
	}{DataModelUpdate: msg}
	return json.Marshal(wrapper)
}

func (e *Encoder) EncodeDeleteSurface(ctx context.Context, msg DeleteSurface) (json.RawMessage, error) {
	wrapper := struct {
		DeleteSurface DeleteSurface `json:"deleteSurface"`
	}{DeleteSurface: msg}
	return json.Marshal(wrapper)
}

func (e *Encoder) EncodePlanAsSurface(ctx context.Context, plan *Plan, surfaceID string) ([]json.RawMessage, error) {
	var messages []json.RawMessage

	beginMsg, err := e.EncodeBeginRendering(ctx, BeginRendering{
		SurfaceID: surfaceID,
		Root:      "plan_root",
		Styles: &SurfaceStyles{
			PrimaryColor: "#00BFFF",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode beginRendering: %w", err)
	}
	messages = append(messages, beginMsg)

	components := e.buildPlanComponents(plan)
	updateMsg, err := e.EncodeSurfaceUpdate(ctx, SurfaceUpdate{
		SurfaceID:  surfaceID,
		Components: components,
	})
	if err != nil {
		return nil, fmt.Errorf("encode surfaceUpdate: %w", err)
	}
	messages = append(messages, updateMsg)

	dataMsg, err := e.EncodeDataModelUpdate(ctx, DataModelUpdate{
		SurfaceID: surfaceID,
		Contents:  e.buildPlanDataModel(plan),
	})
	if err != nil {
		return nil, fmt.Errorf("encode dataModelUpdate: %w", err)
	}
	messages = append(messages, dataMsg)

	return messages, nil
}

func (e *Encoder) buildPlanComponents(plan *Plan) []Component {
	var components []Component

	components = append(components, Component{
		ID: "plan_root",
		Component: ComponentBody{Column: &ColumnComponent{
			Children: ChildrenDef{ExplicitList: []string{"plan_title", "plan_steps", "plan_actions"}},
		}},
	})

	components = append(components, Component{
		ID: "plan_title",
		Component: ComponentBody{Text: &TextComponent{
			Text:      DataBinding{LiteralString: StrPtr(plan.Goal)},
			UsageHint: "h2",
		}},
	})

	var stepIDs []string
	for _, step := range plan.Steps {
		stepID := "step_" + step.ID
		stepIDs = append(stepIDs, stepID)
		components = append(components, Component{
			ID: stepID,
			Component: ComponentBody{Card: &CardComponent{
				Child: stepID + "_inner",
			}},
		})
		components = append(components, Component{
			ID: stepID + "_inner",
			Component: ComponentBody{Column: &ColumnComponent{
				Children: ChildrenDef{ExplicitList: []string{stepID + "_name", stepID + "_desc", stepID + "_status"}},
			}},
		})
		components = append(components, Component{
			ID: stepID + "_name",
			Component: ComponentBody{Text: &TextComponent{
				Text:      DataBinding{Path: StrPtr("/steps/" + step.ID + "/name")},
				UsageHint: "h4",
			}},
		})
		components = append(components, Component{
			ID: stepID + "_desc",
			Component: ComponentBody{Text: &TextComponent{
				Text: DataBinding{Path: StrPtr("/steps/" + step.ID + "/description")},
			}},
		})
		components = append(components, Component{
			ID: stepID + "_status",
			Component: ComponentBody{Text: &TextComponent{
				Text:      DataBinding{Path: StrPtr("/steps/" + step.ID + "/status")},
				UsageHint: "caption",
			}},
		})
	}

	components = append(components, Component{
		ID: "plan_steps",
		Component: ComponentBody{Column: &ColumnComponent{
			Children: ChildrenDef{ExplicitList: stepIDs},
		}},
	})

	components = append(components, Component{
		ID: "plan_actions",
		Component: ComponentBody{Row: &RowComponent{
			Children:     ChildrenDef{ExplicitList: []string{"btn_approve", "btn_reject"}},
			Distribution: "spaceEvenly",
		}},
	})

	components = append(components, Component{
		ID: "btn_approve",
		Component: ComponentBody{Button: &ButtonComponent{
			Child:   "btn_approve_text",
			Primary: true,
			Action: ActionDef{
				Name: "approve",
				Context: []ActionContextEntry{
					{Key: "planId", Value: DataBinding{LiteralString: StrPtr(plan.ID)}},
				},
			},
		}},
	})
	components = append(components, Component{
		ID: "btn_approve_text",
		Component: ComponentBody{Text: &TextComponent{
			Text: DataBinding{LiteralString: StrPtr("Approve")},
		}},
	})

	components = append(components, Component{
		ID: "btn_reject",
		Component: ComponentBody{Button: &ButtonComponent{
			Child:   "btn_reject_text",
			Primary: false,
			Action: ActionDef{
				Name: "reject",
				Context: []ActionContextEntry{
					{Key: "planId", Value: DataBinding{LiteralString: StrPtr(plan.ID)}},
				},
			},
		}},
	})
	components = append(components, Component{
		ID: "btn_reject_text",
		Component: ComponentBody{Text: &TextComponent{
			Text: DataBinding{LiteralString: StrPtr("Reject")},
		}},
	})

	return components
}

func (e *Encoder) buildPlanDataModel(plan *Plan) []DataEntry {
	entries := []DataEntry{
		{Key: "goal", ValueString: StrPtr(plan.Goal)},
	}

	var stepEntries []DataEntry
	for _, step := range plan.Steps {
		stepEntries = append(stepEntries, DataEntry{
			Key: step.ID,
			ValueMap: []DataEntry{
				{Key: "name", ValueString: StrPtr(step.Name)},
				{Key: "description", ValueString: StrPtr(step.Description)},
				{Key: "status", ValueString: StrPtr("pending")},
				{Key: "agent", ValueString: StrPtr(step.AgentName)},
			},
		})
	}
	entries = append(entries, DataEntry{Key: "steps", ValueMap: stepEntries})

	return entries
}

func (e *Encoder) EncodeStepProgressUpdate(ctx context.Context, surfaceID, stepID, status string) (json.RawMessage, error) {
	return e.EncodeDataModelUpdate(ctx, DataModelUpdate{
		SurfaceID: surfaceID,
		Path:      "/steps/" + stepID,
		Contents: []DataEntry{
			{Key: "status", ValueString: StrPtr(status)},
		},
	})
}
