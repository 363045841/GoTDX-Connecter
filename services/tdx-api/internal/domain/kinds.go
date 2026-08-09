// 品种 kind 常量与主市场判定辅助，供 api/v1 两层 K 线路由共用。
package domain

import (
	"fmt"
	"strings"

	"github.com/bensema/gotdx/types"
)

// 搜索结果 params.kind：与 gotdx types 品种分类对齐，供 K 线路由使用。
const (
	KindStock = "stock"
	KindIndex = "index"
	KindEx    = "ex"
)

// MainExchange 返回主市场行情代码对应的交易所缩写。
func MainExchange(market uint8) string {
	switch market {
	case 0:
		return "SZ"
	case 1:
		return "SH"
	case 2:
		return "BJ"
	default:
		return ""
	}
}

// MainMarketSymbolKind 用 gotdx types.IsIndex(code.EXCHANGE) 判定主市场品种
func MainMarketSymbolKind(code, exchange string) string {
	// IsIndex 要求 9 位形如 000001.SH
	if types.IsIndex(fmt.Sprintf("%s.%s", code, strings.ToUpper(exchange))) {
		return KindIndex
	}
	return KindStock
}

// IsIndexKind 是否按指数接口拉线：优先 kind，否则 IsIndex(code.EXCHANGE)
func IsIndexKind(kind string, market uint8, code string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindIndex:
		return true
	case KindStock, KindEx:
		return false
	}
	// kind 未传时，用 gotdx 规则判定（不猜 market alone）
	return MainMarketSymbolKind(code, MainExchange(market)) == KindIndex
}
