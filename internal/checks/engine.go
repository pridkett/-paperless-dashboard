// Package checks evaluates document-freshness rules against a Paperless-NGX
// instance. Each check divides its lookback window into periods (months,
// quarters, or years) and classifies every period as OK, missing, or pending
// (the current period, still within its grace window).
package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pridkett/paperless-dashboard/internal/config"
	"github.com/pridkett/paperless-dashboard/internal/currency"
	"github.com/pridkett/paperless-dashboard/internal/paperless"
)

// PeriodStatus classifies one period of one check.
type PeriodStatus string

const (
	StatusOK      PeriodStatus = "ok"
	StatusMissing PeriodStatus = "missing"
	StatusPending PeriodStatus = "pending" // current period, grace window still open
)

// Period is one time bucket within a check's lookback window, oldest first.
type Period struct {
	Label  string // e.g. "Jan 2026", "Q3 2025", "2024"
	Start  time.Time
	End    time.Time // exclusive
	Status PeriodStatus
	Count  int    // documents found in this period
	DocID  int    // newest document in this period (0 if none)
	Doc    string // title of that document

	newestCreated string // raw created stamp of DocID, for newest-wins comparison
}

// FieldValue is the most recent value seen for a tracked custom field.
type FieldValue struct {
	Name  string
	Value string
}

// Result is the evaluation of one check.
type Result struct {
	Check        config.Check
	Periods      []Period
	Missing      int // periods with StatusMissing
	Total        int
	LatestDoc    string // title of newest matching document
	LatestID     int    // its Paperless document ID (0 if none)
	LatestDate   time.Time
	CustomValues []FieldValue
	Err          string // non-empty if the check could not be evaluated
}

// Healthy reports whether the check has no missing periods and no error.
func (r Result) Healthy() bool { return r.Err == "" && r.Missing == 0 }

// Engine evaluates checks against one Paperless client.
type Engine struct {
	Client *paperless.Client
}

// Evaluate runs every check. Individual check failures (e.g. a misspelled
// correspondent) are reported in the Result rather than aborting the run.
func (e *Engine) Evaluate(ctx context.Context, cks []config.Check, now time.Time) []Result {
	results := make([]Result, len(cks))
	for i, ck := range cks {
		results[i] = e.evaluate(ctx, ck, now)
	}
	return results
}

func (e *Engine) evaluate(ctx context.Context, ck config.Check, now time.Time) Result {
	res := Result{Check: ck}

	filter, err := e.buildFilter(ctx, ck)
	if err != nil {
		res.Err = err.Error()
		return res
	}

	periods := buildPeriods(ck, now)
	if len(periods) == 0 {
		res.Err = "no periods to evaluate"
		return res
	}
	filter.CreatedFrom = periods[0].Start
	filter.CreatedTo = periods[len(periods)-1].End.AddDate(0, 0, -1)

	docs, err := e.Client.Documents(ctx, filter)
	if err != nil {
		res.Err = err.Error()
		return res
	}

	e.classify(ck, periods, docs, now)
	res.Periods = periods
	res.Total = len(periods)
	for _, p := range periods {
		if p.Status == StatusMissing {
			res.Missing++
		}
	}

	e.latestInfo(ctx, &res, docs)
	return res
}

func (e *Engine) buildFilter(ctx context.Context, ck config.Check) (paperless.Filter, error) {
	var f paperless.Filter
	var err error
	if ck.Correspondent != "" {
		if f.CorrespondentID, err = e.Client.CorrespondentID(ctx, ck.Correspondent); err != nil {
			return f, err
		}
	}
	if ck.DocumentType != "" {
		if f.DocumentTypeID, err = e.Client.DocumentTypeID(ctx, ck.DocumentType); err != nil {
			return f, err
		}
	}
	for _, tag := range ck.Tags {
		id, err := e.Client.TagID(ctx, tag)
		if err != nil {
			return f, err
		}
		f.TagIDs = append(f.TagIDs, id)
	}
	return f, nil
}

