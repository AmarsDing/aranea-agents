package graph

import "testing"

func TestContentPipelineBuiltinRegistered(t *testing.T) {
	tmpl := GetBuiltinTemplate("content_pipeline")
	if tmpl == nil {
		t.Fatal("content_pipeline missing from builtinTemplates")
	}
	if tmpl.EntryPoint != "ingest_or_pick" || tmpl.FinishPoint != "build_publish_package" {
		t.Fatalf("entry/finish: %+v", tmpl)
	}
	want := map[string]bool{"ingest_or_pick": true, "team_produce": true, "machine_scan": true, "human_review": true, "build_publish_package": true}
	for _, n := range tmpl.Nodes {
		delete(want, n.NodeID)
	}
	if len(want) != 0 {
		t.Fatalf("missing nodes: %v", want)
	}
}
