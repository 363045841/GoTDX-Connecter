// 本文件测试旧 /api/stock/history-tick 接口：分时构建、昨收解析与响应形状。
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/domain"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

// 验证分时统一响应包含昨收与数据点。
func TestStockHistoryTickResponseIncludesPreClose(t *testing.T) {
	volume := 100
	response := newStockHistoryTickResponse(8.3, []stockHistoryTickItem{{
		Timestamp: "2026-07-27T09:30:00+08:00",
		Price:     8.5,
		Avg:       8.5,
		Volume:    &volume,
	}})

	if response.PreClose != 8.3 {
		t.Fatalf("preClose = %v, want 8.3", response.PreClose)
	}
	if len(response.Data) != 1 || response.Data[0].Price != 8.5 {
		t.Fatalf("data = %#v, want one 8.5 tick", response.Data)
	}
}

// 验证指数目标日K线缺失昨收时返回网关错误。
func TestStockHistoryTickIndexRejectsMissingPreCloseOnDailyBar(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := domain.StockPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		DailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
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

// 验证指数历史分时使用 gotdx 日K昨收字段。
func TestStockHistoryTickIndexUsesGotdxPreClose(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	preCloseSource := domain.StockPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called")
		},
		DailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
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

// 验证分时昨收无法解析时返回网关错误。
func TestStockHistoryTickReturnsBadGatewayWhenBaselineCannotBeResolved(t *testing.T) {
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{}, nil
	}
	loc := time.FixedZone("CST", 8*60*60)
	preCloseSource := domain.StockPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 27, 10, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("quote unavailable")
		},
		DailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
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

