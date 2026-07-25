package biz

import "testing"

func TestCriticLoopCondFuncRefForThreshold(t *testing.T) {
	if got := CriticLoopCondFuncRefForThreshold(0); got != CriticLoopCondFuncRef {
		t.Fatalf("threshold 0: got %q want %q", got, CriticLoopCondFuncRef)
	}
	got := CriticLoopCondFuncRefForThreshold(0.8)
	want := CriticLoopCondFuncRef + "@0.8"
	if got != want {
		t.Fatalf("threshold 0.8: got %q want %q", got, want)
	}
}

func TestCriticLoopCondFuncRefForConfig(t *testing.T) {
	cases := []struct {
		threshold     float64
		maxIterations int
		want          string
	}{
		{0, 0, CriticLoopCondFuncRef},
		{0.8, 0, CriticLoopCondFuncRef + "@0.8"},
		{0, 3, CriticLoopCondFuncRef + "#3"},
		{0.8, 5, CriticLoopCondFuncRef + "@0.8#5"},
	}
	for _, c := range cases {
		if got := CriticLoopCondFuncRefForConfig(c.threshold, c.maxIterations); got != c.want {
			t.Fatalf("ForConfig(%v,%d): got %q want %q", c.threshold, c.maxIterations, got, c.want)
		}
	}
}

func TestCriticLoopCondFuncRefForNode(t *testing.T) {
	cases := []struct {
		threshold     float64
		maxIterations int
		nodeID        string
		want          string
	}{
		// nodeID 为空时退化为 ForConfig（裸 ref / 图域 LLM 节点路径）。
		{0, 0, "", CriticLoopCondFuncRef},
		{0.8, 5, "", CriticLoopCondFuncRef + "@0.8#5"},
		// team 编译路径：nodeID 编码进 ref，cond func 按节点隔离轮次。
		{0, 3, "member-2", CriticLoopCondFuncRef + "#3%member-2"},
		{0.8, 5, "member-2", CriticLoopCondFuncRef + "@0.8#5%member-2"},
	}
	for _, c := range cases {
		if got := CriticLoopCondFuncRefForNode(c.threshold, c.maxIterations, c.nodeID); got != c.want {
			t.Fatalf("ForNode(%v,%d,%q): got %q want %q", c.threshold, c.maxIterations, c.nodeID, got, c.want)
		}
	}
}

func TestCriticLoopMetaKeysForNode(t *testing.T) {
	// 空 nodeID：返回裸 key（向后兼容旧 checkpoint / 裸 ref 路径）。
	r, l, p := CriticLoopMetaKeysForNode("")
	if r != CriticLoopRoundsMetaKey || l != CriticLoopLastResponseMetaKey || p != CriticLoopPrevResponseMetaKey {
		t.Fatalf("empty nodeID: got (%q,%q,%q)", r, l, p)
	}
	// 非空 nodeID：scoped key = bare + "/" + nodeID，多 critic 图互不覆盖。
	r, l, p = CriticLoopMetaKeysForNode("member-2")
	if r != "critic_loop_rounds/member-2" {
		t.Fatalf("rounds key: got %q", r)
	}
	if l != "critic_loop_last_response/member-2" {
		t.Fatalf("last key: got %q", l)
	}
	if p != "critic_loop_prev_response/member-2" {
		t.Fatalf("prev key: got %q", p)
	}
}

func TestParseCriticLoopCondFuncRef(t *testing.T) {
	cases := []struct {
		ref           string
		wantThreshold float64
		wantMaxIter   int
		wantNodeID    string
		wantOK        bool
	}{
		{CriticLoopCondFuncRef, 0, 0, "", true},
		{CriticLoopCondFuncRef + "@0.8", 0.8, 0, "", true},
		{CriticLoopCondFuncRef + "#3", 0, 3, "", true},
		{CriticLoopCondFuncRef + "@0.8#5", 0.8, 5, "", true},
		// 节点维度 ref（team 编译路径，多 critic 图隔离轮次）。
		{CriticLoopCondFuncRef + "#3%member-2", 0, 3, "member-2", true},
		{CriticLoopCondFuncRef + "@0.8#5%member-2", 0.8, 5, "member-2", true},
		{CriticLoopCondFuncRef + "%member-2", 0, 0, "member-2", true},
		{"other_func", 0, 0, "", false},
		{CriticLoopCondFuncRef + "@abc", 0, 0, "", false},
		{CriticLoopCondFuncRef + "#x", 0, 0, "", false},
		{CriticLoopCondFuncRef + "%", 0, 0, "", false},                      // % 后空 nodeID = 畸形
		{CriticLoopCondFuncRef + "#3%", 0, 0, "", false},                    // 同上
		{CriticLoopCondFuncRef + "@0.8%member-2", 0.8, 0, "member-2", true}, // @/% 间 # 可省略
	}
	for _, c := range cases {
		threshold, maxIter, nodeID, ok := ParseCriticLoopCondFuncRef(c.ref)
		if ok != c.wantOK || threshold != c.wantThreshold || maxIter != c.wantMaxIter || nodeID != c.wantNodeID {
			t.Fatalf("Parse(%q): got (%v,%d,%q,%v) want (%v,%d,%q,%v)",
				c.ref, threshold, maxIter, nodeID, ok, c.wantThreshold, c.wantMaxIter, c.wantNodeID, c.wantOK)
		}
	}
}

func TestNodeCircuitBreakerRegistry_OpensAfterThreshold(t *testing.T) {
	reg := NewNodeCircuitBreakerRegistry()
	pol := &CircuitBreakerPolicy{FailureThreshold: 2, ResetTimeoutSeconds: 60}
	cb := reg.ForNode("team:t1", "member-1", pol)
	if cb == nil {
		t.Fatal("expected breaker")
	}
	cb.RecordFailure()
	allowed, _ := cb.Allow()
	if !allowed {
		t.Fatal("should allow after 1 failure")
	}
	cb.RecordFailure()
	allowed, st := cb.Allow()
	if allowed || string(st) != "open" {
		t.Fatalf("expected open after threshold, allowed=%v state=%s", allowed, st)
	}
	msg := CircuitOpenErrorMessage("member-1", string(st))
	if !IsCircuitOpenError(errString(msg)) {
		t.Fatal("IsCircuitOpenError should match message")
	}
	if IsCircuitOpenError(nil) {
		t.Fatal("nil should not be circuit open")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
