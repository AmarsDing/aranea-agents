package plugintrpc

import (
	"encoding/json"
	"testing"
)

func TestMergeToolArgumentsJSON_replaceArguments(t *testing.T) {
	cur := []byte(`{"q":"hello"}`)
	patch := map[string]any{
		"arguments": map[string]any{"q": "replaced", "limit": 3},
	}
	out := mergeToolArgumentsJSON(cur, patch)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["q"] != "replaced" || m["limit"].(float64) != 3 {
		t.Fatalf("got %#v", m)
	}
}

func TestMergeToolArgumentsJSON_deepMergeNested(t *testing.T) {
	cur := []byte(`{"q":"hello","opts":{"page":1,"tags":["a"]}}`)
	patch := map[string]any{
		"merge_arguments": map[string]any{
			"opts": map[string]any{
				"page": 2,
				"extra": true,
			},
			"limit": 5,
		},
	}
	out := mergeToolArgumentsJSON(cur, patch)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["q"] != "hello" || m["limit"].(float64) != 5 {
		t.Fatalf("top: %#v", m)
	}
	opts, ok := m["opts"].(map[string]any)
	if !ok {
		t.Fatalf("opts=%#v", m["opts"])
	}
	if opts["page"].(float64) != 2 || opts["extra"] != true {
		t.Fatalf("opts=%#v", opts)
	}
	tags, ok := opts["tags"].([]any)
	if !ok || len(tags) != 1 {
		t.Fatalf("tags preserved=%#v", opts["tags"])
	}
}

func TestMergeToolArgumentsJSON_argumentsWinsOverMerge(t *testing.T) {
	cur := []byte(`{"q":"hello"}`)
	patch := map[string]any{
		"arguments":       map[string]any{"only": "replace"},
		"merge_arguments": map[string]any{"q": "ignored"},
	}
	out := mergeToolArgumentsJSON(cur, patch)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["only"] != "replace" {
		t.Fatalf("got %#v", m)
	}
}

func TestMergeToolArgumentsJSON_emptyCurrent(t *testing.T) {
	patch := map[string]any{
		"merge_arguments": map[string]any{"k": "v"},
	}
	out := mergeToolArgumentsJSON(nil, patch)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["k"] != "v" {
		t.Fatalf("got %#v", m)
	}
}
