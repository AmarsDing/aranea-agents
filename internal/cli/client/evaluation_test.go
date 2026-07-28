package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
	"aranea-agents/internal/cli/client"
)

func TestListDatasets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/evaluation/datasets" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"ds1","name":"baseline","caseCount":3}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListDatasets(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ListDatasets: %v", err)
	}
	if len(resp.Items) != 1 || resp.Total != 1 {
		t.Errorf("expected 1 dataset, got %d (total %d)", len(resp.Items), resp.Total)
	}
}

func TestGetDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/evaluation/datasets/ds1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ds1","name":"baseline"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	ds, err := c.GetDataset(context.Background(), "ds1")
	if err != nil {
		t.Fatalf("GetDataset: %v", err)
	}
	if ds.Name != "baseline" {
		t.Errorf("expected name baseline, got %s", ds.Name)
	}
}

func TestCreateDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/evaluation/datasets" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"name":"baseline"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ds-new","name":"baseline"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &evaluationv1.CreateDatasetRequest{Name: "baseline", Description: "d"}
	ds, err := c.CreateDataset(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}
	if ds.Id != "ds-new" {
		t.Errorf("expected id ds-new, got %s", ds.Id)
	}
}

func TestListEvalRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/evaluation/runs" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("dataset_id") != "ds1" {
			t.Errorf("missing dataset_id: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"r1","datasetId":"ds1","status":"completed"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListEvalRuns(context.Background(), "ds1", "", 0, 0)
	if err != nil {
		t.Fatalf("ListEvalRuns: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 run, got %d", len(resp.Items))
	}
}

func TestGetEvalRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/evaluation/runs/r1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r1","status":"running","totalCases":10,"completedCases":4}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	run, err := c.GetEvalRun(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetEvalRun: %v", err)
	}
	if run.Status != "running" || run.CompletedCases != 4 {
		t.Errorf("unexpected run: %+v", run)
	}
}

func TestRunEvaluation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/evaluation/runs" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"datasetId":"ds1"`) || !strings.Contains(string(body), `"agentId":"a1"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r-new","status":"pending"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &evaluationv1.RunEvaluationRequest{DatasetId: "ds1", AgentId: "a1", NumRuns: 1}
	run, err := c.RunEvaluation(context.Background(), req)
	if err != nil {
		t.Fatalf("RunEvaluation: %v", err)
	}
	if run.Id != "r-new" {
		t.Errorf("expected id r-new, got %s", run.Id)
	}
}

func TestGetRunResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/evaluation/runs/r1/results" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"res1","caseId":"case1","exactMatch":true}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.GetRunResults(context.Background(), "r1", 0, 0)
	if err != nil {
		t.Fatalf("GetRunResults: %v", err)
	}
	if len(resp.Items) != 1 || !resp.Items[0].ExactMatch {
		t.Errorf("unexpected results: %+v", resp.Items)
	}
}
