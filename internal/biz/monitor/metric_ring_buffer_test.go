package monitor_test

import (
	"testing"

	"aranea-agents/internal/biz/monitor"
)

func TestNewMetricRingBuffer(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	if rb == nil {
		t.Fatal("NewMetricRingBuffer() = nil, want non-nil")
	}
}

func TestMetricRingBuffer_IncTotal(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	rb.IncTotal("runner.completion")
	rb.IncTotal("runner.completion")

	wr := rb.SumLastN(1)
	if wr.Total != 3 {
		t.Errorf("SumLastN(1).Total = %d, want 3", wr.Total)
	}
}

func TestMetricRingBuffer_IncError(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	rb.IncTotal("runner.completion")
	rb.IncError("runner.completion")

	wr := rb.SumLastN(1)
	if wr.Total != 2 {
		t.Errorf("SumLastN(1).Total = %d, want 2", wr.Total)
	}
	if wr.Errors != 1 {
		t.Errorf("SumLastN(1).Errors = %d, want 1", wr.Errors)
	}
}

func TestMetricRingBuffer_AddDuration(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.AddDuration("runner.completion", 100)
	rb.AddDuration("runner.completion", 200)
	rb.AddDuration("runner.completion", 300)

	wr := rb.SumLastN(1)
	if wr.AvgMs != 200 {
		t.Errorf("SumLastN(1).AvgMs = %.2f, want 200", wr.AvgMs)
	}
	if wr.CountMs != 3 {
		t.Errorf("SumLastN(1).CountMs = %d, want 3", wr.CountMs)
	}
}

func TestMetricRingBuffer_AddDuration_Single(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.AddDuration("runner.completion", 500)

	wr := rb.SumLastN(1)
	if wr.AvgMs != 500 {
		t.Errorf("SumLastN(1).AvgMs = %.2f, want 500", wr.AvgMs)
	}
}

func TestMetricRingBuffer_RecordCompletion_Success(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("success", 150)

	wr := rb.SumLastN(1)
	if wr.Total != 1 {
		t.Errorf("SumLastN(1).Total = %d, want 1", wr.Total)
	}
	if wr.Errors != 0 {
		t.Errorf("SumLastN(1).Errors = %d, want 0", wr.Errors)
	}
	if wr.AvgMs != 150 {
		t.Errorf("SumLastN(1).AvgMs = %.2f, want 150", wr.AvgMs)
	}
}

func TestMetricRingBuffer_RecordCompletion_Error(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("error", 300)

	wr := rb.SumLastN(1)
	if wr.Total != 1 {
		t.Errorf("SumLastN(1).Total = %d, want 1", wr.Total)
	}
	if wr.Errors != 1 {
		t.Errorf("SumLastN(1).Errors = %d, want 1", wr.Errors)
	}
	if wr.AvgMs != 300 {
		t.Errorf("SumLastN(1).AvgMs = %.2f, want 300", wr.AvgMs)
	}
}

func TestMetricRingBuffer_RecordCompletion_ErrorZeroDuration(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("error", 0)

	wr := rb.SumLastN(1)
	if wr.Total != 1 {
		t.Errorf("SumLastN(1).Total = %d, want 1", wr.Total)
	}
	if wr.Errors != 1 {
		t.Errorf("SumLastN(1).Errors = %d, want 1", wr.Errors)
	}
	if wr.AvgMs != 0 {
		t.Errorf("SumLastN(1).AvgMs = %.2f, want 0", wr.AvgMs)
	}
	if wr.CountMs != 0 {
		t.Errorf("SumLastN(1).CountMs = %d, want 0", wr.CountMs)
	}
}

func TestMetricRingBuffer_RecordCompletion_NegativeDuration(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("success", -100)

	wr := rb.SumLastN(1)
	if wr.Total != 1 {
		t.Errorf("SumLastN(1).Total = %d, want 1", wr.Total)
	}
	if wr.CountMs != 0 {
		t.Errorf("SumLastN(1).CountMs = %d, want 0 (negative duration not recorded)", wr.CountMs)
	}
}

