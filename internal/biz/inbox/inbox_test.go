package inbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSource struct {
	name    string
	items   []Item
	total   int64
	listErr error

	decided []string // "<nativeID>|<action>|<note>"
	decErr  error
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) List(_ context.Context, limit int) ([]Item, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	items := append([]Item(nil), f.items...)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, f.total, nil
}

func (f *fakeSource) Decide(_ context.Context, item Item, action, note string) error {
	if f.decErr != nil {
		return f.decErr
	}
	f.decided = append(f.decided, item.NativeID+"|"+action+"|"+note)
	return nil
}

var t0 = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListSortsByRiskThenDueThenCreated(t *testing.T) {
	a := &fakeSource{name: "a", items: []Item{
		{NativeID: "low-new", Risk: RiskLow, CreatedAt: t0.Add(time.Hour)},
		{NativeID: "high-old", Risk: RiskHigh, CreatedAt: t0},
		{NativeID: "med-nodue", Risk: RiskMedium, CreatedAt: t0.Add(2 * time.Hour)},
	}}
	b := &fakeSource{name: "b", items: []Item{
		{NativeID: "high-due-late", Risk: RiskHigh, CreatedAt: t0, DueAt: t0.Add(3 * time.Hour)},
		{NativeID: "high-due-soon", Risk: RiskHigh, CreatedAt: t0, DueAt: t0.Add(time.Hour)},
		{NativeID: "med-due", Risk: RiskMedium, CreatedAt: t0, DueAt: t0.Add(time.Hour)},
		{NativeID: "unknown-risk", Risk: "", CreatedAt: t0},
	}}
	res, err := NewAggregator(a, b).List(context.Background(), nil, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"b:high-due-soon", "b:high-due-late", "a:high-old",
		"b:med-due", "a:med-nodue", "b:unknown-risk",
		"a:low-new",
	}
	if got := ids(res.Items); !equal(got, want) {
		t.Fatalf("order\n got %v\nwant %v", got, want)
	}
	if res.Total != 7 || res.HighRisk != 3 {
		t.Fatalf("total=%d high=%d", res.Total, res.HighRisk)
	}
	if res.Items[5].Risk != RiskMedium {
		t.Fatalf("unknown risk should normalize to medium, got %q", res.Items[5].Risk)
	}
}

