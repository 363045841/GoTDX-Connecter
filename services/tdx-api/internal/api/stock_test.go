package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
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
	if indexFallbackPageSize == 0 || indexFallbackPageSize >= indexPageSize {
		t.Fatalf("indexFallbackPageSize = %d invalid relative to indexPageSize", indexFallbackPageSize)
	}
}

func TestStockHistoryTickResponseIncludesPreClose(t *testing.T) {
	response := newStockHistoryTickResponse(8.3, []stockHistoryTickItem{{
		Timestamp: "2026-07-27T09:30:00+08:00",
		Price:     8.5,
		Avg:       8.5,
		Vol:       100,
	}})

	if response.PreClose != 8.3 {
		t.Fatalf("preClose = %v, want 8.3", response.PreClose)
	}
	if len(response.Data) != 1 || response.Data[0].Price != 8.5 {
		t.Fatalf("data = %#v, want one 8.5 tick", response.Data)
	}
}

func TestResolveTimeSharePreCloseUsesTargetDailyBarForHistoricalDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	source := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called for a historical date")
		},
		dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			if market != 1 || code != "600519" || date.Format("20060102") != "20260724" {
				t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
			}
			return []proto.SecurityBar{{
				DateTime: date,
				Last:     1418.5,
				Close:    1432.8,
			}}, nil
		},
	}

	got, err := resolveTimeSharePreClose(1, "600519", 20260724, source)
	if err != nil {
		t.Fatalf("resolve historical preClose: %v", err)
	}
	if got != 1418.5 {
		t.Fatalf("preClose = %v, want 1418.5", got)
	}
}

func TestResolveTimeSharePreCloseRejectsZeroRealtimeBaseline(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	source := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, nil
		},
		dailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
			return nil, errors.New("daily bars must not be called for the current date")
		},
	}

	if _, err := resolveTimeSharePreClose(0, "000001", 20260727, source); err == nil {
		t.Fatal("resolve current preClose succeeded with zero baseline")
	}
}

func TestStockHistoryTickReturnsBadGatewayWhenBaselineCannotBeResolved(t *testing.T) {
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	loc := time.FixedZone("CST", 8*60*60)
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("quote unavailable")
		},
		dailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
			return nil, errors.New("unexpected daily bar request")
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20260727,"market":0,"code":"000001"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "quote unavailable") {
		t.Fatalf("response = %s, want explicit upstream error", resp.Body.String())
	}
}
