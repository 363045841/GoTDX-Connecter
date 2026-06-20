package api

import (
	"log"
	"math"
	"sort"
	"time"

	"KlineChartQuantGo/internal/client"
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
	count, err := client.Get().ExCount()
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
	data, err := client.Get().ExList(req.Start, req.Count)
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
	data, err := client.Get().ExQuote(req.Category, req.Code)
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
	data, err := client.Get().ExQuotes(req.Categories, req.Codes)
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
	klines, err := client.Get().ExKLine(req.Category, req.Code, req.Period, req.Start, req.Count, req.Times)
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
	tick, err := client.Get().ExTickChart(req.Category, req.Code, req.Date)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tick)
}

func handleExHistoryTransaction(c *gin.Context) {
	var req exHistoryTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().ExHistoryTransaction(req.Date, req.Category, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleExTable(c *gin.Context) {
	data, err := client.Get().ExTable()
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

func ExKLineRange(category uint8, code string, period uint16, times uint16, startDate, endDate time.Time) ([]proto.ExKLineItem, error) {
	out := []proto.ExKLineItem{}
	seen := make(map[string]bool)
	end := endDate.Add(24 * time.Hour)

	for start := uint32(0); ; start += uint32(klinePageSize) {
		klines, err := safeExKLine(category, code, period, uint32(start), klinePageSize, times)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			break
		}

		for _, k := range klines {
			if seen[k.DateTime] {
				continue
			}
			seen[k.DateTime] = true
			t, err := time.ParseInLocation("2006-01-02", k.DateTime, time.Local)
			if err != nil {
				continue
			}
			if (t.Equal(startDate) || t.After(startDate)) && t.Before(end) {
				out = append(out, k)
			}
		}

		if len(klines) < int(klinePageSize) {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool {
		ti, _ := time.ParseInLocation("2006-01-02", out[i].DateTime, time.Local)
		tj, _ := time.ParseInLocation("2006-01-02", out[j].DateTime, time.Local)
		return ti.Before(tj)
	})
	return out, nil
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

	klines, err := ExKLineRange(req.Category, req.Code, req.Period, req.Times, start, end)
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
