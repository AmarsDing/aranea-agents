package measuregate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMeasure_Within(t *testing.T) {
	if got := Measure(context.Background(), 50*time.Millisecond, func(context.Context) error { return nil }); got != Within {
		t.Fatalf("expected within, got %v", got)
	}
}

func TestMeasure_Exceeded(t *testing.T) {
	got := Measure(context.Background(), 10*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})
	if got != Exceeded {
		t.Fatalf("expected exceeded, got %v", got)
	}
}

// fn 吞掉 ctxerr 但把 WrappedDeadline 传出来时，只要预算哨兵是终止原因仍判 Exceeded。
func TestMeasure_ExceededWrappedDeadline(t *testing.T) {
	got := Measure(context.Background(), 10*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return errors.Join(errors.New("probe failed"), ctx.Err())
	})
	if got != Exceeded {
		t.Fatalf("expected exceeded via cause, got %v", got)
	}
}

func TestMeasure_UnknownOnFnError(t *testing.T) {
	got := Measure(context.Background(), time.Second, func(context.Context) error {
		return errors.New("backend flaked")
	})
	if got != Unknown {
		t.Fatalf("fn error must be unknown, got %v", got)
	}
}

// 父上下文先取消：无法归责于本次预算 → Unknown（保守分支）。
func TestMeasure_UnknownWhenParentCancelled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	got := Measure(parent, time.Hour, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if got != Unknown {
		t.Fatalf("parent cancellation must be unknown, got %v", got)
	}
}

// 嵌套 Measure：外层预算耗尽不得被内层误计为 Exceeded（C10——哨兵按调用实例判身份）。
// 计时即被测机制本身；30ms 预算配 5s 兜底，最差只变慢不会误判。
func TestMeasure_NestedOuterBudgetStaysUnknown(t *testing.T) {
	inner := Unknown
	outer := Measure(context.Background(), 30*time.Millisecond, func(octx context.Context) error {
		inner = Measure(octx, time.Hour, func(ictx context.Context) error {
			<-ictx.Done()
			return ictx.Err()
		})
		return nil
	})
	if outer != Within {
		t.Fatalf("outer fn returned nil as budget fired: want Within, got %v", outer)
	}
	if inner != Unknown {
		t.Fatalf("outer budget must not count as inner's Exceeded: want Unknown, got %v", inner)
	}
}

// 父上下文自带超时先触发：同样不可归责于本次预算 → Unknown。
func TestMeasure_UnknownWhenParentDeadlineFires(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	got := Measure(parent, time.Hour, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if got != Unknown {
		t.Fatalf("parent deadline must be unknown, got %v", got)
	}
}

func TestMeasure_UnknownOnBadInput(t *testing.T) {
	if got := Measure(context.Background(), 0, func(context.Context) error { return nil }); got != Unknown {
		t.Fatalf("zero budget must be unknown, got %v", got)
	}
	if got := Measure(context.Background(), time.Second, nil); got != Unknown {
		t.Fatalf("nil fn must be unknown, got %v", got)
	}
}

func TestVerdict_String(t *testing.T) {
	if Within.String() != "within" || Exceeded.String() != "exceeded" || Unknown.String() != "unknown" {
		t.Fatalf("unexpected verdict strings: %v %v %v", Within, Exceeded, Unknown)
	}
}
