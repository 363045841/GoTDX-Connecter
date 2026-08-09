// 扩展行情旧 HTTP 接口：请求解析、领域委托与响应序列化。
// K 线分页、分时构建与昨收解析等领域逻辑统一在 domain 包。
package api

import (
	"errors"
	"log"
	"math"
	"net/http"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"KlineChartQuantGo/services/tdx-api/internal/domain"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

type exListRequest struct {
	Start uint32 `json:"start"`
	Count uint16 `json:"count"`
}

type exQuoteRequest struct {
	Category uint8  `json:"category"`
	Code     string `json:"code"`
}

type exQuotesRequest struct {
	Categories []uint8  `json:"categories"`
	Codes      []string `json:"codes"`
}

type exKLineRequest struct {
	Category uint8  `json:"category"`
	Code     string `json:"code"`
	Period   uint16 `json:"period"`
	Start    uint32 `json:"start"`
	Count    uint16 `json:"count"`
	Times    uint16 `json:"times"`
}

type exTickRequest struct {
	Category uint8  `json:"category"`
	Code     string `json:"code"`
	Date     uint32 `json:"date"`
}

type exHistoryTransactionRequest struct {
	Date     uint32 `json:"date"`
	Category uint8  `json:"category"`
	Code     string `json:"code"`
}

func handleExCount(c *gin.Context) {
	count, err := exCall(func(c client.ExQuerier) (uint32, error) { return c.ExCount() })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, map[string]uint32{"count": count})
}

func handleExList(c *gin.Context) {
	var req exListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := exCall(func(c client.ExQuerier) ([]proto.ExListItem, error) { return c.ExList(req.Start, req.Count) })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleExQuote(c *gin.Context) {
	var req exQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := exCall(func(c client.ExQuerier) (*proto.ExQuoteItem, error) { return c.ExQuote(req.Category, req.Code) })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleExQuotes(c *gin.Context) {
	var req exQuotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(req.Categories) == 0 || len(req.Codes) == 0 {
		c.JSON(400, gin.H{"error": "categories and codes are required"})
		return
	}
	data, err := exCall(func(c client.ExQuerier) ([]proto.ExQuoteItem, error) { return c.ExQuotes(req.Categories, req.Codes) })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleExKLine(c *gin.Context) {
	var req exKLineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	klines, err := exCall(func(c client.ExQuerier) ([]proto.ExKLineItem, error) {
		return c.ExKLine(req.Category, req.Code, req.Period, req.Start, req.Count, req.Times)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, klines)
}

func handleExTick(c *gin.Context) {
	var req exTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	tick, err := exCall(func(c client.ExQuerier) ([]proto.ExTickChartData, error) {
		return c.ExTickChart(req.Category, req.Code, req.Date)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tick)
}

type exHistoryTickRequest struct {
	Date     uint32 `json:"date"`
	Category uint8  `json:"category"`
	Code     string `json:"code"`
}

func handleExHistoryTick(c *gin.Context) {
	handleExHistoryTickWithDeps(c, domain.FetchExHistoryTick, domain.NewDefaultExPreCloseSource())
}

func handleExHistoryTickWithDeps(
	c *gin.Context,
	fetchTick domain.ExHistoryTickFetcher,
	preCloseSource domain.ExPreCloseSource,
) {
	var req exHistoryTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Code == "" {
		c.JSON(400, gin.H{"error": "code is required"})
		return
	}

	points, err := domain.BuildExTimeSharePoints(
		domain.ExTimeShareRequest{Date: req.Date, Category: req.Category, Code: req.Code},
		fetchTick,
	)
	if err != nil {
		if errors.Is(err, domain.ErrNoTimeShareData) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	resp := make([]stockHistoryTickItem, 0, len(points))
	for _, p := range points {
		resp = append(resp, stockHistoryTickItem{
			Timestamp: p.At.Format("2006-01-02T15:04:05-07:00"),
			Price:     p.Price,
			Avg:       p.Avg,
		})
	}

	preClose, err := domain.ResolveExPreClose(req.Category, req.Code, req.Date, preCloseSource)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "resolve preClose: " + err.Error()})
		return
	}
	c.JSON(200, newStockHistoryTickResponse(preClose, resp))
}

func handleExHistoryTransaction(c *gin.Context) {
	var req exHistoryTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := exCall(func(c client.ExQuerier) ([]proto.ExHistoryTransactionItem, error) {
		return c.ExHistoryTransaction(req.Date, req.Category, req.Code)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleExTable(c *gin.Context) {
	data, err := exCall(func(c client.ExQuerier) (string, error) { return c.ExTable() })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, map[string]string{"table": data})
}

type exKLineByDateRequest struct {
	Category  uint8  `json:"category"`
	Code      string `json:"code"`
	Period    uint16 `json:"period"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Times     uint16 `json:"times"`
}

type exKLineByDateResponse struct {
	proto.ExKLineItem
	Amplitude float64 `json:"Amplitude"`
}

func handleExKLineByDate(c *gin.Context) {
	var req exKLineByDateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	start, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid start_date: " + err.Error()})
		return
	}
	end, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid end_date: " + err.Error()})
		return
	}
	if req.Times == 0 {
		req.Times = 1
	}

	klines, err := domain.ExKLineRange(req.Category, req.Code, req.Period, req.Times, start, end)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[gotdx] ex/kline-by-date %s cat=%d count=%d range=[%s,%s]",
		req.Code, req.Category, len(klines), req.StartDate, req.EndDate)

	resp := make([]exKLineByDateResponse, len(klines))
	for i, k := range klines {
		amp := 0.0
		if k.Open != 0 {
			amp = math.Round((k.High-k.Low)/k.Open*10000) / 100
		}
		resp[i] = exKLineByDateResponse{ExKLineItem: k, Amplitude: amp}
	}
	c.JSON(200, resp)
}
