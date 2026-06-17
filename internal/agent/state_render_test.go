package agent

import (
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestRenderStateTemplate_EmptyTemplate(t *testing.T) {
	result, err := RenderStateTemplate("", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRenderStateTemplate_NoPlaceholders(t *testing.T) {
	result, err := RenderStateTemplate("hello world", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", result)
	}
}

func TestRenderStateTemplate_InvocationState(t *testing.T) {
	inv := &trpcagent.Invocation{}
	inv.SetState("capital_city", "Paris")

	result, err := RenderStateTemplate(
		"Tell me about the city stored in {invocation:capital_city}.",
		inv, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Tell me about the city stored in Paris."
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_SessionState(t *testing.T) {
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("greeting", []byte(`"hello"`))

	result, err := RenderStateTemplate(
		"The greeting is {greeting}.",
		nil, sess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "The greeting is hello."
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_SessionStateNamespaced(t *testing.T) {
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState(trpcsession.StateAppPrefix+"config", []byte(`"enabled"`))
	sess.SetState(trpcsession.StateUserPrefix+"pref", []byte(`"dark"`))
	sess.SetState(trpcsession.StateTempPrefix+"cache", []byte(`"hit"`))

	result, err := RenderStateTemplate(
		"app={app:config} user={user:pref} temp={temp:cache}",
		nil, sess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "app=enabled user=dark temp=hit"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_OptionalPlaceholder(t *testing.T) {
	result, err := RenderStateTemplate(
		"Hello {missing_name?}!",
		nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello !"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_NonOptionalPlaceholderPreserved(t *testing.T) {
	result, err := RenderStateTemplate(
		"Hello {missing_name}!",
		nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello {missing_name}!"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_InvocationFallsBackToSession(t *testing.T) {
	inv := &trpcagent.Invocation{}
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("shared_key", []byte(`"from_session"`))
	inv.Session = sess

	// Non-invocation: prefix resolves from session.
	result, err := RenderStateTemplate(
		"{shared_key}",
		inv, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "from_session"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_InvocationPrefixIgnoresSession(t *testing.T) {
	inv := &trpcagent.Invocation{}
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("invocation:key", []byte(`"from_session"`))
	inv.Session = sess

	// {invocation:key} should only look at invocation state, not session.
	result, err := RenderStateTemplate(
		"{invocation:key}",
		inv, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unresolved non-optional stays literal.
	want := "{invocation:key}"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_NumericStateValue(t *testing.T) {
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("count", []byte(`42`))

	result, err := RenderStateTemplate(
		"Count: {count}",
		nil, sess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Count: 42"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_ObjectStateValue(t *testing.T) {
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("config", []byte(`{"theme":"dark","lang":"en"}`))

	result, err := RenderStateTemplate(
		"Config: {config}",
		nil, sess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `Config: {"theme":"dark","lang":"en"}`
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_DoubleBraceSyntax(t *testing.T) {
	sess := trpcsession.NewSession("test-app", "test-user", "test-session")
	sess.SetState("name", []byte(`"World"`))

	result, err := RenderStateTemplate(
		"Hello {{name}}!",
		nil, sess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello World!"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_NilInvocationAndSession(t *testing.T) {
	result, err := RenderStateTemplate(
		"Hello {invocation:key} and {app:val}!",
		nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All unresolved, non-optional preserved.
	want := "Hello {invocation:key} and {app:val}!"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_SessionOverridesInvocationSession(t *testing.T) {
	inv := &trpcagent.Invocation{}
	invSession := trpcsession.NewSession("test-app", "test-user", "test-session")
	invSession.SetState("key", []byte(`"from_inv_session"`))
	inv.Session = invSession

	explicitSess := trpcsession.NewSession("test-app", "test-user", "test-session")
	explicitSess.SetState("key", []byte(`"from_explicit_session"`))

	result, err := RenderStateTemplate(
		"{key}",
		inv, explicitSess,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Explicit session takes precedence over invocation.Session.
	want := "from_explicit_session"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_InvocationStateNilValue(t *testing.T) {
	inv := &trpcagent.Invocation{}
	inv.SetState("nil_key", nil)

	result, err := RenderStateTemplate(
		"{invocation:nil_key}",
		inv, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil value treated as not found, non-optional preserved.
	want := "{invocation:nil_key}"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}

func TestRenderStateTemplate_InvocationStateNonString(t *testing.T) {
	inv := &trpcagent.Invocation{}
	inv.SetState("count", 42)

	result, err := RenderStateTemplate(
		"Count: {invocation:count}",
		inv, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Count: 42"
	if result != want {
		t.Errorf("expected %q, got %q", want, result)
	}
}
