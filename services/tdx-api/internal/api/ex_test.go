package api

import (
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

func TestParseExKLineDateTimeAcceptsGotdxSpaceFormat(t *testing.T) {
	loc := time.Local
	got, err := parseExKLineDateTime("2026-07-22 15:00:00")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := time.Date(2026, 7, 22, 15, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseExKLineDateTimeAcceptsDateOnly(t *testing.T) {
	got, err := parseExKLineDateTime("2025-07-24")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := time.Date(2025, 7, 24, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterExKLineByDateKeepsBarsWithSpaceDateTime(t *testing.T) {
	loc := time.Local
	klines := []proto.ExKLineItem{
		{DateTime: "2026-07-22 15:00:00", Open: 1, High: 1, Low: 1, Close: 1},
		{DateTime: "2026-07-23 15:00:00", Open: 2, High: 2, Low: 2, Close: 2},
		{DateTime: "2026-07-23 15:00:00", Open: 2, High: 2, Low: 2, Close: 2}, // dup
		{DateTime: "2026-07-24 15:00:00", Open: 3, High: 3, Low: 3, Close: 3},
		{DateTime: "2026-07-25 15:00:00", Open: 4, High: 4, Low: 4, Close: 4},
	}

	got := filterExKLineByDate(
		klines,
		time.Date(2026, 7, 23, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 24, 0, 0, 0, 0, loc),
	)

	if len(got) != 2 {
		t.Fatalf("filtered bars = %d, want 2; got %#v", len(got), got)
	}
	if got[0].DateTime != "2026-07-23 15:00:00" || got[1].DateTime != "2026-07-24 15:00:00" {
		t.Fatalf("dates = %q, %q; want ascending 23 then 24", got[0].DateTime, got[1].DateTime)
	}
}
