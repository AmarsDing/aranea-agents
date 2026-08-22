package codeexecutor

import (
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// fakeClock is a manually advanced clock for probe TTL tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// stubDockerProbeHooks replaces the docker probe command and clock, returning
// the probe invocation count and the fake clock for TTL advancement.
func stubDockerProbeHooks(t *testing.T, ok bool) (*int, *fakeClock) {
	t.Helper()
	count := new(int)
	clock := &fakeClock{t: time.Now()}
	origRun, origNow := dockerProbeRun, dockerNow
	dockerProbeRun = func() bool { *count++; return ok }
	dockerNow = clock.now
	t.Cleanup(func() {
		dockerProbeRun, dockerNow = origRun, origNow
		ResetDockerProbe()
	})
	ResetDockerProbe()
	return count, clock
}

func TestDockerAvailableCachesWithinTTL(t *testing.T) {
	count, clock := stubDockerProbeHooks(t, true)

	if !DockerAvailable() {
		t.Fatal("expected docker available")
	}
	if *count != 1 {
		t.Fatalf("expected 1 probe, got %d", *count)
	}
	clock.advance(dockerProbeTTL - time.Second)
	if !DockerAvailable() {
		t.Fatal("expected cached docker availability within TTL")
	}
	if *count != 1 {
		t.Fatalf("expected cached probe within TTL, got %d probes", *count)
	}
}

func TestDockerAvailableRefreshesAfterTTL(t *testing.T) {
	count, clock := stubDockerProbeHooks(t, true)

	DockerAvailable()
	clock.advance(dockerProbeTTL + time.Second)
	if !DockerAvailable() {
		t.Fatal("expected docker available after TTL refresh")
	}
	if *count != 2 {
		t.Fatalf("expected re-probe after TTL, got %d probes", *count)
	}
}

// stubLocalProbeHooks replaces the local interpreter probe with a runnable
// set keyed by binary name.
func stubLocalProbeHooks(t *testing.T, runnable map[string]bool) {
	t.Helper()
	origRun := localProbeRun
	localProbeRun = func(name string) bool { return runnable[name] }
	t.Cleanup(func() {
		localProbeRun = origRun
		ResetLocalProbe()
	})
	ResetLocalProbe()
}

func findCapability(caps []Capability, typ string) Capability {
	for _, c := range caps {
		if c.Type == typ {
			return c
		}
	}
	return Capability{Type: typ}
}

func TestLocalCapabilityUnavailableWhenNoInterpreter(t *testing.T) {
	stubLocalProbeHooks(t, map[string]bool{})

	f := NewFactoryWithLogger(loggateway.NewNoop())
	cap := findCapability(f.Capabilities(), TypeLocal)
	if cap.Available {
		t.Fatal("expected local unavailable when no python3/bash runs")
	}
	if cap.Reason == "" {
		t.Fatal("expected non-empty reason for unavailable local")
	}
}

func TestLocalCapabilityAvailableWhenInterpreterRuns(t *testing.T) {
	stubLocalProbeHooks(t, map[string]bool{"python3": true})

	f := NewFactoryWithLogger(loggateway.NewNoop())
	cap := findCapability(f.Capabilities(), TypeLocal)
	if !cap.Available {
		t.Fatalf("expected local available when python3 runs, got reason %q", cap.Reason)
	}
}

func TestRegisteredTypesReflectsLocalProbe(t *testing.T) {
	f := NewFactoryWithLogger(loggateway.NewNoop())

	stubLocalProbeHooks(t, map[string]bool{"bash": true})
	found := false
	for _, typ := range f.RegisteredTypes() {
		if typ == TypeLocal {
			found = true
		}
	}
	if !found {
		t.Fatal("expected local in RegisteredTypes when an interpreter runs")
	}

	ResetLocalProbe()
	localProbeRun = func(string) bool { return false }
	for _, typ := range f.RegisteredTypes() {
		if typ == TypeLocal {
			t.Fatal("expected local excluded from RegisteredTypes when no interpreter runs")
		}
	}
}
