package modelcatalog

import "testing"

func TestBackfillCostFromMicro(t *testing.T) {
	in := `{"input_price_micro_usd_per_1k":3000,"output_price_micro_usd_per_1k":15000}`
	out, changed := BackfillCostFromMicro(in)
	if !changed {
		t.Fatal("expected backfill")
	}
	if out == in {
		t.Fatalf("unchanged output")
	}
}
