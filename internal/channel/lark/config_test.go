package lark

import "testing"

func TestAppAndRegionFromConfig(t *testing.T) {
	region, appID, err := AppAndRegionFromConfig(`{"config":{"app_id":"cli_x","region":"lark"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if appID != "cli_x" || region != "lark" {
		t.Fatalf("got region=%q appID=%q", region, appID)
	}
	_, _, err = AppAndRegionFromConfig(`{"config":{}}`)
	if err == nil {
		t.Fatal("expected missing app_id error")
	}
}
