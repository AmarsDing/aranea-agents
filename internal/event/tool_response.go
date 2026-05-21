package event

import "trpc.group/trpc-go/trpc-agent-go/model"

// ToolNameFromResponse resolves a tool call name by id from an LLM response.
func ToolNameFromResponse(rsp *model.Response, id string) string {
	if rsp == nil {
		return ""
	}
	for _, choice := range rsp.Choices {
		for _, tc := range choice.Message.ToolCalls {
			if tc.ID == id {
				return tc.Function.Name
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			if tc.ID == id {
				return tc.Function.Name
			}
		}
	}
	return ""
}
