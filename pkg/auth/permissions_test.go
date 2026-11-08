package auth

import "testing"

func TestHasPermissionSuperadminWildcard(t *testing.T) {
	a := &Auth{Roles: []string{"superadmin"}}
	for _, code := range []string{"knowledge.read", "system.setting", "user.clearance", "anything.at.all"} {
		if !a.HasPermission(code) {
			t.Fatalf("superadmin should hold %q", code)
		}
	}
}

func TestHasPermissionPrefixWildcard(t *testing.T) {
	a := &Auth{Roles: []string{"sysadmin"}}
	if !a.HasPermission("space.read") || !a.HasPermission("space.write") {
		t.Fatal("sysadmin space.* should cover space.read/write")
	}
	if a.HasPermission("user.clearance") {
		t.Fatal("sysadmin must NOT hold user.clearance (secadmin only)")
	}
	if !a.HasPermission("user.read") {
		t.Fatal("sysadmin should hold user.read")
	}
}

func TestHasPermissionExactAndEmpty(t *testing.T) {
	a := &Auth{Roles: []string{"auditor"}}
	if !a.HasPermission("audit.read") {
		t.Fatal("auditor should hold audit.read")
	}
	if a.HasPermission("audit.write") {
		t.Fatal("auditor must be read-only")
	}
	if a.HasPermission("") {
		t.Fatal("empty code must not match")
	}
	var nilAuth *Auth
	if nilAuth.HasPermission("audit.read") {
		t.Fatal("nil principal must hold nothing")
	}
}

func TestHasPermissionUnknownRole(t *testing.T) {
	a := &Auth{Roles: []string{"no-such-role"}}
	if a.HasPermission("knowledge.read") {
		t.Fatal("unknown role must grant nothing")
	}
}

func TestSetPermissionResolverNilRestoresBuiltin(t *testing.T) {
	SetPermissionResolver(testResolver{grants: map[string]struct{}{"custom.code": {}}})
	a := &Auth{Roles: []string{"whatever"}}
	if !a.HasPermission("custom.code") {
		t.Fatal("custom resolver should be consulted")
	}
	SetPermissionResolver(nil)
	if a.HasPermission("custom.code") {
		t.Fatal("nil resolver must restore builtin static mapping")
	}
}

type testResolver struct {
	grants map[string]struct{}
}

func (r testResolver) PermissionsForRoles([]string) map[string]struct{} { return r.grants }

func TestLegacyRolesForAccess(t *testing.T) {
	if got := LegacyRolesForAccess("admin"); len(got) != 1 || got[0] != "superadmin" {
		t.Fatalf("access=admin → %v, want [superadmin]", got)
	}
	if got := LegacyRolesForAccess("user"); len(got) != 1 || got[0] != "user" {
		t.Fatalf("access=user → %v, want [user]", got)
	}
	if got := LegacyRolesForAccess(""); len(got) != 1 || got[0] != "user" {
		t.Fatalf("access empty → %v, want [user]", got)
	}
}

func TestHasRoleAndAnyRole(t *testing.T) {
	a := &Auth{Roles: []string{"ceo", "auditor"}}
	if !a.HasRole("ceo") || a.HasRole("secadmin") {
		t.Fatal("HasRole mismatch")
	}
	if !a.HasAnyRole("secadmin", "auditor") {
		t.Fatal("HasAnyRole should match auditor")
	}
	if a.HasAnyRole("secadmin", "sysadmin") {
		t.Fatal("HasAnyRole should not match")
	}
}

func TestClearanceLevelBounds(t *testing.T) {
	if got := (&Auth{}).ClearanceLevel(); got != 1 {
		t.Fatalf("zero clearance → L1, got %d", got)
	}
	if got := (&Auth{Clearance: 9}).ClearanceLevel(); got != 5 {
		t.Fatalf("clearance clamped to L5, got %d", got)
	}
	if !(&Auth{Clearance: 3}).HasClearance(2) || (&Auth{Clearance: 3}).HasClearance(4) {
		t.Fatal("HasClearance comparison wrong")
	}
	var nilAuth *Auth
	if got := nilAuth.ClearanceLevel(); got != 1 {
		t.Fatalf("nil principal → L1, got %d", got)
	}
}

func TestDevBypassPrincipalClaims(t *testing.T) {
	p := DevBypassPrincipal()
	if !p.HasRole("superadmin") || !p.HasPermission("user.clearance") || !p.HasClearance(5) {
		t.Fatal("bypass principal must be superadmin/L5 so gates pass in dev")
	}
}
