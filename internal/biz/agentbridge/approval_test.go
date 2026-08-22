package agentbridge

import "testing"

func TestResolvePermissionOption(t *testing.T) {
	opts := []PermissionOption{
		{OptionID: "allow", Name: "允许", Kind: "allow_once"},
		{OptionID: "always", Name: "始终允许", Kind: "allow_always"},
		{OptionID: "deny", Name: "拒绝", Kind: "reject_once"},
	}
	id, remember, err := ResolvePermissionOption(opts, DecisionApprove, false)
	if err != nil || id != "allow" || remember {
		t.Fatalf("approve: id=%s remember=%v err=%v", id, remember, err)
	}
	id, remember, err = ResolvePermissionOption(opts, DecisionAlways, false)
	if err != nil || id != "always" || !remember {
		t.Fatalf("always: id=%s remember=%v err=%v", id, remember, err)
	}
	id, remember, err = ResolvePermissionOption(opts, DecisionDeny, false)
	if err != nil || id != "deny" || remember {
		t.Fatalf("deny: id=%s remember=%v err=%v", id, remember, err)
	}
	id, remember, err = ResolvePermissionOption(opts, DecisionApprove, true)
	if err != nil || id != "always" || !remember {
		t.Fatalf("approve with cache should pick allow_always: id=%s remember=%v err=%v", id, remember, err)
	}
}

func TestResolvePermissionOption_EmptyAndUnknown(t *testing.T) {
	if _, _, err := ResolvePermissionOption(nil, DecisionApprove, false); err == nil {
		t.Fatal("empty opts must fail")
	}
	opts := []PermissionOption{{OptionID: "x", Kind: "allow_once"}}
	if _, _, err := ResolvePermissionOption(opts, "maybe", false); err == nil {
		t.Fatal("unknown decision must fail")
	}
}
