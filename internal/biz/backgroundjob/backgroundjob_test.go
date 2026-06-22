package backgroundjob

import (
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestJob_IsDone(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{name: "queued is not done", status: StatusQueued, want: false},
		{name: "claimed is not done", status: StatusClaimed, want: false},
		{name: "succeeded is done", status: StatusSucceeded, want: true},
		{name: "failed is done", status: StatusFailed, want: true},
		{name: "cancelled is done", status: StatusCancelled, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{Status: tt.status}
			if got := j.IsDone(); got != tt.want {
				t.Fatalf("IsDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Status
		want string
	}{
		{name: "StatusQueued", got: StatusQueued, want: "queued"},
		{name: "StatusClaimed", got: StatusClaimed, want: "claimed"},
		{name: "StatusSucceeded", got: StatusSucceeded, want: "succeeded"},
		{name: "StatusFailed", got: StatusFailed, want: "failed"},
		{name: "StatusCancelled", got: StatusCancelled, want: "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestPriorityConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "PriorityRealtime", got: PriorityRealtime, want: 10},
		{name: "PriorityNormal", got: PriorityNormal, want: 50},
		{name: "PriorityBackground", got: PriorityBackground, want: 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %d, want %d", tt.got, tt.want)
			}
		})
	}
}

func TestOwnerTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  OwnerType
		want string
	}{
		{name: "OwnerTypeSession", got: OwnerTypeSession, want: "session"},
		{name: "OwnerTypeChannel", got: OwnerTypeChannel, want: "channel"},
		{name: "OwnerTypeSystem", got: OwnerTypeSystem, want: "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestErrNotFound(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound should not be nil")
	}
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("ErrNotFound should match itself via errors.Is")
	}
	ae, ok := apierror.From(ErrNotFound)
	if !ok {
		t.Fatalf("ErrNotFound should be apierror, got %T", ErrNotFound)
	}
	if ae.Code != apierror.CodeNotFound {
		t.Fatalf("ErrNotFound should be NotFound apierror, got %v", ErrNotFound)
	}
}
