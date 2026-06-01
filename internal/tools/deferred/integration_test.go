package deferred

import (
	"context"
	"sync/atomic"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type weatherInput struct {
	City string `json:"city" jsonschema:"description=City name,required"`
}

type weatherOutput struct {
	Temperature string `json:"temperature"`
	Condition   string `json:"condition"`
}

type translateInput struct {
	Text   string `json:"text" jsonschema:"description=Text to translate,required"`
	Target string `json:"target" jsonschema:"description=Target language,required"`
}

type translateOutput struct {
	Translation string `json:"translation"`
}

type calcInput struct {
	Expression string `json:"expression" jsonschema:"description=Math expression,required"`
}

type calcOutput struct {
	Result string `json:"result"`
}

func createWeatherTool(_ context.Context) (trpctool.Tool, error) {
	return trpcfunction.NewFunctionTool(
		func(_ context.Context, in weatherInput) (weatherOutput, error) {
			return weatherOutput{Temperature: "22°C", Condition: "sunny"}, nil
		},
		trpcfunction.WithName("weather_lookup"),
		trpcfunction.WithDescription("Look up weather for a city"),
	), nil
}

func createTranslateTool(_ context.Context) (trpctool.Tool, error) {
	return trpcfunction.NewFunctionTool(
		func(_ context.Context, in translateInput) (translateOutput, error) {
			return translateOutput{Translation: "hola"}, nil
		},
		trpcfunction.WithName("translate_text"),
		trpcfunction.WithDescription("Translate text between languages"),
	), nil
}

func createCalcTool(_ context.Context) (trpctool.Tool, error) {
	return trpcfunction.NewFunctionTool(
		func(_ context.Context, in calcInput) (calcOutput, error) {
			return calcOutput{Result: "42"}, nil
		},
		trpcfunction.WithName("calculator"),
		trpcfunction.WithDescription("Evaluate math expressions"),
	), nil
}

func buildCatalog() []DeferredToolEntry {
	return []DeferredToolEntry{
		{Name: "weather_lookup", Description: "Look up weather for a city", Category: "weather", Factory: createWeatherTool},
		{Name: "translate_text", Description: "Translate text between languages", Category: "language", Factory: createTranslateTool},
		{Name: "calculator", Description: "Evaluate math expressions", Category: "math", Factory: createCalcTool},
	}
}

func TestDeferredIntegration_SearchAndCreate(t *testing.T) {
	catalog := buildCatalog()
	searchTool := NewToolSearchTool(catalog)

	result, err := searchTool.Call(context.Background(), []byte(`{"query": "weather"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Tools))
	}
	if output.Tools[0].Name != "weather_lookup" {
		t.Fatalf("expected weather_lookup, got %s", output.Tools[0].Name)
	}

	tool, err := searchTool.FindAndCreate(context.Background(), "weather_lookup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decl := tool.Declaration()
	if decl.Name != "weather_lookup" {
		t.Fatalf("expected declaration name weather_lookup, got %s", decl.Name)
	}

	callResult, err := tool.(trpctool.CallableTool).Call(context.Background(), []byte(`{"city":"Beijing"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	weather, ok := callResult.(weatherOutput)
	if !ok {
		t.Fatalf("expected weatherOutput, got %T", callResult)
	}
	if weather.Temperature != "22°C" || weather.Condition != "sunny" {
		t.Fatalf("unexpected weather output: %+v", weather)
	}
}

func TestDeferredIntegration_DeferredCallableLazyResolution(t *testing.T) {
	var factoryCalls int32

	factory := func(_ context.Context) (trpctool.Tool, error) {
		atomic.AddInt32(&factoryCalls, 1)
		return trpcfunction.NewFunctionTool(
			func(_ context.Context, in weatherInput) (weatherOutput, error) {
				return weatherOutput{Temperature: "18°C", Condition: "cloudy"}, nil
			},
			trpcfunction.WithName("weather_lookup"),
			trpcfunction.WithDescription("Look up weather for a city"),
		), nil
	}

	decl := &trpctool.Declaration{
		Name:        "weather_lookup",
		Description: "Look up weather for a city",
	}
	dt := NewDeferredCallableTool(decl, factory, loggateway.Global())

	if atomic.LoadInt32(&factoryCalls) != 0 {
		t.Fatal("factory should not be called on construction")
	}

	gotDecl := dt.Declaration()
	if gotDecl.Name != "weather_lookup" {
		t.Fatalf("expected declaration name weather_lookup, got %s", gotDecl.Name)
	}

	callResult, err := dt.Call(context.Background(), []byte(`{"city":"Shanghai"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&factoryCalls) != 1 {
		t.Fatal("factory should be called on first Call")
	}
	weather, ok := callResult.(weatherOutput)
	if !ok {
		t.Fatalf("expected weatherOutput, got %T", callResult)
	}
	if weather.Temperature != "18°C" || weather.Condition != "cloudy" {
		t.Fatalf("unexpected weather output: %+v", weather)
	}

	_, err = dt.Call(context.Background(), []byte(`{"city":"Tokyo"}`))
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if atomic.LoadInt32(&factoryCalls) != 1 {
		t.Fatal("factory should not be called again on second Call (sync.Once)")
	}
}

func TestDeferredIntegration_SearchNoMatch(t *testing.T) {
	catalog := buildCatalog()
	searchTool := NewToolSearchTool(catalog)

	result, err := searchTool.Call(context.Background(), []byte(`{"query": "nonexistent_tool_xyz"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 0 {
		t.Fatalf("expected 0 results, got %d", len(output.Tools))
	}
	if output.Suggestion == "" {
		t.Fatal("expected suggestion for no results")
	}
}

func TestDeferredIntegration_CatalogNames(t *testing.T) {
	catalog := buildCatalog()
	searchTool := NewToolSearchTool(catalog)

	names := searchTool.CatalogNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	expected := map[string]bool{
		"weather_lookup": true,
		"translate_text": true,
		"calculator":     true,
	}
	for _, name := range names {
		if !expected[name] {
			t.Fatalf("unexpected catalog name: %s", name)
		}
	}
}

func TestDeferredIntegration_SearchAutoActivates(t *testing.T) {
	catalog := buildCatalog()
	searchTool := NewToolSearchTool(catalog)
	manager := searchTool.Manager()

	if manager.IsActivated("weather_lookup") {
		t.Fatal("weather_lookup should not be activated before search")
	}

	_, err := searchTool.Call(context.Background(), []byte(`{"query": "weather"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !manager.IsActivated("weather_lookup") {
		t.Fatal("weather_lookup should be discovered after search")
	}
}

func TestDeferredIntegration_ToolFilter(t *testing.T) {
	catalog := buildCatalog()
	searchTool := NewToolSearchTool(catalog)
	manager := searchTool.Manager()
	filter := manager.ToolFilter()

	weatherTool, _ := createWeatherTool(context.Background())
	translateTool, _ := createTranslateTool(context.Background())

	if filter(context.Background(), weatherTool) {
		t.Fatal("weather_lookup should be filtered out before activation")
	}
	if filter(context.Background(), translateTool) {
		t.Fatal("translate_text should be filtered out before activation")
	}

	manager.Activate(context.Background(), "weather_lookup")

	if !filter(context.Background(), weatherTool) {
		t.Fatal("weather_lookup should pass filter after activation")
	}
	if filter(context.Background(), translateTool) {
		t.Fatal("translate_text should still be filtered out")
	}
}
