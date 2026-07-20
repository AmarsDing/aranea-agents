package session

import (
	"testing"
	"time"
)

func TestCompressSuppressManager(t *testing.T) {
	now := time.Now()
	m := newCompressSuppressManager()

	// 无记录 → 不抑制
	if suppressed, _ := m.check("s1", "openai/gpt-4o", 10*time.Minute, now); suppressed {
		t.Fatal("无记录不应抑制")
	}

	// 确定性失败 → 同模型抑制，不同模型放行
	m.record("s1", compressFailureDeterministic, "openai/gpt-4o", now)
	if suppressed, _ := m.check("s1", "openai/gpt-4o", 10*time.Minute, now.Add(time.Hour)); !suppressed {
		t.Fatal("确定性失败应 sticky 抑制（不受 minGap 影响）")
	}
	if suppressed, _ := m.check("s1", "anthropic/claude", 10*time.Minute, now); suppressed {
		t.Fatal("模型切换应解除抑制")
	}

	// 瞬态失败 → minGap 内抑制，过后放行
	m.record("s2", compressFailureTransient, "openai/gpt-4o", now)
	if suppressed, _ := m.check("s2", "openai/gpt-4o", 10*time.Minute, now.Add(5*time.Minute)); !suppressed {
		t.Fatal("瞬态失败 minGap 内应抑制")
	}
	if suppressed, _ := m.check("s2", "openai/gpt-4o", 10*time.Minute, now.Add(11*time.Minute)); suppressed {
		t.Fatal("瞬态失败超过 minGap 应放行")
	}

	// clear 解除
	m.record("s3", compressFailureDeterministic, "openai/gpt-4o", now)
	m.clear("s3")
	if suppressed, _ := m.check("s3", "openai/gpt-4o", 10*time.Minute, now); suppressed {
		t.Fatal("clear 后不应抑制")
	}

	// record 边界：none kind 与空 sessionID 不记录
	m.record("", compressFailureDeterministic, "openai/gpt-4o", now)
	m.record("s4", compressFailureNone, "openai/gpt-4o", now)
	if suppressed, _ := m.check("", "openai/gpt-4o", 10*time.Minute, now); suppressed {
		t.Fatal("空 sessionID 不应记录")
	}
	if suppressed, _ := m.check("s4", "openai/gpt-4o", 10*time.Minute, now); suppressed {
		t.Fatal("none kind 不应记录")
	}
}
