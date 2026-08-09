// 本文件测试旧 /api/ex/history-tick 接口：分时构建、昨收解析与错误处理。
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

	"KlineChartQuantGo/services/tdx-api/internal/domain"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

// 验证扩展行情历史分时返回统一响应结构。
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
	preCloseSource := domain.ExPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("realtime quote must not be called for historical date")
		},
		DailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
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
	if body.Data[0].Price != 18.6 || body.Data[0].Avg != 18.55 {
		t.Fatalf("first tick = %#v", body.Data[0])
	}
	if body.Data[0].Volume != nil || body.Data[0].Amount != nil {
		t.Fatalf("first tick metrics = %#v, want no unverified volume or amount", body.Data[0])
	}
}

// 验证扩展行情全零分时模板被识别为无历史数据。
func TestExHistoryTickRejectsAllZeroUpstreamTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fetchTick := func(uint32, uint8, string) ([]proto.ExTickChartData, error) {
		return []proto.ExTickChartData{
			{Time: "09:30"},
			{Time: "09:31"},
		}, nil
	}
	preCloseSource := domain.ExPreCloseSource{
		Now: func() time.Time { return time.Now() },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("preClose must not be resolved for unavailable tick data")
		},
		DailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
			return nil, errors.New("preClose must not be resolved for unavailable tick data")
		},
	}

	router := gin.New()
	router.POST("/api/ex/history-tick", func(c *gin.Context) {
		handleExHistoryTickWithDeps(c, fetchTick, preCloseSource)
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/ex/history-tick",
		bytes.NewBufferString(`{"date":20250908,"category":31,"code":"00700"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if body.Error != "该日期暂无历史分时数据" {
		t.Fatalf("error = %q, want explicit unavailable-data message", body.Error)
	}
}

// 验证扩展行情昨收无法解析时返回网关错误。
func TestExHistoryTickReturnsBadGatewayWhenBaselineCannotBeResolved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fetchTick := func(uint32, uint8, string) ([]proto.ExTickChartData, error) {
		return []proto.ExTickChartData{{Time: "09:30", Price: 1, Avg: 1, Vol: 1}}, nil
	}
	loc := time.FixedZone("CST", 8*60*60)
	preCloseSource := domain.ExPreCloseSource{
		Now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
		Quote: func(uint8, string) (float64, error) {
			return 0, errors.New("quote unavailable")
		},
		// 当日 quote 失败后回退日线；日线也失败才 502
		DailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
			return nil, errors.New("daily bar unavailable")
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
	if !strings.Contains(resp.Body.String(), "daily bar unavailable") {
		t.Fatalf("response = %s, want daily-bar fallback error", resp.Body.String())
	}
}
