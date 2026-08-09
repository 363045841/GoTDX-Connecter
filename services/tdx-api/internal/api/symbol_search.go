// 证券目录搜索 HTTP 接口：包装 directory 包的缓存搜索为 gin handler。
package api

import (
	"strings"

	"KlineChartQuantGo/services/tdx-api/internal/directory"
	"github.com/gin-gonic/gin"
)

const (
	defaultSymbolSearchLimit = 20
	maxSymbolSearchLimit     = 100
)

type symbolSearchRequest struct {
	Query string `json:"query"`
	Limit *int   `json:"limit"`
}

// newSymbolSearchHandler 创建证券目录搜索接口 handler。
func newSymbolSearchHandler(cache *directory.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req symbolSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		query := strings.TrimSpace(req.Query)
		if query == "" {
			c.JSON(400, gin.H{"error": "query is required"})
			return
		}
		limit := defaultSymbolSearchLimit
		if req.Limit != nil && *req.Limit > 0 {
			limit = *req.Limit
		}
		if limit > maxSymbolSearchLimit {
			limit = maxSymbolSearchLimit
		}
		items, err := cache.Search(query, limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, items)
	}
}
