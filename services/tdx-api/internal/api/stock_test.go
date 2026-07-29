package api

import (
	"bytes"
	"encoding/json"
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
				PreClose: 1418.5,
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

func TestStockHistoryTickIndexRejectsMissingPreCloseOnDailyBar(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			if market != 0 || code != "399001" || date.Format("20060102") != "20241213" {
				t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
			}
			return []proto.SecurityBar{{
				Close:    10713.07,
				Year:     2024,
				Month:    12,
				Day:      13,
				DateTime: time.Date(2024, 12, 13, 15, 0, 0, 0, loc),
			}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20241213,"market":0,"code":"399001","kind":"index"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "invalid historical preClose") {
		t.Fatalf("response = %s, want invalid historical preClose", resp.Body.String())
	}
}

func TestStockHistoryTickIndexUsesGotdxPreClose(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
			if market != 0 || code != "399001" || date.Format("20060102") != "20241213" {
				t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
			}
			return []proto.SecurityBar{{
				PreClose:  10957.13,
				LastClose: 10957.13,
				Close:     10713.07,
				Year:      2024,
				Month:     12,
				Day:       13,
				DateTime:  time.Date(2024, 12, 13, 15, 0, 0, 0, loc),
			}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20241213,"market":0,"code":"399001","kind":"index"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"preClose":10957.13`) {
		t.Fatalf("response = %s, want gotdx PreClose 10957.13", resp.Body.String())
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

func TestStockHistoryTickPrependsOpeningTradeOnCurrentDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	original := fetchOpeningTrade
	t.Cleanup(func() { fetchOpeningTrade = original })

	var sawDate uint32
	fetchOpeningTrade = func(date uint32, market uint8, code string, now time.Time) (openingTrade, bool) {
		sawDate = date
		if market != 1 || code != "601360" {
			t.Fatalf("opening trade request = %d/%s", market, code)
		}
		if now.Format("20060102") != "20260729" {
			t.Fatalf("now = %s, want current trading day", now.Format("20060102"))
		}
		return openingTrade{Price: 8.95, Vol: 1200}, true
	}

	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{{Price: 8.97, Avg: 8.86, Vol: 49146}}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now:   func() time.Time { return time.Date(2026, 7, 29, 11, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) { return 8.68, nil },
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20260729,"market":1,"code":"601360"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if sawDate != 20260729 {
		t.Fatalf("opening trade date = %d, want 20260729", sawDate)
	}
	var body stockHistoryTickResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if len(body.Data) != 2 {
		t.Fatalf("data len = %d, want 2 (09:30 + first minute)", len(body.Data))
	}
	if body.Data[0].Timestamp != "2026-07-29T09:30:00+08:00" {
		t.Fatalf("first timestamp = %q, want 09:30", body.Data[0].Timestamp)
	}
	if body.Data[0].Price != 8.95 || body.Data[0].Vol != 1200 {
		t.Fatalf("opening tick = %#v, want price 8.95 vol 1200", body.Data[0])
	}
	if body.Data[1].Timestamp != "2026-07-29T09:31:00+08:00" || body.Data[1].Price != 8.97 {
		t.Fatalf("second tick = %#v, want 09:31 price 8.97", body.Data[1])
	}
}

func TestStockHistoryTickPrependsOpeningTradeOnHistoricalDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	original := fetchOpeningTrade
	t.Cleanup(func() { fetchOpeningTrade = original })

	fetchOpeningTrade = func(date uint32, market uint8, code string, now time.Time) (openingTrade, bool) {
		if date != 20260728 || market != 1 || code != "601360" {
			t.Fatalf("opening trade request = %d/%d/%s", date, market, code)
		}
		if now.Format("20060102") != "20260729" {
			t.Fatalf("now = %s, want 20260729 for historical branch", now.Format("20060102"))
		}
		return openingTrade{Price: 8.5, Vol: 800}, true
	}

	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{{Price: 8.6, Avg: 8.55, Vol: 100}}, nil
	}
	preCloseSource := timeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 29, 11, 0, 0, 0, loc) },
		dailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
			return []proto.SecurityBar{{PreClose: 8.4, Close: 8.7}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20260728,"market":1,"code":"601360"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"timestamp":"2026-07-28T09:30:00+08:00"`) {
		t.Fatalf("response = %s, want historical 09:30", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"Price":8.5`) {
		t.Fatalf("response = %s, want opening price 8.5", resp.Body.String())
	}
}


