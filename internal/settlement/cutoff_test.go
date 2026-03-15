package settlement

import (
	"testing"
	"time"
)

func TestSettlementDate_BeforeCutoff_SameDay(t *testing.T) {
	// 2026-03-09 18:00 CT = before 6:30 PM CT
	loc, _ := time.LoadLocation("America/Chicago")
	tt := time.Date(2026, 3, 9, 18, 0, 0, 0, loc)
	got := SettlementDate(tt.UTC())
	if got != "2026-03-09" {
		t.Fatalf("want 2026-03-09, got %s", got)
	}
}

func TestSettlementDate_AfterCutoff_NextBusinessDay(t *testing.T) {
	// 2026-03-09 19:00 CT = after 6:30 PM CT -> next day 2026-03-10 (Tuesday)
	loc, _ := time.LoadLocation("America/Chicago")
	tt := time.Date(2026, 3, 9, 19, 0, 0, 0, loc)
	got := SettlementDate(tt.UTC())
	if got != "2026-03-10" {
		t.Fatalf("want 2026-03-10, got %s", got)
	}
}

func TestSettlementDate_Friday7PM_Monday(t *testing.T) {
	// Friday 2026-03-13 19:00 CT -> next business day is Monday 2026-03-16
	loc, _ := time.LoadLocation("America/Chicago")
	tt := time.Date(2026, 3, 13, 19, 0, 0, 0, loc)
	got := SettlementDate(tt.UTC())
	if got != "2026-03-16" {
		t.Fatalf("want 2026-03-16 (Monday), got %s", got)
	}
}

func TestAfterCutoff(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	before := time.Date(2026, 3, 9, 18, 29, 0, 0, loc)
	after := time.Date(2026, 3, 9, 18, 31, 0, 0, loc)
	if AfterCutoff(before.UTC()) {
		t.Fatal("18:29 CT should not be after cutoff")
	}
	if !AfterCutoff(after.UTC()) {
		t.Fatal("18:31 CT should be after cutoff")
	}
}
