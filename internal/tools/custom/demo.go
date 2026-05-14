package custom

import (
	"context"
	"fmt"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type DemoInput struct {
	Query string `json:"query" jsonschema:"description=The search query string,required"`
	Limit int    `json:"limit" jsonschema:"description=Maximum number of results to return,default=5"`
}

type DemoOutput struct {
	Results []DemoResult `json:"results"`
}

type DemoResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func demoExecute(ctx context.Context, input DemoInput) (DemoOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	out := DemoOutput{Results: make([]DemoResult, 0, limit)}
	for i := 0; i < limit; i++ {
		out.Results = append(out.Results, DemoResult{
			Title: fmt.Sprintf("Result %d for %q", i+1, input.Query),
			URL:   fmt.Sprintf("https://example.com/result/%d", i+1),
		})
	}
	return out, nil
}

func NewDemoTool() trpctool.Tool {
	return function.NewFunctionTool(
		demoExecute,
		function.WithName("demo_search"),
		function.WithDescription("A demo custom tool that simulates a search. Use this as a template for building your own tools."),
	)
}
