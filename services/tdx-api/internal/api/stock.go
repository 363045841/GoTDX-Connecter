package api

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
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
	stocks, err := mainCall(func(c client.MainQuerier) ([]proto.SecurityQuote, error) {
		return c.StockQuotesDetail(req.Markets, req.Codes)
	})
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
	klines, err := mainCall(func(c client.MainQuerier) ([]proto.SecurityBar, error) {
		return c.StockKLine(req.Category, req.Market, req.Code, req.Start, req.Count, req.Times, req.Adjust)
	})
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
	tick, err := mainCall(func(c client.MainQuerier) ([]proto.MinuteTimeData, error) {
		return c.StockTickChart(req.Market, req.Code, req.Start, req.Count)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tick)
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
	tick, err := mainCall(func(c client.MainQuerier) ([]proto.HistoryMinuteTimeData, error) {
		return c.StockHistoryTickChart(req.Date, req.Market, req.Code)
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
		if trans, err := mainCall(func(c client.MainQuerier) ([]proto.HistoryTransactionData, error) {
			return c.StockHistoryFullTransaction(req.Date, req.Market, req.Code)
		}); err == nil && len(trans) > 0 {
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
	stocks, err := mainCall(func(c client.MainQuerier) ([]proto.Security, error) {
		return c.StockList(req.Market, req.Start, req.Count)
	})
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
	count, err := mainCall(func(c client.MainQuerier) (uint16, error) { return c.StockCount(req.Market) })
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
	info, err := mainCall(func(c client.MainQuerier) (*proto.GetIndexInfoReply, error) {
		return c.StockIndexInfo(req.Market, req.Code)
	})
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
	data, err := mainCall(func(c client.MainQuerier) ([]proto.TransactionData, error) {
		return c.StockTransaction(req.Market, req.Code, req.Start, req.Count)
	})
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
	data, err := mainCall(func(c client.MainQuerier) ([]proto.HistoryTransactionData, error) {
		return c.StockHistoryTransaction(req.Date, req.Market, req.Code, req.Start, req.Count)
	})
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
	// Kind 来自搜索 params.kind：stock|index；空则按 gotdx types.IsIndex(code.exchange) 判定
	Kind string `json:"kind"`
}

type stockKLineCountRequest struct {
	Market   uint8  `json:"market"`
	Code     string `json:"code"`
	Category uint16 `json:"category"`
	Count    uint16 `json:"count"`
	Times    uint16 `json:"times"`
	Adjust   uint16 `json:"adjust"`
	Kind     string `json:"kind"`
}

func tryStockKLine(category uint16, market uint8, code string, start uint16, count uint16, times uint16, adjust uint16) (klines []proto.SecurityBar, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("StockKLine panic: %v", r)
		}
	}()
	return mainCall(func(c client.MainQuerier) ([]proto.SecurityBar, error) {
		return c.StockKLine(category, market, code, start, count, times, adjust)
	})
}

// tryIndexBars 拉指数 K 线并映射为 SecurityBar，供与股票接口共用按日筛选
func tryIndexBars(category uint16, market uint8, code string, start uint16, count uint16) (klines []proto.SecurityBar, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("GetIndexBars panic: %v", r)
		}
	}()
	reply, err := mainCall(func(c client.MainQuerier) (*proto.GetIndexBarsReply, error) {
		return c.GetIndexBars(category, market, code, start, count)
	})
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return []proto.SecurityBar{}, nil
	}
	out := make([]proto.SecurityBar, 0, len(reply.List))
	for _, b := range reply.List {
		dt, parseErr := time.ParseInLocation("2006-01-02 15:04:05", b.DateTime, time.Local)
		if parseErr != nil {
			dt, parseErr = time.ParseInLocation("2006-01-02T15:04:05", b.DateTime, time.Local)
		}
		if parseErr != nil {
			dt = time.Date(b.Year, time.Month(b.Month), b.Day, b.Hour, b.Minute, 0, 0, time.Local)
		}
		out = append(out, proto.SecurityBar{
			Open:      b.Open,
			Close:     b.Close,
			High:      b.High,
			Low:       b.Low,
			Vol:       b.Vol,
			Amount:    b.Amount,
			Year:      b.Year,
			Month:     b.Month,
			Day:       b.Day,
			Hour:      b.Hour,
			Minute:    b.Minute,
			DateTime:  dt,
			UpCount:   b.UpCount,
			DownCount: b.DownCount,
		})
	}
	return out, nil
}

// isIndexKLineRequest 是否按指数接口拉线：优先 kind，否则 IsIndex(code.EXCHANGE)
func isIndexKLineRequest(kind string, market uint8, code string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case symbolKindIndex:
		return true
	case symbolKindStock, symbolKindEx:
		return false
	}
	// kind 未传时，用 gotdx 规则判定（不猜 market  alone）
	return mainMarketSymbolKind(code, mainExchange(market)) == symbolKindIndex
}

