package service

import "testing"

func TestGateHysteresis_FirstTurnAdoptsRaw(t *testing.T) {
	t.Parallel()
	s := newGateHysteresisStore()
	// S06/S07 单轮组队场景：首轮 force 必须立即生效，不得被滞回延迟。
	if gear, damped := s.Apply("s1", true); !gear || damped {
		t.Fatalf("first turn must adopt raw=true, got gear=%v damped=%v", gear, damped)
	}
	s2 := newGateHysteresisStore()
	if gear, damped := s2.Apply("s1", false); gear || damped {
		t.Fatalf("first turn must adopt raw=false, got gear=%v damped=%v", gear, damped)
	}
}

// R4-Q9 原序列：simple→moderate(force)→simple→simple→simple。
// moderate 单轮尖峰不得切档。
func TestGateHysteresis_S11OscillationDamped(t *testing.T) {
	t.Parallel()
	s := newGateHysteresisStore()
	seq := []struct {
		raw        bool
		wantGear   bool
		wantDamped bool
	}{
		{false, false, false}, // t1 simple 直采
		{true, false, true},   // t2 force 尖峰：压住
		{false, false, false}, // t3 回落：计数归零
		{false, false, false},
		{false, false, false},
	}
	for i, tt := range seq {
		gear, damped := s.Apply("s11", tt.raw)
		if gear != tt.wantGear || damped != tt.wantDamped {
			t.Fatalf("turn %d: raw=%v got gear=%v damped=%v, want gear=%v damped=%v",
				i+1, tt.raw, gear, damped, tt.wantGear, tt.wantDamped)
		}
	}
}

// 真实持续升级：连续 2 轮 force 才切档（迟滞一轮），切档后单轮回落不降级。
func TestGateHysteresis_SustainedEscalationSwitches(t *testing.T) {
	t.Parallel()
	s := newGateHysteresisStore()
	s.Apply("s2", false) // 定档 simple
	if _, damped := s.Apply("s2", true); !damped {
		t.Fatal("first opposite crossing must be damped")
	}
	gear, damped := s.Apply("s2", true)
	if !gear || damped {
		t.Fatalf("second consecutive crossing must switch gear, got gear=%v damped=%v", gear, damped)
	}
	// 切到 force 后单轮 simple 不降级。
	if gear, damped := s.Apply("s2", false); !gear || !damped {
		t.Fatalf("single relaxation after switch must be damped, got gear=%v damped=%v", gear, damped)
	}
	// 连续 2 轮 simple 才降档。
	if gear, _ := s.Apply("s2", false); gear {
		t.Fatal("second consecutive relaxation must switch gear off")
	}
}

// 方向反复（force/simple 交替）永不切档。
func TestGateHysteresis_AlternatingNeverSwitches(t *testing.T) {
	t.Parallel()
	s := newGateHysteresisStore()
	s.Apply("s3", false)
	for i := 0; i < 6; i++ {
		raw := i%2 == 0 // true,false,true,...
		gear, _ := s.Apply("s3", raw)
		if gear {
			t.Fatalf("alternating turn %d must never switch to force", i)
		}
	}
}

func TestGateHysteresis_NilStorePassThrough(t *testing.T) {
	t.Parallel()
	var s *gateHysteresisStore
	if gear, damped := s.Apply("s4", true); !gear || damped {
		t.Fatal("nil store must pass raw through")
	}
	if gear, damped := s.Apply("", false); gear || damped {
		t.Fatal("empty session must pass raw through")
	}
}

func TestGateHysteresis_Eviction(t *testing.T) {
	t.Parallel()
	s := newGateHysteresisStore()
	for i := 0; i < gateHysteresisMaxSessions+8; i++ {
		s.Apply(string(rune('a'+i%26))+string(rune('0'+i%10))+string(rune(rune(i))), true)
	}
	if len(s.sessions) > gateHysteresisMaxSessions {
		t.Fatalf("sessions = %d, want <= %d", len(s.sessions), gateHysteresisMaxSessions)
	}
}