func TestMetricRingBuffer_SumLastN_Empty(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	wr := rb.SumLastN(10)
	if wr.Total != 0 {
		t.Errorf("SumLastN(10).Total = %d, want 0", wr.Total)
	}
	if wr.Errors != 0 {
		t.Errorf("SumLastN(10).Errors = %d, want 0", wr.Errors)
	}
	if wr.AvgMs != 0 {
		t.Errorf("SumLastN(10).AvgMs = %.2f, want 0", wr.AvgMs)
	}
}

func TestMetricRingBuffer_SumLastN_ZeroWindow(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	wr := rb.SumLastN(0)
	if wr.Total != 1 {
		t.Errorf("SumLastN(0).Total = %d, want 1 (zero defaults to capacity)", wr.Total)
	}
}

func TestMetricRingBuffer_SumLastN_NegativeWindow(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	wr := rb.SumLastN(-5)
	if wr.Total != 1 {
		t.Errorf("SumLastN(-5).Total = %d, want 1 (negative defaults to capacity)", wr.Total)
	}
}

func TestMetricRingBuffer_SumLastN_WindowExceedsCapacity(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	wr := rb.SumLastN(9999)
	if wr.Total != 1 {
		t.Errorf("SumLastN(9999).Total = %d, want 1 (clamped to capacity)", wr.Total)
	}
}

func TestMetricRingBuffer_MultipleKeys(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")
	rb.IncTotal("other.metric")

	wr := rb.SumLastN(1)
	if wr.Total != 1 {
		t.Errorf("SumLastN(1).Total = %d, want 1 (only runner.completion counted)", wr.Total)
	}
}

func TestMetricRingBuffer_IncTotalAndErrorCombined(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	for i := 0; i < 10; i++ {
		rb.IncTotal("runner.completion")
		if i%3 == 0 {
			rb.IncError("runner.completion")
		}
	}

	wr := rb.SumLastN(1)
	if wr.Total != 10 {
		t.Errorf("Total = %d, want 10", wr.Total)
	}
	if wr.Errors != 4 {
		t.Errorf("Errors = %d, want 4", wr.Errors)
	}
}

func TestMetricRingBuffer_AddDuration_NoDurations(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.IncTotal("runner.completion")

	wr := rb.SumLastN(1)
	if wr.AvgMs != 0 {
		t.Errorf("AvgMs = %.2f, want 0 when no durations recorded", wr.AvgMs)
	}
}

func TestMetricRingBuffer_RecordCompletion_Mixed(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("success", 100)
	rb.RecordCompletion("success", 200)
	rb.RecordCompletion("error", 500)
	rb.RecordCompletion("success", 150)

	wr := rb.SumLastN(1)
	if wr.Total != 4 {
		t.Errorf("Total = %d, want 4", wr.Total)
	}
	if wr.Errors != 1 {
		t.Errorf("Errors = %d, want 1", wr.Errors)
	}
	if wr.AvgMs != 237.5 {
		t.Errorf("AvgMs = %.2f, want 237.5", wr.AvgMs)
	}
}

func TestMetricRingBuffer_WindowResult_LargeVolume(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	for i := 0; i < 1000; i++ {
		status := "success"
		if i%10 == 0 {
			status = "error"
		}
		rb.RecordCompletion(status, int64(i+1))
	}

	wr := rb.SumLastN(1)
	if wr.Total != 1000 {
		t.Errorf("Total = %d, want 1000", wr.Total)
	}
	if wr.Errors != 100 {
		t.Errorf("Errors = %d, want 100", wr.Errors)
	}
	if wr.CountMs != 1000 {
		t.Errorf("CountMs = %d, want 1000", wr.CountMs)
	}
}
