package testexec

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"aranea-agents/internal/biz"
)

// mockL1Store is an in-memory mock that satisfies L1AdminReader + L1TaskWriter + L1FieldWriter.
// It is used only by the test harness to exercise working_memory tools without a real database.
type mockL1Store struct {
	mu    sync.Mutex
	tasks map[string]*mockTask  // key = sessionID+"|"+taskID
	fields map[string]*mockField // key = taskID+"|"+fieldPath
	counter int
}

type mockTask struct {
	id        string
	sessionID string
	agentID   string
	taskKey   string
	taskTitle string
	taskGoal  string
	status    string
}

type mockField struct {
	id         string
	taskID     string
	sessionID  string
	agentID    string
	fieldPath  string
	fieldKind  string
	valueText  string
	pinToPrompt bool
	source     string
}

func newMockL1Store() *mockL1Store {
	return &mockL1Store{
		tasks:  make(map[string]*mockTask),
		fields: make(map[string]*mockField),
	}
}

// --- L1AdminReader ---

func (m *mockL1Store) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rows [][]byte
	for _, t := range m.tasks {
		if t.sessionID == sessionID && (agentID == "" || t.agentID == agentID) {
			if status == "" || t.status == status {
				b, _ := json.Marshal(t)
				rows = append(rows, b)
			}
		}
	}
	return rows, nil
}

func (m *mockL1Store) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rows [][]byte
	for _, f := range m.fields {
		if f.taskID == taskID {
			b, _ := json.Marshal(map[string]any{
				"id":            f.id,
				"task_id":       f.taskID,
				"field_path":    f.fieldPath,
				"field_kind":    f.fieldKind,
				"value_text":    f.valueText,
				"pin_to_prompt": f.pinToPrompt,
				"source":        f.source,
			})
			rows = append(rows, b)
		}
	}
	return rows, nil
}

func (m *mockL1Store) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[sessionID+"|"+id]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	b, _ := json.Marshal(t)
	return b, nil
}

func (m *mockL1Store) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.fields[taskID+"|"+fieldPath]
	if !ok {
		return nil, fmt.Errorf("field not found")
	}
	b, _ := json.Marshal(map[string]any{
		"id":            f.id,
		"task_id":       f.taskID,
		"field_path":    f.fieldPath,
		"field_kind":    f.fieldKind,
		"value_text":    f.valueText,
		"pin_to_prompt": f.pinToPrompt,
		"source":        f.source,
	})
	return b, nil
}

// --- L1TaskWriter ---

func (m *mockL1Store) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	id := fmt.Sprintf("mock-task-%d", m.counter)
	t := &mockTask{
		id:        id,
		sessionID: in.SessionID,
		agentID:   in.AgentID,
		taskKey:   in.TaskKey,
		taskTitle: in.TaskTitle,
		taskGoal:  in.TaskGoal,
		status:    "active",
	}
	m.tasks[in.SessionID+"|"+id] = t
	b, _ := json.Marshal(t)
	return b, nil
}

func (m *mockL1Store) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[sessionID+"|"+taskID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	t.status = status
	b, _ := json.Marshal(t)
	return b, nil
}

func (m *mockL1Store) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[sessionID+"|"+taskID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	t.status = "archived"
	b, _ := json.Marshal(t)
	return b, nil
}

// --- L1FieldWriter ---

func (m *mockL1Store) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := in.TaskID + "|" + in.FieldPath
	if _, exists := m.fields[key]; !exists {
		m.counter++
	}
	f := &mockField{
		id:          fmt.Sprintf("mock-field-%d", m.counter),
		taskID:      in.TaskID,
		sessionID:   in.SessionID,
		agentID:     in.AgentID,
		fieldPath:   in.FieldPath,
		fieldKind:   in.FieldKind,
		valueText:   in.ValueText,
		pinToPrompt: in.PinToPrompt,
		source:      in.Source,
	}
	m.fields[key] = f
	b, _ := json.Marshal(map[string]any{
		"id":            f.id,
		"task_id":       f.taskID,
		"field_path":    f.fieldPath,
		"value_text":    f.valueText,
		"pin_to_prompt": f.pinToPrompt,
	})
	return b, nil
}

func (m *mockL1Store) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.fields, taskID+"|"+fieldPath)
	return nil
}

func (m *mockL1Store) PatchL1Fields(ctx context.Context, fields []biz.L1FieldInsert) ([][]byte, error) {
	var rows [][]byte
	for _, in := range fields {
		b, err := m.UpsertL1Field(ctx, in)
		if err != nil {
			return nil, err
		}
		rows = append(rows, b)
	}
	return rows, nil
}

// Ensure mockL1Store satisfies the required interfaces at compile time.
var (
	_ biz.L1AdminReader  = (*mockL1Store)(nil)
	_ biz.L1TaskWriter   = (*mockL1Store)(nil)
	_ biz.L1FieldWriter  = (*mockL1Store)(nil)
)