func TestListUsesSourceTotalAndPaginates(t *testing.T) {
	a := &fakeSource{name: "a", total: 40, items: []Item{
		{NativeID: "1", CreatedAt: t0.Add(3 * time.Minute)},
		{NativeID: "2", CreatedAt: t0.Add(2 * time.Minute)},
		{NativeID: "3", CreatedAt: t0.Add(time.Minute)},
	}}
	agg := NewAggregator(a)
	res, err := agg.List(context.Background(), nil, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(res.Items); !equal(got, []string{"a:3"}) {
		t.Fatalf("page 2 = %v", got)
	}
	if res.Total != 40 || res.Counts["a"] != 40 {
		t.Fatalf("total should come from source: %d / %d", res.Total, res.Counts["a"])
	}
	res, _ = agg.List(context.Background(), nil, 9, 2)
	if len(res.Items) != 0 || res.Total != 40 {
		t.Fatalf("out-of-range page: items=%d total=%d", len(res.Items), res.Total)
	}
}

func TestFailingSourceIsDegradedNotFatal(t *testing.T) {
	ok := &fakeSource{name: "ok", items: []Item{{NativeID: "1", Risk: RiskHigh}}}
	bad := &fakeSource{name: "bad", listErr: errors.New("boom")}
	agg := NewAggregator(ok, bad)

	res, err := agg.List(context.Background(), nil, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Total != 1 {
		t.Fatalf("healthy source must still be returned: %+v", res)
	}
	if !equal(res.Degraded, []string{"bad"}) {
		t.Fatalf("degraded = %v", res.Degraded)
	}
	if n, present := res.Counts["bad"]; !present || n != 0 {
		t.Fatalf("degraded source should be listed with 0, got %d present=%v", n, present)
	}

	sum := agg.Summary(context.Background())
	if sum.Items != nil || sum.Total != 1 || sum.HighRisk != 1 || !equal(sum.Degraded, []string{"bad"}) {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestSourceFilter(t *testing.T) {
	a := &fakeSource{name: "a", items: []Item{{NativeID: "1"}}}
	b := &fakeSource{name: "b", items: []Item{{NativeID: "2"}}}
	agg := NewAggregator(a, b)

	res, err := agg.List(context.Background(), []string{"b", "", "b"}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(res.Items); !equal(got, []string{"b:2"}) {
		t.Fatalf("filtered = %v", got)
	}
	if !equal(res.Order, []string{"b"}) {
		t.Fatalf("order = %v", res.Order)
	}
	if _, err := agg.List(context.Background(), []string{"nope"}, 1, 50); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("want ErrUnknownSource, got %v", err)
	}
}

func TestRegistrationSkipsInvalidAndKeepsOrder(t *testing.T) {
	agg := NewAggregator(
		&fakeSource{name: "a"},
		nil,
		&fakeSource{name: ""},
		&fakeSource{name: "has:colon"},
		&fakeSource{name: "b"},
		&fakeSource{name: "a", items: []Item{{NativeID: "x"}}},
	)
	if got := agg.Sources(); !equal(got, []string{"a", "b"}) {
		t.Fatalf("sources = %v", got)
	}
	res, _ := agg.List(context.Background(), nil, 1, 50)
	if len(res.Items) != 1 {
		t.Fatalf("later registration should override earlier one")
	}
}

func TestDecide(t *testing.T) {
	src := &fakeSource{name: "s", items: []Item{
		{NativeID: "a:b:c", Actions: []string{ActionApprove, ActionReject}, RejectRequiresNote: true},
		{NativeID: "ackonly", Actions: []string{ActionAck}},
	}}
	agg := NewAggregator(src)
	ctx := context.Background()

	if _, err := agg.Decide(ctx, "nocolon", ActionApprove, ""); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid id: %v", err)
	}
	if _, err := agg.Decide(ctx, "other:1", ActionApprove, ""); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("unknown source: %v", err)
	}
	if _, err := agg.Decide(ctx, "s:missing", ActionApprove, ""); !errors.Is(err, ErrGone) {
		t.Fatalf("gone: %v", err)
	}
	if _, err := agg.Decide(ctx, "s:ackonly", ActionApprove, ""); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("action not allowed: %v", err)
	}
	if _, err := agg.Decide(ctx, "s:a:b:c", ActionReject, "   "); !errors.Is(err, ErrNoteRequired) {
		t.Fatalf("note required: %v", err)
	}
	if len(src.decided) != 0 {
		t.Fatalf("rejected requests must not reach the source: %v", src.decided)
	}

	it, err := agg.Decide(ctx, " s:a:b:c ", " Reject ", " too risky ")
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != "s:a:b:c" || it.Source != "s" {
		t.Fatalf("item = %+v", it)
	}
	if !equal(src.decided, []string{"a:b:c|reject|too risky"}) {
		t.Fatalf("decided = %v", src.decided)
	}

	src.decErr = errors.New("domain says no")
	if _, err := agg.Decide(ctx, "s:ackonly", ActionAck, ""); err == nil || err.Error() != "domain says no" {
		t.Fatalf("domain error must pass through unchanged, got %v", err)
	}
}

func TestDecideSurfacesListFailure(t *testing.T) {
	src := &fakeSource{name: "s", listErr: errors.New("db down")}
	if _, err := NewAggregator(src).Decide(context.Background(), "s:1", ActionApprove, ""); err == nil || errors.Is(err, ErrGone) {
		t.Fatalf("list failure must not be reported as gone: %v", err)
	}
}
