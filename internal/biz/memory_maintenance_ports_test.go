package biz

import "testing"

func TestMemoryFactWrite_SourceEpisodeID(t *testing.T) {
	f := MemoryFactWrite{
		Statement:       "test fact",
		SourceEpisodeID: "ep-123",
	}
	if f.SourceEpisodeID != "ep-123" {
		t.Errorf("SourceEpisodeID = %q, want %q", f.SourceEpisodeID, "ep-123")
	}
}

func TestEpisodeWrite_ID(t *testing.T) {
	ep := EpisodeWrite{
		ID:    "ep-456",
		Title: "Test Episode",
	}
	if ep.ID != "ep-456" {
		t.Errorf("ID = %q, want %q", ep.ID, "ep-456")
	}
}

func TestEpisodeWrite_ConsolidationStatus(t *testing.T) {
	ep := EpisodeWrite{
		ID:                 "ep-789",
		ConsolidationStatus: "consolidated",
	}
	if ep.ConsolidationStatus != "consolidated" {
		t.Errorf("ConsolidationStatus = %q, want %q", ep.ConsolidationStatus, "consolidated")
	}
}
