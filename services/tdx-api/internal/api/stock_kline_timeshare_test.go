// 本文件测试股票和指数日K、历史分时及昨收解析逻辑。
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

	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

// 验证指数日K按日期排序并去除重复K线。
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

// 验证指数日K分页大小不超过上游接口限制。
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

// 验证分时统一响应包含昨收与数据点。
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

// 验证历史股票分时使用目标日K线昨收。
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

// 验证当日实时昨收为零时拒绝无效基准。
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

// 验证指数目标日K线缺失昨收时返回网关错误。
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

// 验证指数历史分时使用 gotdx 日K昨收字段。
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

// 验证分时昨收无法解析时返回网关错误。
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

// 验证当日股票分时补入 09:30 开盘首笔。
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

// 验证历史股票分时补入对应日期的 09:30 开盘首笔。
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

// 验证新三板、基金和 REITs 日K透传对应复权参数。
func TestStockKLineRangePassesAdjustForSpecifiedSecurityTypes(t *testing.T) {
	previous := fetchStockKLinePage
	t.Cleanup(func() { fetchStockKLinePage = previous })

	loc := time.FixedZone("CST", 8*60*60)
	startDate := time.Date(2026, 7, 24, 0, 0, 0, 0, loc)
	endDate := startDate
	tests := []struct {
		name   string
		code   string
		adjust uint16
	}{
		// 东海证券（832970）：新三板日K前复权。
		{name: "new third board forward adjusted", code: "832970", adjust: 0},
		// 国泰中证同业存单AAA指数7天持有期（015825）：基金日K不复权。
		{name: "fund unadjusted", code: "015825", adjust: 1},
		// 国泰海通砂之船商业REIT（508602）：REITs日K后复权。
		{name: "REIT backward adjusted", code: "508602", adjust: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchStockKLinePage = func(category uint16, market uint8, code string, start, count uint16, times, adjust uint16) ([]proto.SecurityBar, error) {
				if category != 4 || market != 1 || code != tt.code || times != 1 || adjust != tt.adjust {
					t.Fatalf("StockKLine request = category=%d market=%d code=%s times=%d adjust=%d", category, market, code, times, adjust)
				}
				return []proto.SecurityBar{{DateTime: startDate, Close: 10}}, nil
			}

			bars, err := StockKLineRange(4, 1, tt.code, 1, tt.adjust, startDate, endDate)
			if err != nil {
				t.Fatalf("StockKLineRange: %v", err)
			}
			if len(bars) != 1 || bars[0].Close != 10 {
				t.Fatalf("bars = %#v, want one daily bar", bars)
			}
		})
	}
}

// 验证上证指数和深证成指使用对应市场的指数日K接口。
func TestIndexKLineRangeUsesSpecifiedIndexCodes(t *testing.T) {
	previous := fetchIndexBarsPage
	t.Cleanup(func() { fetchIndexBarsPage = previous })

	loc := time.FixedZone("CST", 8*60*60)
	startDate := time.Date(2026, 7, 24, 0, 0, 0, 0, loc)
	endDate := startDate
	tests := []struct {
		name   string
		market uint8
		code   string
	}{
		// 上证指数（000001）使用上海市场指数日K接口。
		{name: "Shanghai Composite", market: 1, code: "000001"},
		// 深证成指（399001）使用深圳市场指数日K接口。
		{name: "Shenzhen Component", market: 0, code: "399001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchIndexBarsPage = func(category uint16, market uint8, code string, start, count uint16) ([]proto.SecurityBar, error) {
				if category != 4 || market != tt.market || code != tt.code {
					t.Fatalf("GetIndexBars request = category=%d market=%d code=%s", category, market, code)
				}
				return []proto.SecurityBar{{DateTime: startDate, Close: 3000}}, nil
			}

			bars, err := IndexKLineRange(4, tt.market, tt.code, startDate, endDate)
			if err != nil {
				t.Fatalf("IndexKLineRange: %v", err)
			}
			if len(bars) != 1 || bars[0].Close != 3000 {
				t.Fatalf("bars = %#v, want one daily bar", bars)
			}
		})
	}
}

// 验证指定品种与指数均可获取历史分时及目标日昨收。
func TestStockHistoryTickSupportsSpecifiedProductsAndIndexes(t *testing.T) {
	previous := fetchOpeningTrade
	t.Cleanup(func() { fetchOpeningTrade = previous })
	fetchOpeningTrade = func(uint32, uint8, string, time.Time) (openingTrade, bool) {
		return openingTrade{}, false
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
			preCloseSource := timeSharePreCloseSource{
				now: func() time.Time { return time.Date(2026, 7, 25, 10, 0, 0, 0, loc) },
				dailyBars: func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
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
