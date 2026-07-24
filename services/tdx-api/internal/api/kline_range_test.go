package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

func dayBar(y int, m time.Month, d int) proto.SecurityBar {
	loc := time.Local
	return proto.SecurityBar{
		Year:     y,
		Month:    int(m),
		Day:      d,
		Hour:     15,
		Minute:   0,
		DateTime: time.Date(y, m, d, 15, 0, 0, 0, loc),
	}
}

func TestPaginateFromRecentAdvancesByActualLen(t *testing.T) {
	// 短页 3 条：若误用 requested pageSize 推进会跳页
	var starts []uint32
	pages := map[uint32][]proto.SecurityBar{
		0: {
			dayBar(2024, 1, 3),
			dayBar(2024, 1, 4),
			dayBar(2024, 1, 5),
		},
		3: {
			dayBar(2023, 12, 1),
			dayBar(2023, 12, 2),
			dayBar(2023, 12, 3),
		},
		6: {},
	}
	startDate := time.Date(2023, 12, 1, 0, 0, 0, 0, time.Local)
	got, err := paginateFromRecent(798, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		starts = append(starts, start)
		_ = count
		return pages[start], nil
	}, securityBarOldest, startDate, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) < 2 || starts[0] != 0 || starts[1] != 3 {
		t.Fatalf("starts = %v, want [0, 3, ...]", starts)
	}
	if len(got) != 6 {
		t.Fatalf("bars = %d, want 6", len(got))
	}
}

func TestPaginateFromRecentStopsWhenOldestBeforeStartDate(t *testing.T) {
	var starts []uint32
	pages := map[uint32][]proto.SecurityBar{
		0: {dayBar(2024, 6, 1), dayBar(2024, 6, 2)},
		2: {dayBar(2023, 1, 1), dayBar(2023, 1, 2)}, // 最旧已早于 startDate
		4: {dayBar(2022, 1, 1)},                      // 不应再请求
	}
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	got, err := paginateFromRecent(798, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		starts = append(starts, start)
		return pages[start], nil
	}, securityBarOldest, startDate, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) != 2 {
		t.Fatalf("starts = %v, want exactly 2 pages", starts)
	}
	if len(got) != 4 {
		t.Fatalf("bars = %d, want 4 (second page kept for filter)", len(got))
	}
}

func TestPaginateFromRecentToleratesPartialError(t *testing.T) {
	pages := 0
	startDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	got, err := paginateFromRecent(798, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		pages++
		if start == 0 {
			return []proto.SecurityBar{dayBar(2024, 1, 1), dayBar(2024, 1, 2)}, nil
		}
		return nil, fmt.Errorf("invalid kline datetime: 1376385400")
	}, securityBarOldest, startDate, true)
	if err != nil {
		t.Fatalf("want partial success, got err %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("bars = %d, want 2 from first page", len(got))
	}
	if pages != 2 {
		t.Fatalf("pages = %d, want 2", pages)
	}
}

func TestIndexKLineRangeUsesFallbackCountThenPartial(t *testing.T) {
	prev := fetchIndexBarsPage
	t.Cleanup(func() { fetchIndexBarsPage = prev })

	type call struct {
		start, count uint16
	}
	var calls []call
	fetchIndexBarsPage = func(category uint16, market uint8, code string, start, count uint16) ([]proto.SecurityBar, error) {
		calls = append(calls, call{start, count})
		if start == 0 && count == indexPageSize {
			return []proto.SecurityBar{dayBar(2024, 1, 1), dayBar(2024, 1, 2)}, nil
		}
		if start == 2 && count == indexPageSize {
			return nil, fmt.Errorf("invalid kline datetime: 1376385400")
		}
		if start == 2 && count == indexFallbackPageSize {
			return nil, fmt.Errorf("invalid kline datetime: 1376385400")
		}
		return nil, fmt.Errorf("unexpected start=%d count=%d", start, count)
	}

	startDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	got, err := IndexKLineRange(4, 1, "000001", startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("bars = %d, want 2", len(got))
	}
	// 第 2 页：先 798 失败，再 400 失败，截断
	if len(calls) < 3 {
		t.Fatalf("calls = %#v, want fallback retry", calls)
	}
	if calls[1].count != indexPageSize || calls[2].count != indexFallbackPageSize {
		t.Fatalf("fallback sequence = %#v", calls)
	}
}

func TestExKLineRangeAdvancesByActualShortPage(t *testing.T) {
	prev := fetchExKLinePage
	t.Cleanup(func() { fetchExKLinePage = prev })

	var starts []uint32
	fetchExKLinePage = func(category uint8, code string, period uint16, start uint32, count uint16, times uint16) ([]proto.ExKLineItem, error) {
		starts = append(starts, start)
		_ = category
		_ = code
		_ = period
		_ = count
		_ = times
		switch start {
		case 0:
			// 短页 700 语义：只返回 2 条模拟
			return []proto.ExKLineItem{
				{DateTime: "2024-06-01 15:00:00", Open: 1, High: 1, Low: 1, Close: 1},
				{DateTime: "2024-06-02 15:00:00", Open: 2, High: 2, Low: 2, Close: 2},
			}, nil
		case 2:
			return []proto.ExKLineItem{
				{DateTime: "2023-01-01 15:00:00", Open: 3, High: 3, Low: 3, Close: 3},
				{DateTime: "2023-01-02 15:00:00", Open: 4, High: 4, Low: 4, Close: 4},
			}, nil
		default:
			return nil, nil
		}
	}

	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.Local)
	got, err := ExKLineRange(31, "01810", 4, 1, startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) < 2 || starts[1] != 2 {
		t.Fatalf("starts = %v, want second page at actual len 2 (not klinePageSize)", starts)
	}
	if len(got) != 4 {
		t.Fatalf("bars = %d, want 4", len(got))
	}
}

func TestStockKLineRangeUsesPaginate(t *testing.T) {
	prev := fetchStockKLinePage
	t.Cleanup(func() { fetchStockKLinePage = prev })

	var starts []uint16
	fetchStockKLinePage = func(category uint16, market uint8, code string, start, count uint16, times, adjust uint16) ([]proto.SecurityBar, error) {
		starts = append(starts, start)
		if start == 0 {
			return []proto.SecurityBar{dayBar(2024, 1, 10), dayBar(2024, 1, 11)}, nil
		}
		if start == 2 {
			return []proto.SecurityBar{dayBar(2023, 6, 1), dayBar(2023, 6, 2)}, nil
		}
		return nil, nil
	}

	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.Local)
	got, err := StockKLineRange(4, 1, "600000", 1, 0, startDate, endDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(starts) < 2 || starts[1] != 2 {
		t.Fatalf("starts = %v, want advance by actual page len", starts)
	}
	if len(got) != 4 {
		t.Fatalf("bars = %d, want 4", len(got))
	}
}
