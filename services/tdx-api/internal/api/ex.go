package api

import (
	"log"
	"math"
	"sort"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
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

// parseExKLineDateTime 解析扩展行情 DateTime；gotdx 日线为 "2006-01-02 15:04:05"
func parseExKLineDateTime(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

// filterExKLineByDate 按日期区间过滤扩展 K 线，去重并升序
func filterExKLineByDate(klines []proto.ExKLineItem, startDate, endDate time.Time) []proto.ExKLineItem {
	out := make([]proto.ExKLineItem, 0, len(klines))
	seen := make(map[string]bool)
	end := endDate.Add(24 * time.Hour)

	for _, k := range klines {
		if seen[k.DateTime] {
			continue
		}
		seen[k.DateTime] = true
		t, err := parseExKLineDateTime(k.DateTime)
		if err != nil {
			continue
		}
		if (t.Equal(startDate) || t.After(startDate)) && t.Before(end) {
			out = append(out, k)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		ti, _ := parseExKLineDateTime(out[i].DateTime)
		tj, _ := parseExKLineDateTime(out[j].DateTime)
		return ti.Before(tj)
	})
	return out
}

// fetchExKLinePage 可测注入点；生产默认走 safeExKLine
var fetchExKLinePage = safeExKLine

func exKLineOldest(k proto.ExKLineItem) (time.Time, bool) {
	t, err := parseExKLineDateTime(k.DateTime)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ExKLineRange 按 ExKLine 分页拉取扩展行情并按日期过滤
func ExKLineRange(category uint8, code string, period uint16, times uint16, startDate, endDate time.Time) ([]proto.ExKLineItem, error) {
	raw, err := paginateFromRecent(klinePageSize, func(start uint32, count uint16) ([]proto.ExKLineItem, error) {
		return fetchExKLinePage(category, code, period, start, count, times)
	}, exKLineOldest, startDate, false)
	if err != nil {
		return nil, err
	}
	return filterExKLineByDate(raw, startDate, endDate), nil
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
