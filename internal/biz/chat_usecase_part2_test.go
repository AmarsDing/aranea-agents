package biz

import (
	"testing"
)

// Re-registering the same tool scope replaces the stale entry (interrupt
// semantics) while sibling scopes stay fully functional.
func TestChatUsecase_AwaitChannelForTool_ReRegisterSameScopeKeepsSiblings(t *testing.T) {
	t.Parallel()
	uc := newChatUsecaseForTest(&stubChatRunGateway{}, &stubChatPersister{}, &stubChatEventPublisher{})

	stale := make(AwaitChannel, 1)
	uc.RegisterAwaitChannelForTool("sess-1", "tc-1", stale)
	fresh := make(AwaitChannel, 1)
	uc.RegisterAwaitChannelForTool("sess-1", "tc-1", fresh)
	sibling := make(AwaitChannel, 1)
	uc.RegisterAwaitChannelForTool("sess-1", "tc-2", sibling)

	if got, ok := uc.LoadAwaitChannelForTool("sess-1", "tc-1"); !ok || got != fresh {
		t.Fatal("re-registration must replace the scoped channel")
	}

	if !uc.TrySendAwaitChannelForTool("sess-1", "tc-1", AwaitReplyMsg{Reply: "approved"}) {
		t.Fatal("send to re-registered scope rejected")
	}
	select {
	case <-fresh:
	default:
		t.Fatal("fresh channel did not receive the reply")
	}
	select {
	case <-stale:
		t.Fatal("stale channel must not receive replies after re-registration")
	default:
	}

	if !uc.TrySendAwaitChannelForTool("sess-1", "tc-2", AwaitReplyMsg{Reply: "approved"}) {
		t.Fatal("sibling scope must survive a same-scope re-registration")
	}
	select {
	case <-sibling:
	default:
		t.Fatal("sibling scope did not receive its reply")
	}
}

// A scoped lookup/send with no matching tool entry falls back to the
// session-level slot (legacy registration + scoped delivery interop: the
// awaiting run registered before this change only holds the session slot).
func TestChatUsecase_AwaitChannelForTool_FallsBackToSessionSlot(t *testing.T) {
	t.Parallel()
	uc := newChatUsecaseForTest(&stubChatRunGateway{}, &stubChatPersister{}, &stubChatEventPublisher{})

	sessionCh := make(AwaitChannel, 1)
	uc.RegisterAwaitChannel("sess-1", sessionCh)

	if got, ok := uc.LoadAwaitChannelForTool("sess-1", "tc-gone"); !ok || got != sessionCh {
		t.Fatal("scoped load must fall back to the session-level slot")
	}
	if !uc.TrySendAwaitChannelForTool("sess-1", "tc-gone", AwaitReplyMsg{Reply: "approved", ToolCallID: "tc-gone"}) {
		t.Fatal("scoped send must fall back to the session-level slot")
	}
	select {
	case msg := <-sessionCh:
		if msg.Reply != "approved" {
			t.Fatalf("session slot received wrong reply %q", msg.Reply)
		}
	default:
		t.Fatal("session-level slot did not receive the fallback reply")
	}
}

// When the scoped key exists, the session-level slot of the same session is
// bypassed — a tool decision must never land in a free-text await slot.
func TestChatUsecase_AwaitChannelForTool_ScopedKeyBeatsSessionSlot(t *testing.T) {
	t.Parallel()
	uc := newChatUsecaseForTest(&stubChatRunGateway{}, &stubChatPersister{}, &stubChatEventPublisher{})

	sessionCh := make(AwaitChannel, 1)
	uc.RegisterAwaitChannel("sess-1", sessionCh)
	scopedCh := make(AwaitChannel, 1)
	uc.RegisterAwaitChannelForTool("sess-1", "tc-1", scopedCh)

	if !uc.TrySendAwaitChannelForTool("sess-1", "tc-1", AwaitReplyMsg{Reply: "approved"}) {
		t.Fatal("scoped send rejected")
	}
	select {
	case <-scopedCh:
	default:
		t.Fatal("scoped channel must win over the session-level slot")
	}
	select {
	case <-sessionCh:
		t.Fatal("session-level slot must not receive a scoped reply")
	default:
	}
}

// Terminal run status sweeps the session-level slot AND every tool-scoped
// entry, so parallel-confirmation leftovers cannot leak until GC; other
// sessions are untouched.
func TestChatUsecase_DeleteAwaitChannelsForSession_SweepsAllScopes(t *testing.T) {
	t.Parallel()
	uc := newChatUsecaseForTest(&stubChatRunGateway{}, &stubChatPersister{}, &stubChatEventPublisher{})

	uc.RegisterAwaitChannel("sess-1", make(AwaitChannel, 1))
	uc.RegisterAwaitChannelForTool("sess-1", "tc-1", make(AwaitChannel, 1))
	uc.RegisterAwaitChannelForTool("sess-1", "tc-2", make(AwaitChannel, 1))
	other := make(AwaitChannel, 1)
	uc.RegisterAwaitChannelForTool("sess-2", "tc-1", other)

	uc.DeleteAwaitChannelsForSession("sess-1")

	if _, ok := uc.LoadAwaitChannel("sess-1"); ok {
		t.Fatal("session-level slot must be swept")
	}
	for _, id := range []string{"tc-1", "tc-2"} {
		if _, ok := uc.LoadAwaitChannelForTool("sess-1", id); ok {
			t.Fatalf("tool scope %q must be swept", id)
		}
	}
	if got, ok := uc.LoadAwaitChannelForTool("sess-2", "tc-1"); !ok || got != other {
		t.Fatal("other sessions' scopes must be untouched")
	}
}
