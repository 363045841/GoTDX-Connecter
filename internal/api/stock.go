package api

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

const klinePageSize uint16 = 798

type stockQuotesRequest struct {
	Markets []uint8  `json:"markets"`
	Codes   []string `json:"codes"`
}

type stockKLineRequest struct {
	Category uint16 `json:"category"`
	Market   uint8  `json:"market"`
	Code     string `json:"code"`
	Start    uint16 `json:"start"`
	Count    uint16 `json:"count"`
	Times    uint16 `json:"times"`
	Adjust   uint16 `json:"adjust"`
}

type stockTickRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
	Start  uint16 `json:"start"`
	Count  uint16 `json:"count"`
}

type stockListRequest struct {
	Market uint8  `json:"market"`
	Start  uint32 `json:"start"`
	Count  uint32 `json:"count"`
}

type stockCountRequest struct {
	Market uint8 `json:"market"`
}

type stockTransactionRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
	Start  uint16 `json:"start"`
	Count  uint16 `json:"count"`
}

type stockHistoryTransactionRequest struct {
	Date   uint32 `json:"date"`
	Market uint8  `json:"market"`
	Code   string `json:"code"`
	Start  uint16 `json:"start"`
	Count  uint16 `json:"count"`
}

type stockIndexInfoRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
}

func handleStockQuotes(c *gin.Context) {
	var req stockQuotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(req.Markets) == 0 || len(req.Codes) == 0 {
		c.JSON(400, gin.H{"error": "markets and codes are required"})
		return
	}
	stocks, err := client.Get().StockQuotesDetail(req.Markets, req.Codes)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, stocks)
}

func handleStockKLine(c *gin.Context) {
	var req stockKLineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	klines, err := client.Get().StockKLine(req.Category, req.Market, req.Code, req.Start, req.Count, req.Times, req.Adjust)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, klines)
}

type stockHistoryTickRequest struct {
	Date   uint32 `json:"date"`
	Market uint8  `json:"market"`
	Code   string `json:"code"`
}

func handleStockTick(c *gin.Context) {
	var req stockTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	tick, err := client.Get().StockTickChart(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tick)
}

func retryWithReprobe[T any](fn func() (T, error)) (T, error) {
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < maxRetries {
			log.Printf("[gotdx] 第%d次重试 (error: %v), re-probing hosts...", attempt, err)
			if rpErr := client.Reprobe(); rpErr != nil {
				log.Printf("[gotdx] re-probe failed: %v", rpErr)
			}
		}
	}
	var zero T
	return zero, fmt.Errorf("all %d retries failed: %w", maxRetries, lastErr)
}

type stockHistoryTickItem struct {
	Timestamp string  `json:"timestamp"`
	Price     float64 `json:"Price"`
	Avg       float64 `json:"Avg"`
	Vol       int     `json:"Vol"`
}

