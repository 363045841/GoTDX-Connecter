// 本文件测试统一行情 REST V1 的路由与协议响应。
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/gin-gonic/gin"
)

// v1TestRouter 创建不依赖真实上游网络的 V1 路由。
func v1TestRouter() *gin.Engine {
	now := time.Now()
	cache := &symbolDirectoryCache{
		entries: []symbolSearchItem{{
			Symbol: "600519", Description: "贵州茅台", Exchange: "SH", Source: v1SourceID,
			Params: map[string]any{"market": uint8(1), "kind": symbolKindStock},
		}},
		loaded: true, loadedAt: now, ttl: time.Hour, now: func() time.Time { return now },
	}
	return newRouterWithStatus(cache, func() client.Status { return client.Status{Ready: true} })
}

// 验证 V1 probe 返回标准 envelope 和在线状态。
func TestV1Probe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	v1TestRouter().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/market-data/sources/gotdx/probe", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Status != "online" || body.RequestID == "" {
		t.Fatalf("response = %#v, want online status and request ID", body)
	}
}

// 验证 V1 搜索将 GOTDX 目录条目归一化为 InstrumentDescriptor。
func TestV1SearchReturnsInstrumentDescriptor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/market-data/instruments/search", bytes.NewBufferString(`{"sourceId":"gotdx","keyword":"茅台","limit":20}`))
	req.Header.Set("Content-Type", "application/json")
	v1TestRouter().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Data struct {
			Items []v1Instrument `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Items) != 1 {
		t.Fatalf("items = %#v, want one instrument", body.Data.Items)
	}
	item := body.Data.Items[0]
	if item.ID != "gotdx:stock:1:600519" || item.SessionID != "CN" || item.AssetClass != symbolKindStock {
		t.Fatalf("instrument = %#v, want normalized GOTDX stock", item)
	}
	if item.Capabilities.Bars == nil || len(item.Capabilities.Bars.Periods) != len(v1PeriodOrder) {
		t.Fatalf("capabilities = %#v, want K-line periods", item.Capabilities)
	}
}

// 验证 V1 K 线在调用上游前拒绝未声明周期。
func TestV1BarsRejectsUnsupportedPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/market-data/bars", bytes.NewBufferString(`{"sourceId":"gotdx","instrument":{"id":"gotdx:stock:1:600519","symbol":"600519","exchange":"SH","providerRef":{"market":1}},"period":"invalid","adjustment":"none","from":1,"to":2}`))
	req.Header.Set("Content-Type", "application/json")
	v1TestRouter().ServeHTTP(resp, req)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

// 验证 V1 分时在调用上游前拒绝非法交易日。
func TestV1TimeShareRejectsInvalidTradingDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/market-data/timeshare", bytes.NewBufferString(`{"sourceId":"gotdx","instrument":{"id":"gotdx:stock:1:600519","symbol":"600519","exchange":"SH","providerRef":{"market":1}},"tradingDate":"2026-02-30"}`))
	req.Header.Set("Content-Type", "application/json")
	v1TestRouter().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}
