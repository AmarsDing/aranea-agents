package trpcmem

import (
	"context"
	"strings"
	"testing"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// TestProactiveRecall_SingleEntityMention verifies that ProactiveRecall
// retrieves memories containing the mentioned entity.
func TestProactiveRecall_SingleEntityMention(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-1", UserID: "user-pr-1"}

	if err := svc.AddMemory(ctx, uk, "User lives in London", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "User likes pizza", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"London"},
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry mentioning London")
	}
	found := false
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Memory.Memory), "london") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find memory mentioning London")
	}
}

// TestProactiveRecall_MultipleEntityMentions verifies that ProactiveRecall
// retrieves memories related to any of the mentioned entities.
func TestProactiveRecall_MultipleEntityMentions(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-2", UserID: "user-pr-2"}

	if err := svc.AddMemory(ctx, uk, "User likes coffee", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "User works as engineer", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "User has a dog", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"coffee", "engineer"},
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	// Should return memories about coffee and engineer, but not dog.
	foundCoffee, foundEngineer, foundDog := false, false, false
	for _, e := range entries {
		low := strings.ToLower(e.Memory.Memory)
		if strings.Contains(low, "coffee") {
			foundCoffee = true
		}
		if strings.Contains(low, "engineer") {
			foundEngineer = true
		}
		if strings.Contains(low, "dog") {
			foundDog = true
		}
	}
	if !foundCoffee {
		t.Error("expected to find memory about coffee")
	}
	if !foundEngineer {
		t.Error("expected to find memory about engineer")
	}
	if foundDog {
		t.Error("should not return unrelated memory about dog")
	}
}

// TestProactiveRecall_TopicMatch verifies that ProactiveRecall retrieves
// memories related to the current topic.
func TestProactiveRecall_TopicMatch(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-3", UserID: "user-pr-3"}

	if err := svc.AddMemory(ctx, uk, "User prefers dark theme", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "User speaks French", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		CurrentTopic: "theme",
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Memory.Memory), "theme") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find memory about theme")
	}
}

// TestProactiveRecall_ContradictionDetection verifies that ProactiveRecall
// returns memories that may contradict the user's statement.
func TestProactiveRecall_ContradictionDetection(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-4", UserID: "user-pr-4"}

	// Store a memory that the user lives in London.
	if err := svc.AddMemory(ctx, uk, "User lives in London", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	// User now claims to live in Paris — potential contradiction.
	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		UserStatement: "I live in Paris",
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	// Should return the London memory since it's relevant to the user's
	// statement about where they live (potential contradiction).
	found := false
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Memory.Memory), "london") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find London memory for contradiction detection")
	}
}

// TestProactiveRecall_EmptyInput verifies that ProactiveRecall returns an
// empty list (not an error) when the conversation context is empty.
func TestProactiveRecall_EmptyInput(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-5", UserID: "user-pr-5"}

	if err := svc.AddMemory(ctx, uk, "User likes tea", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{})
	if err != nil {
		t.Fatalf("ProactiveRecall with empty context: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty context, got %d", len(entries))
	}
}

// TestProactiveRecall_NoMatch verifies that ProactiveRecall returns an empty
// list when no memories match the conversation context.
func TestProactiveRecall_NoMatch(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-6", UserID: "user-pr-6"}

	if err := svc.AddMemory(ctx, uk, "User likes tea", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"quantum-physics-unrelated-topic"},
		CurrentTopic:      "astronomy-unrelated-topic",
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for no match, got %d", len(entries))
	}
}

// TestProactiveRecall_NilDefense verifies that ProactiveRecall handles nil
// service receiver gracefully without panicking. In Go, calling a method on
// a nil interface panics, but a nil pointer wrapped in an interface is valid
// if the method handles nil receiver.
func TestProactiveRecall_NilDefense(t *testing.T) {
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-7", UserID: "user-pr-7"}

	// nil pointer to memoryService wrapped in the interface — the
	// method must handle nil receiver without panicking.
	var nilSvc trpcmemory.Service = (*memoryService)(nil)
	entries, err := nilSvc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"anything"},
	})
	if err != nil {
		t.Fatalf("nil service ProactiveRecall should not error: %v", err)
	}
	if entries != nil {
		t.Fatalf("nil service should return nil entries, got %v", entries)
	}
}

// TestProactiveRecall_FiltersInvalidated verifies that ProactiveRecall
// filters out invalidated memories (Bi-temporal P3-8 compatibility).
func TestProactiveRecall_FiltersInvalidated(t *testing.T) {
	svc, store := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-8", UserID: "user-pr-8"}

	if err := svc.AddMemory(ctx, uk, "User likes coffee", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if err := svc.AddMemory(ctx, uk, "User likes tea", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Invalidate the coffee memory.
	store.mu.Lock()
	for _, row := range store.facts {
		if stmt, _ := row["statement"].(string); stmt == "User likes coffee" {
			row["valid_until"] = "2026-01-01T00:00:00Z"
		}
	}
	store.mu.Unlock()

	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"coffee", "tea"},
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	for _, e := range entries {
		if e.Memory.Memory == "User likes coffee" {
			t.Fatal("expected invalidated memory to be filtered out")
		}
	}
}

// TestProactiveRecall_DeduplicatesAndRanks verifies that ProactiveRecall
// deduplicates entries when the same memory matches multiple signals.
func TestProactiveRecall_DeduplicatesAndRanks(t *testing.T) {
	svc, _ := newBitemporalService()
	ctx := context.Background()
	uk := trpcmemory.UserKey{AppName: "agent-pr-9", UserID: "user-pr-9"}

	if err := svc.AddMemory(ctx, uk, "User works at Google", nil); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Both entity mention and topic match the same memory.
	entries, err := svc.ProactiveRecall(ctx, uk, trpcmemory.ConversationContext{
		MentionedEntities: []string{"Google"},
		CurrentTopic:      "Google",
	})
	if err != nil {
		t.Fatalf("ProactiveRecall: %v", err)
	}
	// Should return only one entry (deduplicated).
	count := 0
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Memory.Memory), "google") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 deduplicated entry, got %d", count)
	}
}
