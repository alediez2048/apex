package settlement

import (
	"time"

	_ "time/tzdata"
)

// CT is Central Time (America/Chicago) for EOD cutoff.
var ct *time.Location

func init() {
	var err error
	ct, err = time.LoadLocation("America/Chicago")
	if err != nil {
		ct = time.UTC
	}
}

// CutoffTime returns 6:30 PM CT on the given date (in CT).
func CutoffTime(t time.Time) time.Time {
	in := t.In(ct)
	y, m, d := in.Date()
	return time.Date(y, m, d, 18, 30, 0, 0, ct)
}

// AfterCutoff reports whether t is strictly after 6:30 PM CT on that day.
func AfterCutoff(t time.Time) bool {
	return t.In(ct).After(CutoffTime(t))
}

// SettlementDate returns the settlement date (YYYY-MM-DD) for a batch:
// if t is before 6:30 PM CT that day, return that day; otherwise next business day (skip Sat/Sun).
func SettlementDate(t time.Time) string {
	in := t.In(ct)
	cutoff := CutoffTime(t)
	if !in.After(cutoff) {
		return in.Format("2006-01-02")
	}
	next := NextBusinessDay(in)
	return next.Format("2006-01-02")
}

// NextBusinessDay returns the next weekday after t (same day if already weekday and we use it as "next" for rollover).
// Used when after cutoff: add one day, then skip weekend.
func NextBusinessDay(t time.Time) time.Time {
	next := t.AddDate(0, 0, 1)
	for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
