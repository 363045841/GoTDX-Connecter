// 本文件测试证券目录搜索 HTTP 接口的参数校验与路由注册。
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/directory"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

// fakeDirectoryLoader 为搜索测试提供可注入的证券目录加载器。
type fakeDirectoryLoader struct {
	stocks map[uint8][]proto.Security
}

func (l *fakeDirectoryLoader) StockAll(market uint8) ([]proto.Security, error) {
	return l.stocks[market], nil
}

func (l *fakeDirectoryLoader) ExCount() (uint32, error) {
	return 0, nil
}

func (l *fakeDirectoryLoader) ExList(uint32, uint16) ([]proto.ExListItem, error) {
	return nil, nil
}

// 验证符号搜索路由已注册并校验请求参数。
func TestSymbolSearchHandlerValidatesRequestAndIsRegistered(t *testing.T) {
	loader := &fakeDirectoryLoader{stocks: map[uint8][]proto.Security{0: {{Code: "000001", Name: "Ping An"}}}}
	cache := directory.NewCache(loader, nil, 24*time.Hour)
	router := newRouter(cache)
	for _, body := range []string{"{", `{"query":"   "}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/symbol/search", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("POST /api/symbol/search %s = %d, want 400", body, resp.Code)
		}
	}

	handlerRouter := gin.New()
	handlerRouter.POST("/api/symbol/search", newSymbolSearchHandler(cache))
	req := httptest.NewRequest(http.MethodPost, "/api/symbol/search", bytes.NewBufferString(`{"query":"000001","limit":101}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handlerRouter.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var items []directory.Item
	if err := json.Unmarshal(resp.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}
