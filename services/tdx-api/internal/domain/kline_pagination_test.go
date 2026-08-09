// 本文件测试股票、指数和扩展行情K线的分页及日期范围处理逻辑。
package domain

import (
	"fmt"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

// dayBar 创建指定日期的日K测试数据。
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

// 验证K线分页按实际返回条数推进起始位置。
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

// 验证分页到达早于查询起始日的数据后停止请求。
func TestPaginateFromRecentStopsWhenOldestBeforeStartDate(t *testing.T) {
	var starts []uint32
	pages := map[uint32][]proto.SecurityBar{
		0: {dayBar(2024, 6, 1), dayBar(2024, 6, 2)},
		2: {dayBar(2023, 1, 1), dayBar(2023, 1, 2)}, // 最旧已早于 startDate
		4: {dayBar(2022, 1, 1)},                     // 不应再请求
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

// 验证允许部分数据时保留已取得的K线分页结果。
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

// 验证指数深页请求失败时缩小页大小并返回部分数据。
func TestIndexKLineRangeUsesFallbackCountThenPartial(t *testing.T) {
	prev := FetchIndexBarsPage
	t.Cleanup(func() { FetchIndexBarsPage = prev })

	type call struct {
		start, count uint16
	}
	var calls []call
	FetchIndexBarsPage = func(category uint16, market uint8, code string, start, count uint16) ([]proto.SecurityBar, error) {
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

// 验证恶意/异常 fetch 持续返回新页时 paginateFromRecent 会被页数硬上限截断，不会无限追加内存。
func TestPaginateFromRecentSafetyCap(t *testing.T) {
	pages := 0
	startDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	got, err := paginateFromRecent(798, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		pages++
		// 永远返回比 startDate 新的数据，正常终止条件永不满足
		return []proto.SecurityBar{dayBar(2024, 1, 1)}, nil
	}, securityBarOldest, startDate, false)
	if err != nil {
		t.Fatal(err)
	}
	if pages != maxKLinePages {
		t.Fatalf("pages = %d, want capped at %d", pages, maxKLinePages)
	}
	if len(got) != maxKLinePages {
		t.Fatalf("bars = %d, want %d (not unbounded growth)", len(got), maxKLinePages)
	}
}

// 验证扩展行情短页K线按实际条数继续分页。
func TestExKLineRangeAdvancesByActualShortPage(t *testing.T) {
	prev := FetchExKLinePage
	t.Cleanup(func() { FetchExKLinePage = prev })

	var starts []uint32
	FetchExKLinePage = func(category uint8, code string, period uint16, start uint32, count uint16, times uint16) ([]proto.ExKLineItem, error) {
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
				{DateTime: time.Date(2024, 6, 1, 15, 0, 0, 0, time.Local), Open: 1, High: 1, Low: 1, Close: 1},
				{DateTime: time.Date(2024, 6, 2, 15, 0, 0, 0, time.Local), Open: 2, High: 2, Low: 2, Close: 2},
			}, nil
		case 2:
			return []proto.ExKLineItem{
				{DateTime: time.Date(2023, 1, 1, 15, 0, 0, 0, time.Local), Open: 3, High: 3, Low: 3, Close: 3},
				{DateTime: time.Date(2023, 1, 2, 15, 0, 0, 0, time.Local), Open: 4, High: 4, Low: 4, Close: 4},
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

// 验证股票日K查询使用通用分页逻辑。
func TestStockKLineRangeUsesPaginate(t *testing.T) {
	prev := FetchStockKLinePage
	t.Cleanup(func() { FetchStockKLinePage = prev })

	var starts []uint16
	FetchStockKLinePage = func(category uint16, market uint8, code string, start, count uint16, times, adjust uint16) ([]proto.SecurityBar, error) {
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
