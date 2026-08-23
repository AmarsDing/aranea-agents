package main

import "testing"

func TestClassifyBackendPort(t *testing.T) {
	cases := []struct {
		name           string
		open, healthy  bool
		want           checkLevel
	}{
		{"free", false, false, checkOK},
		{"running backend", true, true, checkInfo},
		{"foreign occupant", true, false, checkWarn},
	}
	for _, c := range cases {
		if got, _ := classifyBackendPort(c.open, c.healthy); got != c.want {
			t.Fatalf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestClassifyBundledPGPort(t *testing.T) {
	if got, _ := classifyBundledPGPort(true); got != checkInfo {
		t.Fatal("open 5433 in bundled mode = already running (INFO)")
	}
	if got, _ := classifyBundledPGPort(false); got != checkOK {
		t.Fatal("closed 5433 = available (OK)")
	}
}