// 验证当日股票分时补入 09:30 开盘首笔。
func TestStockHistoryTickPrependsOpeningTradeOnCurrentDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	original := domain.FetchOpeningTrade
	t.Cleanup(func() { domain.FetchOpeningTrade = original })

	var sawDate uint32
	domain.FetchOpeningTrade = func(date uint32, market uint8, code string, now time.Time) (domain.OpeningTrade, bool) {
		sawDate = date
		if market != 1 || code != "601360" {
			t.Fatalf("opening trade request = %d/%s", market, code)
		}
		if now.Format("20060102") != "20260729" {
			t.Fatalf("now = %s, want current trading day", now.Format("20060102"))
		}
		return domain.OpeningTrade{Price: 8.95, Vol: 1200}, true
	}

	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{{Price: 8.97, Avg: 8.86, Vol: 49146}}, nil
	}
	preCloseSource := domain.StockPreCloseSource{
		Now:   func() time.Time { return time.Date(2026, 7, 29, 11, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) { return 8.68, nil },
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
	if body.Data[0].Price != 8.95 || body.Data[0].Volume == nil || *body.Data[0].Volume != 1200 {
		t.Fatalf("opening tick = %#v, want price 8.95 Volume 1200", body.Data[0])
	}
	if body.Data[0].Amount != nil {
		t.Fatalf("opening Amount = %#v, want nil", body.Data[0].Amount)
	}
	if body.Data[1].Timestamp != "2026-07-29T09:31:00+08:00" || body.Data[1].Price != 8.97 || body.Data[1].Volume == nil || *body.Data[1].Volume != 47946 {
		t.Fatalf("second tick = %#v, want 09:31 price 8.97 Volume 47946", body.Data[1])
	}
}

// 验证指数开盘成交额换算为万元，并从 09:31 合计值中扣除以避免重复。
func TestStockHistoryTickNormalizesIndexOpeningAmount(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	original := domain.FetchOpeningTrade
	t.Cleanup(func() { domain.FetchOpeningTrade = original })

	domain.FetchOpeningTrade = func(uint32, uint8, string, time.Time) (domain.OpeningTrade, bool) {
		return domain.OpeningTrade{Price: 3812.11, Vol: 69728381}, true
	}
	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{{Price: 3826.24, Avg: 3817.4716, Vol: 2918209}}, nil
	}
	preCloseSource := domain.StockPreCloseSource{
		Now:   func() time.Time { return time.Date(2026, 7, 30, 11, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) { return 3828.47, nil },
	}

	router := gin.New()
	router.POST("/api/stock/history-tick", func(c *gin.Context) {
		handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/stock/history-tick",
		bytes.NewBufferString(`{"date":20260730,"market":1,"code":"000001"}`),
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
	if len(body.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(body.Data))
	}
	if body.Data[0].Amount == nil || *body.Data[0].Amount != 6_972_838_100 {
		t.Fatalf("09:30 Amount = %#v, want 6972838100 元", body.Data[0].Amount)
	}
	if body.Data[0].Volume != nil {
		t.Fatalf("09:30 Volume = %#v, want nil", body.Data[0].Volume)
	}
	if body.Data[1].Amount == nil || *body.Data[1].Amount != 22_209_251_900 {
		t.Fatalf("09:31 Amount = %#v, want 22209251900 元", body.Data[1].Amount)
	}
	if body.Data[1].Volume != nil {
		t.Fatalf("09:31 Volume = %#v, want nil", body.Data[1].Volume)
	}
}

// 验证历史股票分时补入对应日期的 09:30 开盘首笔。
func TestStockHistoryTickPrependsOpeningTradeOnHistoricalDate(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	original := domain.FetchOpeningTrade
	t.Cleanup(func() { domain.FetchOpeningTrade = original })

	domain.FetchOpeningTrade = func(date uint32, market uint8, code string, now time.Time) (domain.OpeningTrade, bool) {
		if date != 20260728 || market != 1 || code != "601360" {
			t.Fatalf("opening trade request = %d/%d/%s", date, market, code)
		}
		if now.Format("20060102") != "20260729" {
			t.Fatalf("now = %s, want 20260729 for historical branch", now.Format("20060102"))
		}
		return domain.OpeningTrade{Price: 8.5, Vol: 800}, true
	}

	fetchTick := func(uint32, uint8, string) ([]proto.HistoryMinuteTimeData, error) {
		return []proto.HistoryMinuteTimeData{{Price: 8.6, Avg: 8.55, Vol: 100}}, nil
	}
	preCloseSource := domain.StockPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 29, 11, 0, 0, 0, loc) },
		DailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
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

// 验证指定品种与指数均可获取历史分时及目标日昨收。
func TestStockHistoryTickSupportsSpecifiedProductsAndIndexes(t *testing.T) {
	previous := domain.FetchOpeningTrade
	t.Cleanup(func() { domain.FetchOpeningTrade = previous })
	domain.FetchOpeningTrade = func(uint32, uint8, string, time.Time) (domain.OpeningTrade, bool) {
		return domain.OpeningTrade{}, false
	}

	loc := time.FixedZone("CST", 8*60*60)
	tests := []struct {
		name     string
		market   uint8
		code     string
		preClose float64
	}{
		// 东海证券（832970）：验证新三板历史分时与目标日日线昨收。
		{name: "new third board Donghai Securities", market: 0, code: "832970", preClose: 10.01},
		// 国泰中证同业存单AAA指数7天持有期（015825）：验证基金历史分时与目标日日线昨收。
		{name: "fund Guotai CD AAA", market: 1, code: "015825", preClose: 100.01},
		// 国泰海通砂之船商业REIT（508602）：验证REITs历史分时与目标日日线昨收。
		{name: "REIT Guotai Haitong Sand Ship", market: 1, code: "508602", preClose: 5.01},
		// 上证指数（000001）：验证上海指数历史分时与目标日日线昨收。
		{name: "Shanghai Composite", market: 1, code: "000001", preClose: 3500.01},
		// 深证成指（399001）：验证深圳指数历史分时与目标日日线昨收。
		{name: "Shenzhen Component", market: 0, code: "399001", preClose: 11000.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchTick := func(date uint32, market uint8, code string) ([]proto.HistoryMinuteTimeData, error) {
				if date != 20260724 || market != tt.market || code != tt.code {
					t.Fatalf("history tick request = %d/%d/%s", date, market, code)
				}
				return []proto.HistoryMinuteTimeData{{Price: tt.preClose + 0.01, Avg: tt.preClose + 0.01, Vol: 100}}, nil
			}
			preCloseSource := domain.StockPreCloseSource{
				Now: func() time.Time { return time.Date(2026, 7, 25, 10, 0, 0, 0, loc) },
				DailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
					if market != tt.market || code != tt.code || date.Format("20060102") != "20260724" {
						t.Fatalf("daily bar request = %d/%s/%s", market, code, date.Format("20060102"))
					}
					return []proto.SecurityBar{{DateTime: date, PreClose: tt.preClose}}, nil
				},
			}
			router := gin.New()
			router.POST("/api/stock/history-tick", func(c *gin.Context) {
				handleStockHistoryTickWithDeps(c, fetchTick, preCloseSource)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/stock/history-tick", bytes.NewBufferString(`{"date":20260724,"market":`+strconv.Itoa(int(tt.market))+`,"code":"`+tt.code+`"}`))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
			}
			var body stockHistoryTickResponse
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.PreClose != tt.preClose || len(body.Data) != 1 || body.Data[0].Price != tt.preClose+0.01 {
				t.Fatalf("response = %#v, want preClose=%v and one tick", body, tt.preClose)
			}
		})
	}
}
