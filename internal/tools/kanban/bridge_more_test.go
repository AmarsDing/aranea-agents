package kanban

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTool_Call_showFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envTask string
		showFn  func(ctx context.Context, taskID string) (map[string]any, error)
		wantErr bool
		wantID  string
	}{
		{
			name:    "explicit task_id",
			args:    `{"task_id":"t1"}`,
			envTask: "",
			showFn: func(_ context.Context, taskID string) (map[string]any, error) {
				return map[string]any{"id": taskID}, nil
			},
			wantErr: false,
			wantID:  "t1",
		},
		{
			name:    "fallback to env",
			args:    `{}`,
			envTask: "env-task",
			showFn: func(_ context.Context, taskID string) (map[string]any, error) {
				return map[string]any{"id": taskID}, nil
			},
			wantErr: false,
			wantID:  "env-task",
		},
		{
			name:    "missing task_id",
			args:    `{}`,
			envTask: "",
			showFn:  nil,
			wantErr: true,
		},
		{
			name:    "whitespace task_id falls back to env",
			args:    `{"task_id":"  "}`,
			envTask: "env-task",
			showFn: func(_ context.Context, taskID string) (map[string]any, error) {
				return map[string]any{"id": taskID}, nil
			},
			wantErr: false,
			wantID:  "env-task",
		},
		{
			name:    "invalid json",
			args:    `{bad`,
			envTask: "",
			showFn:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			b := &stubBridge{}
			if tt.showFn != nil {
				b.showFn = tt.showFn
			}
			ts := NewToolset(b)
			result, err := ts[0].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil && tt.wantID != "" {
				m, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("expected map, got %T", result)
				}
				if m["id"] != tt.wantID {
					t.Fatalf("expected id=%q got %q", tt.wantID, m["id"])
				}
			}
		})
	}
}

func TestTool_Call_listFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envExec string
		wantErr bool
		wantLen int
	}{
		{
			name:    "explicit execution_id",
			args:    `{"execution_id":"e1","limit":5}`,
			envExec: "",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "fallback to env execution_id",
			args:    `{"limit":10}`,
			envExec: "env-exec",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "missing execution_id",
			args:    `{}`,
			envExec: "",
			wantErr: true,
		},
		{
			name:    "default limit when zero",
			args:    `{"execution_id":"e1","limit":0}`,
			envExec: "",
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "invalid json",
			args:    `{bad`,
			envExec: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_EXECUTION_ID", tt.envExec)
			b := &stubBridge{
				listFn: func(_ context.Context, executionID, status string, limit int) ([]map[string]any, error) {
					return []map[string]any{{"id": executionID}}, nil
				},
			}
			ts := NewToolset(b)
			result, err := ts[1].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil {
				m, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("expected map, got %T", result)
				}
				items, _ := m["items"].([]map[string]any)
				if len(items) != tt.wantLen {
					t.Fatalf("expected %d items got %d", tt.wantLen, len(items))
				}
			}
		})
	}
}

