package service

import (
	"context"
	"testing"
)

func TestEnforceQuota_nilUsage(t *testing.T) {
	err := enforceQuota(context.Background(), nil, "agent", "a1")
	if err != nil {
		t.Fatalf("nil usage should return nil, got %v", err)
	}
}

func TestEnforceQuota_emptyScopeType(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		scopeID   string
	}{
		{"empty_scope_type", "", "a1"},
		{"whitespace_scope_type", "   ", "a1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforceQuota(context.Background(), nil, tt.scopeType, tt.scopeID)
			if err != nil {
				t.Fatalf("empty scopeType should return nil, got %v", err)
			}
		})
	}
}

func TestEnforceQuota_emptyScopeID(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		scopeID   string
	}{
		{"empty_scope_id", "agent", ""},
		{"whitespace_scope_id", "agent", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforceQuota(context.Background(), nil, tt.scopeType, tt.scopeID)
			if err != nil {
				t.Fatalf("empty scopeID should return nil, got %v", err)
			}
		})
	}
}

func TestEnforceQuota_bothEmpty(t *testing.T) {
	err := enforceQuota(context.Background(), nil, "", "")
	if err != nil {
		t.Fatalf("both empty should return nil, got %v", err)
	}
}

func TestEnforceChatTurnQuotas_nilUsage(t *testing.T) {
	err := enforceChatTurnQuotas(context.Background(), nil, "agent-1", "user-1")
	if err != nil {
		t.Fatalf("nil usage should return nil, got %v", err)
	}
}

func TestEnforceChatTurnQuotas_nilUsageEmptyIDs(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		userID   string
	}{
		{"both_empty", "", ""},
		{"agent_empty", "", "user-1"},
		{"user_empty", "agent-1", ""},
		{"both_whitespace", "   ", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforceChatTurnQuotas(context.Background(), nil, tt.agentID, tt.userID)
			if err != nil {
				t.Fatalf("nil usage with empty IDs should return nil, got %v", err)
			}
		})
	}
}