func handleStockHistoryTick(c *gin.Context) {
	var req stockHistoryTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	tick, err := retryWithReprobe(func() ([]proto.HistoryMinuteTimeData, error) {
		return client.Get().StockHistoryTickChart(req.Date, req.Market, req.Code)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	year := int(req.Date / 10000)
	month := int((req.Date % 10000) / 100)
	day := int(req.Date % 100)
	base := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	var first *stockHistoryTickItem
	if len(tick) > 0 {
		if trans, err := client.Get().StockHistoryFullTransaction(req.Date, req.Market, req.Code); err == nil && len(trans) > 0 {
			first = &stockHistoryTickItem{
				Timestamp: base.Add(9*time.Hour + 30*time.Minute).Format("2006-01-02T15:04:05+08:00"),
				Price:     trans[0].Price,
				Avg:       trans[0].Price,
				Vol:       trans[0].Vol,
			}
		}
	}

	respLen := len(tick)
	if first != nil {
		respLen++
	}
	resp := make([]stockHistoryTickItem, 0, respLen)
	if first != nil {
		resp = append(resp, *first)
	}
	for i, item := range tick {
		var t time.Time
		if i < 120 {
			t = base.Add(9*time.Hour + 31*time.Minute + time.Duration(i)*time.Minute)
		} else {
			t = base.Add(13*time.Hour + 1*time.Minute + time.Duration(i-120)*time.Minute)
		}
		resp = append(resp, stockHistoryTickItem{
			Timestamp: t.Format("2006-01-02T15:04:05+08:00"),
			Price:     item.Price,
			Avg:       item.Avg,
			Vol:       item.Vol,
		})
	}
	c.JSON(200, resp)
}

func handleStockList(c *gin.Context) {
	var req stockListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	stocks, err := client.Get().StockList(req.Market, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, stocks)
}

func handleStockCount(c *gin.Context) {
	var req stockCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	count, err := client.Get().StockCount(req.Market)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, map[string]uint16{"count": count})
}

func handleStockIndexInfo(c *gin.Context) {
	var req stockIndexInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	info, err := client.Get().StockIndexInfo(req.Market, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, info)
}

func handleStockTransaction(c *gin.Context) {
	var req stockTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().StockTransaction(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleStockHistoryTransaction(c *gin.Context) {
	var req stockHistoryTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().StockHistoryTransaction(req.Date, req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

type stockKLineByDateRequest struct {
	Market    uint8  `json:"market"`
	Code      string `json:"code"`
	Category  uint16 `json:"category"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Times     uint16 `json:"times"`
	Adjust    uint16 `json:"adjust"`
}

type stockKLineCountRequest struct {
	Market   uint8  `json:"market"`
	Code     string `json:"code"`
	Category uint16 `json:"category"`
	Count    uint16 `json:"count"`
	Times    uint16 `json:"times"`
	Adjust   uint16 `json:"adjust"`
}

func tryStockKLine(category uint16, market uint8, code string, start uint16, count uint16, times uint16, adjust uint16) (klines []proto.SecurityBar, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("StockKLine panic: %v", r)
		}
	}()
	return client.Get().StockKLine(category, market, code, start, count, times, adjust)
}

func tryExKLine(category uint8, code string, period uint16, start uint32, count uint16, times uint16) (klines []proto.ExKLineItem, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ExKLine panic: %v", r)
		}
	}()
	return client.Get().ExKLine(category, code, period, start, count, times)
}

func safeStockKLine(category uint16, market uint8, code string, start uint16, count uint16, times uint16, adjust uint16) ([]proto.SecurityBar, error) {
	klines, err := tryStockKLine(category, market, code, start, count, times, adjust)
	if err == nil {
		return klines, nil
	}
	log.Printf("[gotdx] StockKLine failed (%v), re-probing hosts and retrying...", err)
	if rpErr := client.Reprobe(); rpErr != nil {
		return nil, fmt.Errorf("re-probe failed: %w (original: %v)", rpErr, err)
	}
	return tryStockKLine(category, market, code, start, count, times, adjust)
}

func safeExKLine(category uint8, code string, period uint16, start uint32, count uint16, times uint16) ([]proto.ExKLineItem, error) {
	klines, err := tryExKLine(category, code, period, start, count, times)
	if err == nil {
		return klines, nil
	}
	log.Printf("[gotdx] ExKLine failed (%v), re-probing hosts and retrying...", err)
	if rpErr := client.Reprobe(); rpErr != nil {
		return nil, fmt.Errorf("re-probe failed: %w (original: %v)", rpErr, err)
	}
	return tryExKLine(category, code, period, start, count, times)
}

func StockKLineRange(category uint16, market uint8, code string, times uint16, adjust uint16, startDate, endDate time.Time) ([]proto.SecurityBar, error) {
	out := []proto.SecurityBar{}
	seen := make(map[string]bool)
	end := endDate.Add(24 * time.Hour)

	for start := uint16(0); ; start += klinePageSize {
		klines, err := safeStockKLine(category, market, code, start, klinePageSize, times, adjust)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			break
		}

		for _, k := range klines {
			key := fmt.Sprintf("%d-%02d-%02dT%02d:%02d", k.Year, k.Month, k.Day, k.Hour, k.Minute)
			if seen[key] {
				continue
			}
			seen[key] = true
			if (k.DateTime.Equal(startDate) || k.DateTime.After(startDate)) && k.DateTime.Before(end) {
				out = append(out, k)
			}
		}

		if len(klines) < int(klinePageSize) {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].DateTime.Before(out[j].DateTime)
	})
	return out, nil
}

func handleStockKLineCount(c *gin.Context) {
	var req stockKLineCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Times == 0 {
		req.Times = 1
	}
	if req.Count == 0 {
		req.Count = 1
	}

	klines, err := safeStockKLine(req.Category, req.Market, req.Code, 0, req.Count, req.Times, req.Adjust)
	if err != nil {
		c.JSON(200, gin.H{
			"ok":    false,
			"error": err.Error(),
			"count": 0,
		})
		return
	}
	c.JSON(200, gin.H{
		"ok":    true,
		"count": len(klines),
	})
}

type stockKLineByDateResponse struct {
	proto.SecurityBar
	Amplitude float64 `json:"Amplitude"`
}

func handleStockKLineByDate(c *gin.Context) {
	var req stockKLineByDateRequest
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

	klines, err := StockKLineRange(req.Category, req.Market, req.Code, req.Times, req.Adjust, start, end)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[gotdx] stock/kline-by-date %s cat=%d count=%d range=[%s,%s]",
		req.Code, req.Category, len(klines), req.StartDate, req.EndDate)

	resp := make([]stockKLineByDateResponse, len(klines))
	for i, k := range klines {
		amp := 0.0
		if k.Last != 0 {
			amp = math.Round((k.High-k.Low)/k.Last*10000) / 100
		} else if k.Open != 0 {
			amp = math.Round((k.High-k.Low)/k.Open*10000) / 100
		}
		resp[i] = stockKLineByDateResponse{SecurityBar: k, Amplitude: amp}
	}
	c.JSON(200, resp)
}
