package biz

import (
	"testing"
)

func TestChannelConcurrentGate_TryAcquireRelease(t *testing.T) {
	g := NewChannelConcurrentGate()
	defer g.Close()

	if _, ok := g.TryAcquire("ch1", "peer1", false, 2); !ok {
		t.Fatal("first acquire should succeed")
	}
	if _, ok := g.TryAcquire("ch1", "peer1", false, 2); !ok {
		t.Fatal("second acquire should succeed (limit=2)")
	}
	if _, ok := g.TryAcquire("ch1", "peer1", false, 2); ok {
		t.Fatal("third acquire should fail (limit=2)")
	}

	release, ok := g.TryAcquire("ch1", "peer1", false, 2)
	_ = release
	_ = ok
	// Use the release function from TryAcquire
	g2 := NewChannelConcurrentGate()
	defer g2.Close()
	if rel, ok2 := g2.TryAcquire("ch1", "peer1", false, 2); !ok2 {
		t.Fatal("first acquire should succeed")
	} else {
		rel()
	}
	if _, ok2 := g2.TryAcquire("ch1", "peer1", false, 2); !ok2 {
		t.Fatal("acquire after release should succeed")
	}
}

func TestChannelConcurrentGate_NilReceiver(t *testing.T) {
	var g *ChannelConcurrentGate
	if _, ok := g.TryAcquire("ch1", "peer1", false, 1); !ok {
		t.Fatal("nil gate should always allow")
	}
}

func TestChannelConcurrentGate_ZeroLimit(t *testing.T) {
	g := NewChannelConcurrentGate()
	defer g.Close()
	if _, ok := g.TryAcquire("ch1", "peer1", false, 0); !ok {
		t.Fatal("zero limit should always allow")
	}
	if _, ok := g.TryAcquire("ch1", "peer1", false, -1); !ok {
		t.Fatal("negative limit should always allow")
	}
}

func TestChannelConcurrentGate_DifferentPeers(t *testing.T) {
	g := NewChannelConcurrentGate()
	defer g.Close()
	if _, ok := g.TryAcquire("ch1", "peer1", false, 1); !ok {
		t.Fatal("peer1 should acquire")
	}
	if _, ok := g.TryAcquire("ch1", "peer2", false, 1); !ok {
		t.Fatal("peer2 should acquire independently")
	}
}

func TestChannelConcurrentGate_ReleaseNonExistent(t *testing.T) {
	g := NewChannelConcurrentGate()
	defer g.Close()
	g.release("nonexistent", "peer", false)
}