// buildPeriods returns the check's time buckets, oldest first, ending with
// the period containing now.
func buildPeriods(ck config.Check, now time.Time) []Period {
	var periods []Period
	switch ck.Frequency {
	case "quarterly":
		q := (int(now.Month()) - 1) / 3
		cur := time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, time.Local)
		for i := ck.Lookback - 1; i >= 0; i-- {
			start := cur.AddDate(0, -3*i, 0)
			periods = append(periods, Period{
				Label: fmt.Sprintf("Q%d %d", (int(start.Month())-1)/3+1, start.Year()),
				Start: start,
				End:   start.AddDate(0, 3, 0),
			})
		}
	case "yearly":
		for i := ck.Lookback - 1; i >= 0; i-- {
			start := time.Date(now.Year()-i, time.January, 1, 0, 0, 0, 0, time.Local)
			periods = append(periods, Period{
				Label: fmt.Sprintf("%d", start.Year()),
				Start: start,
				End:   start.AddDate(1, 0, 0),
			})
		}
	default: // monthly
		cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		for i := ck.Lookback - 1; i >= 0; i-- {
			start := cur.AddDate(0, -i, 0)
			periods = append(periods, Period{
				Label: start.Format("Jan 2006"),
				Start: start,
				End:   start.AddDate(0, 1, 0),
			})
		}
	}
	return periods
}

// classify marks each period OK, missing, or pending based on the documents
// found and, for the current period, the check's grace rules.
func (e *Engine) classify(ck config.Check, periods []Period, docs []paperless.Document, now time.Time) {
	for _, doc := range docs {
		created, err := doc.CreatedTime()
		if err != nil {
			continue
		}
		for i := range periods {
			if !created.Before(periods[i].Start) && created.Before(periods[i].End) {
				periods[i].Count++
				// Remember the newest document so the dashboard can link to it.
				if periods[i].DocID == 0 || doc.Created > periods[i].newestCreated {
					periods[i].DocID = doc.ID
					periods[i].Doc = doc.Title
					periods[i].newestCreated = doc.Created
				}
				break
			}
		}
	}

	for i := range periods {
		p := &periods[i]
		if p.Count > 0 {
			p.Status = StatusOK
			continue
		}
		p.Status = StatusMissing
		if !p.End.After(now) {
			continue // fully in the past: definitively missing
		}
		// Current period: apply grace rules before declaring it missing.
		if ck.Frequency == "yearly" && ck.ExpectedMonth > 0 {
			// Not yet due until the expected filing month (plus grace) passes.
			due := time.Date(p.Start.Year(), time.Month(ck.ExpectedMonth), 1, 0, 0, 0, 0, time.Local).
				AddDate(0, 1, ck.GraceDays)
			if now.Before(due) {
				p.Status = StatusPending
			}
			continue
		}
		grace := ck.GraceDays
		if grace <= 0 {
			grace = defaultGraceDays(ck.Frequency)
		}
		if now.Before(p.Start.AddDate(0, 0, grace)) {
			p.Status = StatusPending
		}
	}
}

// defaultGraceDays is how far into a new period we wait before flagging it:
// most bills arrive within the period itself, so the whole period is grace.
func defaultGraceDays(freq string) int {
	switch freq {
	case "quarterly":
		return 92
	case "yearly":
		return 366
	default:
		return 31
	}
}

// latestInfo records the newest document's title/date and its tracked custom
// field values on the result.
func (e *Engine) latestInfo(ctx context.Context, res *Result, docs []paperless.Document) {
	if len(docs) == 0 {
		return
	}
	sorted := make([]paperless.Document, len(docs))
	copy(sorted, docs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Created > sorted[j].Created })
	newest := sorted[0]

	res.LatestDoc = newest.Title
	res.LatestID = newest.ID
	if t, err := newest.CreatedTime(); err == nil {
		res.LatestDate = t
	}

	for _, name := range res.Check.CustomFields {
		field, err := e.Client.CustomField(ctx, name)
		if err != nil {
			res.CustomValues = append(res.CustomValues, FieldValue{Name: name, Value: "unknown field"})
			continue
		}
		for _, cf := range newest.CustomFields {
			if cf.Field == field.ID {
				res.CustomValues = append(res.CustomValues, FieldValue{
					Name:  name,
					Value: formatFieldValue(field, rawToString(cf.Value)),
				})
				break
			}
		}
	}
}

// formatFieldValue renders one custom field value for display. Monetary
// fields get a currency symbol and grouped digits; a value that already
// names its currency keeps that one, and a bare value inherits the currency
// the field itself defaults to, which is what the Paperless UI shows.
func formatFieldValue(f paperless.CustomField, raw string) string {
	if !f.Monetary() {
		return raw
	}
	code, amount := currency.Split(raw)
	if code == "" {
		code = f.ExtraData.DefaultCurrency
	}
	return currency.Format(code, amount)
}

func rawToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}
