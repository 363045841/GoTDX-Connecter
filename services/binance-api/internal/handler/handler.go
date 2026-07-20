package handler

import (
	"fmt"
	"net/http"
	"time"

	"KlineChartQuantGo/services/binance-api/internal/binance"
	"github.com/gin-gonic/gin"
)

// NewRouter 注册路由:
//   GET /api/binance/orderbook?symbol=btcusdt
//   GET /api/binance/depth-events?symbol=btcusdt
func NewRouter(bc *binance.Client, dh *binance.DepthHub) *gin.Engine {
	r := gin.New()
	r.Use(cors())

	api := r.Group("/api/binance")
	{
		api.GET("/orderbook", func(c *gin.Context) {
			symbol := c.DefaultQuery("symbol", "btcusdt")
			book := bc.GetOrderBook(symbol)
			if book == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "symbol not found or not yet subscribed"})
				return
			}
			c.JSON(http.StatusOK, book)
		})

		api.GET("/depth-events", func(c *gin.Context) {
			symbol := c.DefaultQuery("symbol", "btcusdt")
			ch, err := dh.Subscribe(symbol)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			defer dh.Unsubscribe(symbol, ch)

			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.Writer.WriteHeader(http.StatusOK)

			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-c.Request.Context().Done():
					return
				case <-ticker.C:
					fmt.Fprintf(c.Writer, ": keepalive\n\n")
					c.Writer.Flush()
				case data, ok := <-ch:
					if !ok {
						return
					}
					fmt.Fprintf(c.Writer, "data: %s\n\n", data)
					c.Writer.Flush()
				}
			}
		})
	}

	return r
}

// 全开 CORS 中间件
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
