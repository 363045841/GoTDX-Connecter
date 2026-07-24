package api

import (
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

func TestFilterKLineByDateSortsAndDeduplicatesIndexBars(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	klines := []proto.SecurityBar{
		{Year: 2026, Month: 7, Day: 24, Hour: 15, Minute: 0, DateTime: time.Date(2026, 7, 24, 15, 0, 0, 0, loc)},
		{Year: 2026, Month: 7, Day: 23, Hour: 15, Minute: 0, DateTime: time.Date(2026, 7, 23, 15, 0, 0, 0, loc)},
		{Year: 2026, Month: 7, Day: 23, Hour: 15, Minute: 0, DateTime: time.Date(2026, 7, 23, 15, 0, 0, 0, loc)},
		{Year: 2026, Month: 7, Day: 22, Hour: 15, Minute: 0, DateTime: time.Date(2026, 7, 22, 15, 0, 0, 0, loc)},
	}

	got := filterKLineByDate(
		klines,
		time.Date(2026, 7, 23, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 24, 0, 0, 0, 0, loc),
		true,
	)

	if len(got) != 2 {
		t.Fatalf("filtered bars = %d, want 2", len(got))
	}
	if got[0].DateTime.Day() != 23 || got[1].DateTime.Day() != 24 {
		t.Fatalf("dates = %s, %s; want ascending 23, 24", got[0].DateTime, got[1].DateTime)
	}
}

func TestIndexPageSizeStaysBelowGetIndexBarsLimit(t *testing.T) {
	if indexPageSize != 798 {
		t.Fatalf("index page size = %d, want 798", indexPageSize)
	}
	if indexPageSize >= 801 {
		t.Fatalf("index page size = %d exceeds the verified GetIndexBars limit", indexPageSize)
	}
}
