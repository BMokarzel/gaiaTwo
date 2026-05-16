package cost

import (
	"testing"
	"time"
)

func TestPeriod_IsZero(t *testing.T) {
	if !(Period{}).IsZero() {
		t.Fatal("zero period must report IsZero=true")
	}
	now := time.Now().UTC()
	if (Period{Start: now}).IsZero() {
		t.Fatal("period with Start set must not be zero")
	}
}

func TestPeriod_Contains(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	p := Period{Start: t0, End: t1}

	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"start is inclusive", t0, true},
		{"middle inside", t0.Add(30 * time.Minute), true},
		{"end is exclusive", t1, false},
		{"before start", t0.Add(-time.Second), false},
		{"after end", t1.Add(time.Second), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := p.Contains(c.t); got != c.want {
				t.Fatalf("Contains(%v) = %v, want %v", c.t, got, c.want)
			}
		})
	}

	if (Period{}).Contains(t0) {
		t.Fatal("zero period must contain nothing")
	}
}

func TestLine_HasResource(t *testing.T) {
	if (Line{}).HasResource() {
		t.Fatal("empty line should not have resource")
	}
	if !(Line{ResourceID: "i-abc"}).HasResource() {
		t.Fatal("line with ResourceID must report HasResource=true")
	}
}

func TestLine_DedupKey(t *testing.T) {
	bp := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	l := Line{ReportID: "r1", LineItemID: "li-42", BillingPeriodStart: bp}
	r, id, when := l.DedupKey()
	if r != "r1" || id != "li-42" || !when.Equal(bp) {
		t.Fatalf("DedupKey() = (%q,%q,%v), want (r1,li-42,%v)", r, id, when, bp)
	}
}
