package data

import "testing"

// The client bridge tools are seeded disabled (opt-in) with confirmation
// required, so the tool_confirm_gate catalog gates client_open_app /
// client_open_url for every agent that enables them (74-voice-companion §6.1).
func TestBuiltinPlatformToolSeeds_ClientBridge(t *testing.T) {
	byKey := map[string]platformToolSeed{}
	for _, s := range builtinPlatformToolSeeds {
		byKey[s.key] = s
	}

	app, ok := byKey["client_open_app"]
	if !ok {
		t.Fatal("seed client_open_app missing")
	}
	if app.reqConfirm != true {
		t.Error("client_open_app must require confirmation")
	}
	if app.enabled {
		t.Error("client_open_app must be opt-in (enabled=false)")
	}
	if app.registryName != "client" {
		t.Errorf("client_open_app registryName = %q, want client", app.registryName)
	}
	if app.riskLevel != "medium" {
		t.Errorf("client_open_app riskLevel = %q, want medium", app.riskLevel)
	}

	url, ok := byKey["client_open_url"]
	if !ok {
		t.Fatal("seed client_open_url missing")
	}
	if url.reqConfirm != true {
		t.Error("client_open_url must require confirmation")
	}
	if url.enabled {
		t.Error("client_open_url must be opt-in (enabled=false)")
	}
	if url.registryName != "client" {
		t.Errorf("client_open_url registryName = %q, want client", url.registryName)
	}
	if url.riskLevel != "low" {
		t.Errorf("client_open_url riskLevel = %q, want low", url.riskLevel)
	}
}

// TestDDLMigrations_BuiltinToolsClientReseed guards the M74 V2-T8 gap-1 fix:
// migration 20260610 (builtin_platform_tools) was applied on existing
// databases BEFORE client_open_app/client_open_url were added to the seed
// list; the schema_migrations gate then skips 20260610 forever, so those DBs
// permanently lack the client bridge tools (voice「打开微信」degrades to
// "client offline" because the tool row is missing entirely). The reseed
// migration must stay registered — the seed is idempotent (ON CONFLICT
// DO NOTHING + catalog UPDATEs), so re-running is safe.
func TestDDLMigrations_BuiltinToolsClientReseed(t *testing.T) {
	const wantVersion = 20261202
	for _, m := range ddlMigrations {
		if m.Version == wantVersion {
			if m.Func == nil {
				t.Fatalf("migration %d (%s) must carry the reseed Func", wantVersion, m.Name)
			}
			return
		}
	}
	t.Fatalf("migration %d (builtin_platform_tools_client_reseed) not registered", wantVersion)
}
