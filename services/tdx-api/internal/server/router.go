// HTTP 服务装配：健康检查、证券目录缓存与 V1 行情协议路由。
// 前端只消费 V1 协议，旧 /api/stock、/api/ex 等接口已删除。
package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"KlineChartQuantGo/services/tdx-api/internal/directory"
	"KlineChartQuantGo/services/tdx-api/internal/v1"
	"github.com/gin-gonic/gin"
)

func recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[panic] %v\n%s", rec, debug.Stack())
				if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
					// V1 协议保持 envelope 契约，避免前端解析裸 500 失败
					v1.WriteInternalError(c)
					return
				}
				c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// NewRouter 组装 HTTP 路由：健康检查、证券目录缓存与 V1 行情协议。
func NewRouter() *gin.Engine {
	path := os.Getenv("SYMBOL_DB_PATH")
	if path == "" {
		path = filepath.Join("data", "tdx-symbols.db")
	}
	store, err := directory.NewSQLiteStore(path)
	if err != nil {
		log.Printf("symbol directory database disabled: %v", err)
		store = nil
	}
	cache := directory.NewCache(directory.GotdxLoader{}, store, 0)
	go func() {
		if err := cache.WarmUp(); err != nil {
			log.Printf("symbol directory: warmup failed: %v", err)
		}
	}()
	return newRouterWithStatus(cache, client.DefaultManager().Status)
}

func newRouterWithStatus(symbolCache *directory.Cache, status func() client.Status) *gin.Engine {
	r := gin.New()
	r.Use(recoveryMiddleware(), corsMiddleware())
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		current := status()
		code := http.StatusOK
		if !current.Ready {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, current)
	})

	v1.RegisterRoutes(r, symbolCache, status)

	return r
}
