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

func TestFilterExKLineByDateKeepsBarsInRange(t *testing.T) {
	loc := time.Local
	klines := []proto.ExKLineItem{
		{DateTime: time.Date(2026, 7, 22, 15, 0, 0, 0, loc), Open: 1, High: 1, Low: 1, Close: 1},
		{DateTime: time.Date(2026, 7, 23, 15, 0, 0, 0, loc), Open: 2, High: 2, Low: 2, Close: 2},
		{DateTime: time.Date(2026, 7, 23, 15, 0, 0, 0, loc), Open: 2, High: 2, Low: 2, Close: 2}, // dup
		{DateTime: time.Date(2026, 7, 24, 15, 0, 0, 0, loc), Open: 3, High: 3, Low: 3, Close: 3},
		{DateTime: time.Date(2026, 7, 25, 15, 0, 0, 0, loc), Open: 4, High: 4, Low: 4, Close: 4},
	}

	got := filterExKLineByDate(
		klines,
		time.Date(2026, 7, 23, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 24, 0, 0, 0, 0, loc),
	)

	if len(got) != 2 {
		t.Fatalf("filtered bars = %d, want 2; got %#v", len(got), got)
	}
	if got[0].DateTime.Day() != 23 || got[1].DateTime.Day() != 24 {
		t.Fatalf("dates = %s, %s; want ascending 23 then 24", got[0].DateTime, got[1].DateTime)
	}
}

func TestResolveExTimeSharePreCloseUsesQuoteOnCurrentDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	source := exTimeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		quote: func(category uint8, code string) (float64, error) {
			if category != 31 || code != "01810" {
				t.Fatalf("quote request = %d/%s", category, code)
			}
			return 18.5, nil
		},
		dailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
			return nil, errors.New("daily bars must not be called for the current date")
		},
	}

	got, err := resolveExTimeSharePreClose(31, "01810", 20260728, source)
	if err != nil {
		t.Fatalf("resolve current preClose: %v", err)
	}
	if got != 18.5 {
		t.Fatalf("preClose = %v, want 18.5", got)
	}
}

func TestResolveExTimeSharePreCloseUsesTargetDailyBarForHistoricalDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	source := exTimeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called for a historical date")
		},
		dailyBars: func(category uint8, code string, date time.Time) ([]proto.ExKLineItem, error) {
			if category != 31 || code != "01810" || date.Format("20060102") != "20260724" {
				t.Fatalf("daily bar request = %d/%s/%s", category, code, date.Format("20060102"))
			}
			return []proto.ExKLineItem{{
				DateTime: time.Date(2026, 7, 24, 15, 0, 0, 0, loc),
				PreClose: 18.0,
				Open:     18.2,
				Close:    18.8,
			}}, nil
		},
	}

	got, err := resolveExTimeSharePreClose(31, "01810", 20260724, source)
	if err != nil {
		t.Fatalf("resolve historical preClose: %v", err)
	}
	if got != 18.0 {
		t.Fatalf("preClose = %v, want 18.0", got)
	}
}

func TestExHistoryTickReturnsUnifiedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fetchTick := func(date uint32, category uint8, code string) ([]proto.ExTickChartData, error) {
		if date != 20260724 || category != 31 || code != "01810" {
			t.Fatalf("tick request = %d/%d/%s", date, category, code)
		}
		return []proto.ExTickChartData{
			{Time: "09:30", Price: 18.6, Avg: 18.55, Vol: 100},
			{Time: "09:31", Price: 18.7, Avg: 18.6, Vol: 200},
		}, nil
	}
	loc := time.FixedZone("CST", 8*60*60)
	preCloseSource := exTimeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called for historical date")
		},
		dailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
			return []proto.ExKLineItem{{
				DateTime: time.Date(2026, 7, 24, 15, 0, 0, 0, loc),
				Open:     18.5,
				Close:    18.9,
			}}, nil
		},
	}

	router := gin.New()
	router.POST("/api/ex/history-tick", func(c *gin.Context) {
		handleExHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/ex/history-tick",
		bytes.NewBufferString(`{"date":20260724,"category":31,"code":"01810"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var body stockHistoryTickResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if body.PreClose != 18.5 {
		t.Fatalf("preClose = %v, want 18.5", body.PreClose)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(body.Data))
	}
	if body.Data[0].Timestamp != "2026-07-24T09:30:00+08:00" {
		t.Fatalf("first timestamp = %q, want 2026-07-24T09:30:00+08:00", body.Data[0].Timestamp)
	}
	if body.Data[0].Price != 18.6 || body.Data[0].Avg != 18.55 || body.Data[0].Vol != 100 {
		t.Fatalf("first tick = %#v", body.Data[0])
	}
}

func TestExHistoryTickReturnsBadGatewayWhenBaselineCannotBeResolved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fetchTick := func(uint32, uint8, string) ([]proto.ExTickChartData, error) {
		return []proto.ExTickChartData{{Time: "09:30", Price: 1, Avg: 1, Vol: 1}}, nil
	}
	loc := time.FixedZone("CST", 8*60*60)
	preCloseSource := exTimeSharePreCloseSource{
		now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		quote: func(uint8, string) (float64, error) {
			return 0, errors.New("quote unavailable")
		},
		dailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
			return nil, errors.New("unexpected daily bar request")
		},
	}

	router := gin.New()
	router.POST("/api/ex/history-tick", func(c *gin.Context) {
		handleExHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/ex/history-tick",
		bytes.NewBufferString(`{"date":20260728,"category":31,"code":"01810"}`),
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
