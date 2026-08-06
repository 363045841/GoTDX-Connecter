package api

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/gin-gonic/gin"
)

func recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[panic] %v\n%s", rec, debug.Stack())
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

func NewRouter() *gin.Engine {
	path := os.Getenv("SYMBOL_DB_PATH")
	if path == "" {
		path = filepath.Join("data", "tdx-symbols.db")
	}
	store, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		log.Printf("symbol directory database disabled: %v", err)
		store = nil
	}
	cache := newSymbolDirectoryCache(gotdxSymbolDirectoryLoader{}, store, symbolDirectoryTTL)
	go func() {
		if err := cache.warmUp(); err != nil {
			log.Printf("symbol directory: warmup failed: %v", err)
		}
	}()
	return newRouterWithStatus(cache, client.DefaultManager().Status)
}

func newRouter(symbolCache *symbolDirectoryCache) *gin.Engine {
	return newRouterWithStatus(symbolCache, client.DefaultManager().Status)
}

func newRouterWithStatus(symbolCache *symbolDirectoryCache, status func() client.Status) *gin.Engine {
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

	stock := r.Group("/api/stock")
	{
		stock.POST("/quotes", handleStockQuotes)
		stock.POST("/kline", handleStockKLine)
		stock.POST("/kline-by-date", handleStockKLineByDate)
		stock.POST("/kline-count", handleStockKLineCount)
		stock.POST("/tick", handleStockTick)
		stock.POST("/history-tick", handleStockHistoryTick)
		stock.POST("/list", handleStockList)
		stock.POST("/count", handleStockCount)
		stock.POST("/index-info", handleStockIndexInfo)
		stock.POST("/transaction", handleStockTransaction)
		stock.POST("/history-transaction", handleStockHistoryTransaction)
	}

	ex := r.Group("/api/ex")
	{
		ex.POST("/count", handleExCount)
		ex.POST("/list", handleExList)
		ex.POST("/quote", handleExQuote)
		ex.POST("/quotes", handleExQuotes)
		ex.POST("/kline", handleExKLine)
		ex.POST("/kline-by-date", handleExKLineByDate)
		ex.POST("/tick", handleExTick)
		ex.POST("/history-tick", handleExHistoryTick)
		ex.POST("/history-transaction", handleExHistoryTransaction)
		ex.POST("/table", handleExTable)
	}

	mac := r.Group("/api/mac")
	{
		mac.POST("/board-list", handleMACBoardList)
		mac.POST("/board-members", handleMACBoardMembers)
		mac.POST("/board-members-quotes", handleMACBoardMembersQuotes)
		mac.POST("/board-members-quotes-dynamic", handleMACBoardMembersQuotesDynamic)
		mac.POST("/symbol-quotes", handleMACSymbolQuotes)
		mac.POST("/quotes", handleMACQuotes)
		mac.POST("/transactions", handleMACTransactions)
		mac.POST("/auction", handleMACAuction)
		mac.POST("/tick-charts", handleMACTickCharts)
		mac.GET("/server-info", handleMACServerInfo)
		mac.POST("/symbol-info", handleMACSymbolInfo)
		mac.POST("/capital-flow", handleMACCapitalFlow)
		mac.POST("/market-monitor", handleMACMarketMonitor)
	}

	r.POST("/api/hosts/probe", handleHostProbe)
	r.GET("/api/hosts/list", handleHostList)
	r.POST("/api/symbol/search", newSymbolSearchHandler(symbolCache))

	marketData := r.Group("/api/v1/market-data")
	{
		marketData.GET("/sources/:sourceId/probe", handleV1Probe(status))
		marketData.POST("/instruments/search", handleV1Search(symbolCache))
		marketData.POST("/bars", handleV1Bars)
		marketData.POST("/timeshare", handleV1TimeShare)
	}

	return r
}
