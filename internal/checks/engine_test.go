package checks

import (
	"testing"
	"time"

	"github.com/pridkett/paperless-dashboard/internal/config"
	"github.com/pridkett/paperless-dashboard/internal/paperless"
)

var now = time.Date(2026, time.July, 7, 12, 0, 0, 0, time.Local)

func TestBuildPeriodsMonthly(t *testing.T) {
	ck := config.Check{Frequency: "monthly", Lookback: 12}
	periods := buildPeriods(ck, now)

	if len(periods) != 12 {
		t.Fatalf("got %d periods, want 12", len(periods))
	}
	if periods[0].Label != "Aug 2025" {
		t.Errorf("first period = %q, want Aug 2025", periods[0].Label)
	}
	if periods[11].Label != "Jul 2026" {
		t.Errorf("last period = %q, want Jul 2026", periods[11].Label)
	}
	if !periods[11].End.Equal(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("last period end = %v, want Aug 1 2026", periods[11].End)
	}
}

func TestBuildPeriodsQuarterly(t *testing.T) {
	ck := config.Check{Frequency: "quarterly", Lookback: 4}
	periods := buildPeriods(ck, now)

	want := []string{"Q4 2025", "Q1 2026", "Q2 2026", "Q3 2026"}
	for i, w := range want {
		if periods[i].Label != w {
			t.Errorf("period %d = %q, want %q", i, periods[i].Label, w)
		}
	}
}

func TestClassify(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 3}
	periods := buildPeriods(ck, now) // May, Jun, Jul 2026

	docs := []paperless.Document{
		{Created: "2026-05-14"}, // May present
		// June missing, July (current) empty
	}
	e.classify(ck, periods, docs, now)

	if periods[0].Status != StatusOK {
		t.Errorf("May = %s, want ok", periods[0].Status)
	}
	if periods[1].Status != StatusMissing {
		t.Errorf("Jun = %s, want missing", periods[1].Status)
	}
	// July 7 with default 31-day grace: current month still pending.
	if periods[2].Status != StatusPending {
		t.Errorf("Jul = %s, want pending", periods[2].Status)
	}
}

func TestClassifyGraceExpired(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 2, GraceDays: 5}
	periods := buildPeriods(ck, now) // Jun, Jul 2026
	e.classify(ck, periods, nil, now)

	// July 7 is past a 5-day grace window: current month is missing.
	if periods[1].Status != StatusMissing {
		t.Errorf("Jul = %s, want missing", periods[1].Status)
	}
}

func TestClassifyYearlyExpectedMonth(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "yearly", Lookback: 2, ExpectedMonth: 7}
	periods := buildPeriods(ck, now) // 2025, 2026
	e.classify(ck, periods, nil, now)

	if periods[0].Status != StatusMissing {
		t.Errorf("2025 = %s, want missing", periods[0].Status)
	}
	// Expected in July; it's only July 7, so 2026 is still pending.
	if periods[1].Status != StatusPending {
		t.Errorf("2026 = %s, want pending", periods[1].Status)
	}

	// After August 1 the expected month has passed: missing.
	later := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.Local)
	periods = buildPeriods(ck, later)
	e.classify(ck, periods, nil, later)
	if periods[1].Status != StatusMissing {
		t.Errorf("2026 after due date = %s, want missing", periods[1].Status)
	}
}

func TestFirstOfMonthUTCStaysInItsMonth(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 3}
	periods := buildPeriods(ck, now) // May, Jun, Jul 2026

	// A statement dated June 1 that Paperless serializes as UTC midnight.
	// In any timezone west of UTC this instant is still May 31 locally,
	// but it must be bucketed by its calendar date: June.
	docs := []paperless.Document{
		{ID: 7, Title: "Freedom Mortgage Jun 2026", Created: "2026-06-01T00:00:00Z"},
	}
	e.classify(ck, periods, docs, now)

	if periods[0].Status != StatusMissing {
		t.Errorf("May = %s, want missing (doc belongs to June)", periods[0].Status)
	}
	if periods[1].Status != StatusOK || periods[1].DocID != 7 {
		t.Errorf("Jun = %s (doc %d), want ok with doc 7", periods[1].Status, periods[1].DocID)
	}
}

func TestCreatedDateFieldPreferred(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 2}
	periods := buildPeriods(ck, now) // Jun, Jul 2026

	// created_date (the date shown in the Paperless UI) wins over the
	// created instant when both are present.
	docs := []paperless.Document{
		{ID: 8, Created: "2026-05-31T22:00:00Z", CreatedDate: "2026-06-01"},
	}
	e.classify(ck, periods, docs, now)

	if periods[0].Status != StatusOK {
		t.Errorf("Jun = %s, want ok via created_date", periods[0].Status)
	}
}

func TestPeriodLinksToNewestDocument(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 2}
	periods := buildPeriods(ck, now) // Jun, Jul 2026

	docs := []paperless.Document{
		{ID: 1, Title: "early", Created: "2026-06-02"},
		{ID: 2, Title: "late", Created: "2026-06-20"},
	}
	e.classify(ck, periods, docs, now)

	if periods[0].DocID != 2 || periods[0].Doc != "late" {
		t.Errorf("Jun links to doc %d (%q), want the newest (2, late)", periods[0].DocID, periods[0].Doc)
	}
	if periods[0].Count != 2 {
		t.Errorf("Jun count = %d, want 2", periods[0].Count)
	}
	if periods[1].DocID != 0 {
		t.Errorf("empty Jul has DocID %d, want 0", periods[1].DocID)
	}
}

func TestRFC3339CreatedDates(t *testing.T) {
	e := &Engine{}
	ck := config.Check{Frequency: "monthly", Lookback: 2}
	periods := buildPeriods(ck, now)

	docs := []paperless.Document{{Created: "2026-06-15T10:30:00-04:00"}}
	e.classify(ck, periods, docs, now)
	if periods[0].Status != StatusOK {
		t.Errorf("Jun with RFC3339 date = %s, want ok", periods[0].Status)
	}
}
