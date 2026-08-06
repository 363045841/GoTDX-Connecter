// 本文件测试统一行情 V1 将复用的纯领域服务。
package api

import (
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

// 验证股票分时构建服务可脱离 Gin 生成统一昨收响应。
func TestBuildStockHistoryTickResponse(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	response, err := buildStockHistoryTickResponse(
		stockHistoryTickRequest{Date: 20260724, Market: 1, Code: "600519", Kind: symbolKindStock},
		nil,
		timeSharePreCloseSource{
			now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
			dailyBars: func(uint8, string, time.Time) ([]proto.SecurityBar, error) {
				return []proto.SecurityBar{{PreClose: 1418.5}}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("build stock timeshare response: %v", err)
	}
	if response.PreClose != 1418.5 || len(response.Data) != 0 {
		t.Fatalf("response = %#v, want preClose 1418.5 and no ticks", response)
	}
}

// 验证扩展市场分时构建服务保留点列并使用目标日昨收。
func TestBuildExHistoryTickResponse(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	response, err := buildExHistoryTickResponse(
		exHistoryTickRequest{Date: 20260724, Category: 31, Code: "01810"},
		[]proto.ExTickChartData{{Time: "09:30", Price: 18.6, Avg: 18.55}},
		exTimeSharePreCloseSource{
			now: func() time.Time { return time.Date(2026, 7, 28, 10, 0, 0, 0, loc) },
			dailyBars: func(uint8, string, time.Time) ([]proto.ExKLineItem, error) {
				return []proto.ExKLineItem{{PreClose: 18.0}}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("build ex timeshare response: %v", err)
	}
	if response.PreClose != 18.0 || len(response.Data) != 1 || response.Data[0].Price != 18.6 {
		t.Fatalf("response = %#v, want one tick with preClose 18", response)
	}
}

// 验证目录搜索服务拒绝空关键词并复用缓存搜索结果。
func TestSearchSymbolDirectory(t *testing.T) {
	now := time.Now()
	cache := &symbolDirectoryCache{
		entries:  []symbolSearchItem{{Symbol: "600519", Description: "贵州茅台", Exchange: "SH", Source: "gotdx"}},
		loaded:   true,
		loadedAt: now,
		ttl:      time.Hour,
		now:      func() time.Time { return now },
	}

	if _, err := searchSymbolDirectory(cache, "  ", nil); err == nil {
		t.Fatal("empty query succeeded")
	}
	limit := 1000
	items, err := searchSymbolDirectory(cache, "茅台", &limit)
	if err != nil {
		t.Fatalf("search directory: %v", err)
	}
	if len(items) != 1 || items[0].Symbol != "600519" {
		t.Fatalf("items = %#v, want 贵州茅台", items)
	}
}