func TestTool_Call_completeFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envTask string
		wantErr bool
	}{
		{
			name:    "with task_id and summary",
			args:    `{"task_id":"t1","summary":"done"}`,
			envTask: "",
			wantErr: false,
		},
		{
			name:    "result alias for output",
			args:    `{"task_id":"t1","result":"output-data"}`,
			envTask: "",
			wantErr: false,
		},
		{
			name:    "summary falls back to output",
			args:    `{"task_id":"t1","output":"out"}`,
			envTask: "",
			wantErr: false,
		},
		{
			name:    "missing task_id and summary",
			args:    `{}`,
			envTask: "",
			wantErr: true,
		},
		{
			name:    "missing summary with empty output",
			args:    `{"task_id":"t1"}`,
			envTask: "",
			wantErr: true,
		},
		{
			name:    "fallback to env task_id",
			args:    `{"summary":"done"}`,
			envTask: "env-task",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			b := &stubBridge{
				completeFn: func(_ context.Context, taskID, agentKey, summary, output, metadata string) (map[string]any, error) {
					return map[string]any{"task_id": taskID, "agent_key": agentKey, "status": "completed"}, nil
				},
			}
			ts := NewToolset(b)
			_, err := ts[2].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTool_Call_blockFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envTask string
		wantErr bool
	}{
		{
			name:    "valid block",
			args:    `{"task_id":"t1","reason":"need input"}`,
			envTask: "",
			wantErr: false,
		},
		{
			name:    "missing reason",
			args:    `{"task_id":"t1"}`,
			envTask: "",
			wantErr: true,
		},
		{
			name:    "missing task_id",
			args:    `{"reason":"need input"}`,
			envTask: "",
			wantErr: true,
		},
		{
			name:    "fallback to env task_id",
			args:    `{"reason":"need input"}`,
			envTask: "env-task",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			b := &stubBridge{
				blockFn: func(_ context.Context, taskID, reason, metadata string) (map[string]any, error) {
					return map[string]any{"status": "blocked"}, nil
				},
			}
			ts := NewToolset(b)
			_, err := ts[3].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTool_Call_unblockFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envTask string
		wantErr bool
	}{
		{
			name:    "valid unblock",
			args:    `{"task_id":"t1","comment":"resolved"}`,
			envTask: "",
			wantErr: false,
		},
		{
			name:    "missing task_id",
			args:    `{"comment":"resolved"}`,
			envTask: "",
			wantErr: true,
		},
		{
			name:    "fallback to env task_id",
			args:    `{"comment":"ok"}`,
			envTask: "env-task",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			b := &stubBridge{
				unblockFn: func(_ context.Context, taskID, comment string) (map[string]any, error) {
					return map[string]any{"status": "pending"}, nil
				},
			}
			ts := NewToolset(b)
			_, err := ts[4].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTool_Call_heartbeatFn(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		envTask  string
		envAgent string
		wantErr  bool
	}{
		{
			name:     "valid heartbeat",
			args:     `{"task_id":"t1","agent_key":"a1"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  false,
		},
		{
			name:     "missing agent_key fallback to env",
			args:     `{"task_id":"t1"}`,
			envTask:  "",
			envAgent: "env-agent",
			wantErr:  false,
		},
		{
			name:     "missing both task_id and agent_key",
			args:     `{}`,
			envTask:  "",
			envAgent: "",
			wantErr:  true,
		},
		{
			name:     "missing agent_key no env",
			args:     `{"task_id":"t1"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  true,
		},
		{
			name:     "fallback task_id from env",
			args:     `{"agent_key":"a1"}`,
			envTask:  "env-task",
			envAgent: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			t.Setenv("ARANEA_AGENT_KEY", tt.envAgent)
			b := &stubBridge{
				heartbeatFn: func(_ context.Context, taskID, agentKey, metadata string) (map[string]any, error) {
					return map[string]any{"alive": true}, nil
				},
			}
			ts := NewToolset(b)
			_, err := ts[5].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTool_Call_commentFn(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		envTask  string
		envAgent string
		wantErr  bool
	}{
		{
			name:     "valid comment with body",
			args:     `{"task_id":"t1","body":"hello"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  false,
		},
		{
			name:     "content alias for body",
			args:     `{"task_id":"t1","content":"hello"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  false,
		},
		{
			name:     "body preferred over content",
			args:     `{"task_id":"t1","body":"b","content":"c"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  false,
		},
		{
			name:     "missing body and content",
			args:     `{"task_id":"t1"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  true,
		},
		{
			name:     "missing task_id",
			args:     `{"body":"hello"}`,
			envTask:  "",
			envAgent: "",
			wantErr:  true,
		},
		{
			name:     "author fallback to env",
			args:     `{"task_id":"t1","body":"hello"}`,
			envTask:  "",
			envAgent: "env-agent",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_TASK_ID", tt.envTask)
			t.Setenv("ARANEA_AGENT_KEY", tt.envAgent)
			var capturedAuthor string
			b := &stubBridge{
				commentFn: func(_ context.Context, taskID, author, body, commentType string) (map[string]any, error) {
					capturedAuthor = author
					return map[string]any{"author": author}, nil
				},
			}
			ts := NewToolset(b)
			result, err := ts[6].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil && tt.envAgent != "" {
				m := result.(map[string]any)
				if m["author"] != tt.envAgent {
					t.Fatalf("expected author=%q got %q", tt.envAgent, m["author"])
				}
			}
			_ = capturedAuthor
		})
	}
}

func TestTool_Call_createFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		envExec string
		wantErr bool
	}{
		{
			name:    "valid create",
			args:    `{"execution_id":"e1","title":"New Task"}`,
			envExec: "",
			wantErr: false,
		},
		{
			name:    "missing title",
			args:    `{"execution_id":"e1"}`,
			envExec: "",
			wantErr: true,
		},
		{
			name:    "missing execution_id",
			args:    `{"title":"New Task"}`,
			envExec: "",
			wantErr: true,
		},
		{
			name:    "fallback to env execution_id",
			args:    `{"title":"New Task"}`,
			envExec: "env-exec",
			wantErr: false,
		},
		{
			name:    "input defaults to title",
			args:    `{"execution_id":"e1","title":"New Task"}`,
			envExec: "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARANEA_EXECUTION_ID", tt.envExec)
			var capturedInput string
			b := &stubBridge{
				createFn: func(_ context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
					capturedInput = input
					return map[string]any{"title": title}, nil
				},
			}
			ts := NewToolset(b)
			_, err := ts[7].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil && tt.name == "input defaults to title" {
				if capturedInput != "New Task" {
					t.Fatalf("expected input='New Task' got %q", capturedInput)
				}
			}
		})
	}
}

func TestTool_Call_linkFn(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
		wantOk  bool
	}{
		{
			name:    "valid link",
			args:    `{"parent_id":"p1","child_id":"c1"}`,
			wantErr: false,
			wantOk:  true,
		},
		{
			name:    "missing parent_id",
			args:    `{"child_id":"c1"}`,
			wantErr: true,
		},
		{
			name:    "missing child_id",
			args:    `{"parent_id":"p1"}`,
			wantErr: true,
		},
		{
			name:    "missing both",
			args:    `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &stubBridge{
				linkFn: func(_ context.Context, parentTaskID, childTaskID string) error {
					return nil
				},
			}
			ts := NewToolset(b)
			result, err := ts[8].Call(context.Background(), []byte(tt.args))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err == nil && tt.wantOk {
				m, ok := result.(map[string]any)
				if !ok || m["ok"] != true {
					t.Fatalf("expected ok=true got %v", m)
				}
			}
		})
	}
}

func TestTool_Call_nilBridge(t *testing.T) {
	ts := NewToolset(&stubBridge{})
	t0 := ts[0]
	toolImpl := t0.(*tool)
	toolImpl.bridge = nil
	_, err := toolImpl.Call(context.Background(), []byte(`{"task_id":"t1"}`))
	if err == nil {
		t.Fatal("expected error for nil bridge")
	}
}

func TestTool_StreamableCall_returnsError(t *testing.T) {
	b := &stubBridge{}
	ts := NewToolset(b)
	toolImpl := ts[0].(*tool)
	_, err := toolImpl.StreamableCall(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for StreamableCall")
	}
}

func TestTool_Declaration_hasSchema(t *testing.T) {
	b := &stubBridge{}
	ts := NewToolset(b)
	for i, tool := range ts {
		decl := tool.Declaration()
		if decl.InputSchema == nil {
			t.Fatalf("tool[%d] %q has nil InputSchema", i, decl.Name)
		}
		if decl.InputSchema.Type != "object" {
			t.Fatalf("tool[%d] %q schema type=%q want object", i, decl.Name, decl.InputSchema.Type)
		}
		if len(decl.InputSchema.Properties) == 0 {
			t.Fatalf("tool[%d] %q has no properties", i, decl.Name)
		}
	}
}

func TestTool_Call_commentFn_bodyPreferredOverContent(t *testing.T) {
	t.Setenv("ARANEA_TASK_ID", "")
	t.Setenv("ARANEA_AGENT_KEY", "")
	var capturedBody string
	b := &stubBridge{
		commentFn: func(_ context.Context, taskID, author, body, commentType string) (map[string]any, error) {
			capturedBody = body
			return map[string]any{}, nil
		},
	}
	ts := NewToolset(b)
	_, err := ts[6].Call(context.Background(), []byte(`{"task_id":"t1","body":"body-val","content":"content-val"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if capturedBody != "body-val" {
		t.Fatalf("expected body=body-val got %q", capturedBody)
	}
}

func TestTool_Call_createFn_withParents(t *testing.T) {
	var capturedParents []string
	b := &stubBridge{
		createFn: func(_ context.Context, executionID, nodeID, title, assignee, input string, parentIDs []string) (map[string]any, error) {
			capturedParents = parentIDs
			return map[string]any{}, nil
		},
	}
	ts := NewToolset(b)
	args, _ := json.Marshal(map[string]any{
		"execution_id": "e1",
		"title":        "child",
		"parents":      []string{"p1", "p2"},
	})
	_, err := ts[7].Call(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(capturedParents) != 2 || capturedParents[0] != "p1" || capturedParents[1] != "p2" {
		t.Fatalf("expected parents [p1 p2] got %v", capturedParents)
	}
}