func tryExKLine(category uint8, code string, period uint16, start uint32, count uint16, times uint16) (klines []proto.ExKLineItem, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ExKLine panic: %v", r)
		}
	}()
	return exCall(func(c client.ExQuerier) ([]proto.ExKLineItem, error) {
		return c.ExKLine(category, code, period, start, count, times)
	})
}

func safeStockKLine(category uint16, market uint8, code string, start uint16, count uint16, times uint16, adjust uint16) ([]proto.SecurityBar, error) {
	klines, err := tryStockKLine(category, market, code, start, count, times, adjust)
	if err == nil {
		return klines, nil
	}
	return nil, err
}

func safeIndexBars(category uint16, market uint8, code string, start uint16, count uint16) ([]proto.SecurityBar, error) {
	klines, err := tryIndexBars(category, market, code, start, count)
	if err == nil {
		return klines, nil
	}
	return nil, err
}

func safeExKLine(category uint8, code string, period uint16, start uint32, count uint16, times uint16) ([]proto.ExKLineItem, error) {
	klines, err := tryExKLine(category, code, period, start, count, times)
	if err == nil {
		return klines, nil
	}
	return nil, err
}

// indexPageSize GetIndexBars 单次请求上限，保持在已验证的 800 条以内
const indexPageSize uint16 = 798

// fetchStockKLinePage / fetchIndexBarsPage 可测注入点；生产默认走 safe*
var (
	fetchStockKLinePage = safeStockKLine
	fetchIndexBarsPage  = safeIndexBars
)

func securityBarOldest(k proto.SecurityBar) (time.Time, bool) {
	return k.DateTime, !k.DateTime.IsZero()
}

// StockKLineRange 按 StockKLine 分页拉取 A 股 K 线并按日期过滤
func StockKLineRange(category uint16, market uint8, code string, times uint16, adjust uint16, startDate, endDate time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateFromRecent(klinePageSize, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		return fetchStockKLinePage(category, market, code, s, count, times, adjust)
	}, securityBarOldest, startDate, false)
	if err != nil {
		return nil, err
	}
	return filterKLineByDate(raw, startDate, endDate, true), nil
}

// IndexKLineRange 按 GetIndexBars 分页拉取指数 K 线并按日期过滤
// 深页偶发 gotdx invalid kline datetime：先缩 count 再试，仍失败且已有数据则截断
func IndexKLineRange(category uint16, market uint8, code string, startDate, endDate time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateFromRecent(indexPageSize, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		klines, err := fetchIndexBarsPage(category, market, code, s, count)
		if err != nil && count > indexFallbackPageSize {
			return fetchIndexBarsPage(category, market, code, s, indexFallbackPageSize)
		}
		return klines, err
	}, securityBarOldest, startDate, true)
	if err != nil {
		return nil, err
	}
	return filterKLineByDate(raw, startDate, endDate, true), nil
}

func filterKLineByDate(klines []proto.SecurityBar, startDate, endDate time.Time, dedup bool) []proto.SecurityBar {
	out := make([]proto.SecurityBar, 0, len(klines))
	end := endDate.Add(24 * time.Hour)
	seen := make(map[string]bool)

	for _, k := range klines {
		if dedup {
			key := fmt.Sprintf("%d-%02d-%02dT%02d:%02d", k.Year, k.Month, k.Day, k.Hour, k.Minute)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		if (k.DateTime.Equal(startDate) || k.DateTime.After(startDate)) && k.DateTime.Before(end) {
			out = append(out, k)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].DateTime.Before(out[j].DateTime)
	})
	return out
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

	var klines []proto.SecurityBar
	var err error
	if isIndexKLineRequest(req.Kind, req.Market, req.Code) {
		klines, err = safeIndexBars(req.Category, req.Market, req.Code, 0, req.Count)
	} else {
		klines, err = safeStockKLine(req.Category, req.Market, req.Code, 0, req.Count, req.Times, req.Adjust)
	}
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

	var klines []proto.SecurityBar
	var err2 error
	asIndex := isIndexKLineRequest(req.Kind, req.Market, req.Code)
	if asIndex {
		klines, err2 = IndexKLineRange(req.Category, req.Market, req.Code, start, end)
	} else {
		klines, err2 = StockKLineRange(req.Category, req.Market, req.Code, req.Times, req.Adjust, start, end)
	}
	if err2 != nil {
		c.JSON(500, gin.H{"error": err2.Error()})
		return
	}
	log.Printf("[gotdx] stock/kline-by-date %s kind=%s cat=%d count=%d range=[%s,%s]",
		req.Code, map[bool]string{true: symbolKindIndex, false: symbolKindStock}[asIndex], req.Category, len(klines), req.StartDate, req.EndDate)

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
