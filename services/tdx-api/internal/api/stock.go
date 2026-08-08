package api

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
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
	Kind   string `json:"kind"`
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
	Volume    *int    `json:"Volume,omitempty"`
	Amount    *int64  `json:"Amount,omitempty"`
}

// stockHistoryTickResponse 分时统一契约：点列 + 昨收元数据。
// 客户端只认该形状，后续港股/美股/期货扩展不改字段名。
type stockHistoryTickResponse struct {
	PreClose float64                `json:"preClose"`
	Data     []stockHistoryTickItem `json:"data"`
}

func newStockHistoryTickResponse(preClose float64, data []stockHistoryTickItem) stockHistoryTickResponse {
	if data == nil {
		data = []stockHistoryTickItem{}
	}
	return stockHistoryTickResponse{PreClose: preClose, Data: data}
}

type timeSharePreCloseSource struct {
	now       func() time.Time
	quote     func(market uint8, code string) (float64, error)
	dailyBars func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error)
}

func newDefaultTimeSharePreCloseSource() timeSharePreCloseSource {
	return timeSharePreCloseSource{
		now: time.Now,
		quote: func(market uint8, code string) (float64, error) {
			quotes, err := mainCall(func(c client.MainQuerier) ([]proto.SecurityQuote, error) {
				return c.StockQuotesDetail([]uint8{market}, []string{code})
			})
			if err != nil {
				return 0, err
			}
			if len(quotes) == 0 {
				return 0, errors.New("empty realtime quote response")
			}
			return quotes[0].PreClose, nil
		},
		// dailyBars 由 handleStockHistoryTickWithDeps 按 kind 装配
	}
}

// resolveTimeSharePreClose 当日读取实时行情，历史日读取目标日线的前收价。
func resolveTimeSharePreClose(market uint8, code string, date uint32, source timeSharePreCloseSource) (float64, error) {
	year := int(date / 10000)
	month := time.Month((date % 10000) / 100)
	day := int(date % 100)
	loc := time.FixedZone("CST", 8*60*60)
	target := time.Date(year, month, day, 0, 0, 0, 0, loc)
	if target.Year() != year || target.Month() != month || target.Day() != day {
		return 0, fmt.Errorf("invalid history date: %d", date)
	}

	now := source.now().In(loc)
	currentDate := uint32(now.Year()*10000 + int(now.Month())*100 + now.Day())
	if date == currentDate {
		preClose, err := source.quote(market, code)
		if err != nil {
			return 0, err
		}
		if math.IsNaN(preClose) || math.IsInf(preClose, 0) || preClose <= 0 {
			return 0, fmt.Errorf("invalid realtime preClose: %v", preClose)
		}
		return preClose, nil
	}

	bars, err := source.dailyBars(market, code, target)
	if err != nil {
		return 0, err
	}
	if len(bars) == 0 {
		return 0, errors.New("target daily bar not found")
	}
	preClose := securityBarPreClose(bars[0])
	if math.IsNaN(preClose) || math.IsInf(preClose, 0) || preClose <= 0 {
		return 0, fmt.Errorf("invalid historical preClose: %v", preClose)
	}
	return preClose, nil
}

// securityBarPreClose 读取日线昨收：优先 PreClose，其次 LastClose。
func securityBarPreClose(bar proto.SecurityBar) float64 {
	if bar.PreClose > 0 {
		return bar.PreClose
	}
	return bar.LastClose
}

func fetchStockHistoryTick(date uint32, market uint8, code string) ([]proto.HistoryMinuteTimeData, error) {
	return mainCall(func(c client.MainQuerier) ([]proto.HistoryMinuteTimeData, error) {
		return c.StockHistoryTickChart(date, market, code)
	})
}

// openingTrade 开盘首笔，用于补 09:30 分时点。
type openingTrade struct {
	Price float64
	Vol   int
}

// fetchOpeningTrade 按日期选逐笔源：当日用 StockFullTransaction，历史日用 StockHistoryFullTransaction。
// 可测注入；生产默认见 defaultFetchOpeningTrade。
var fetchOpeningTrade = defaultFetchOpeningTrade

func defaultFetchOpeningTrade(date uint32, market uint8, code string, now time.Time) (openingTrade, bool) {
	loc := time.FixedZone("CST", 8*60*60)
	now = now.In(loc)
	currentDate := uint32(now.Year()*10000 + int(now.Month())*100 + now.Day())
	if date == currentDate {
		trans, err := mainCall(func(c client.MainQuerier) ([]proto.TransactionData, error) {
			return c.StockFullTransaction(market, code)
		})
		if err != nil || len(trans) == 0 {
			return openingTrade{}, false
		}
		return openingTrade{Price: trans[0].Price, Vol: trans[0].Vol}, true
	}
	trans, err := mainCall(func(c client.MainQuerier) ([]proto.HistoryTransactionData, error) {
		return c.StockHistoryFullTransaction(date, market, code)
	})
	if err != nil || len(trans) == 0 {
		return openingTrade{}, false
	}
	return openingTrade{Price: trans[0].Price, Vol: trans[0].Vol}, true
}

func handleStockHistoryTick(c *gin.Context) {
	handleStockHistoryTickWithDeps(c, fetchStockHistoryTick, newDefaultTimeSharePreCloseSource())
}

func handleStockHistoryTickWithDeps(
	c *gin.Context,
	fetchTick func(date uint32, market uint8, code string) ([]proto.HistoryMinuteTimeData, error),
	preCloseSource timeSharePreCloseSource,
) {
	var req stockHistoryTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	isIndex := isIndexKLineRequest(req.Kind, req.Market, req.Code)
	nowFn := preCloseSource.now
	if nowFn == nil {
		nowFn = time.Now
	}
	points, err := buildStockTimeSharePoints(req, fetchTick, nowFn)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	preClose, err := resolveTimeSharePreClose(
		req.Market, req.Code, req.Date,
		normalizeStockPreCloseSource(preCloseSource, req, isIndex),
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "resolve preClose: " + err.Error()})
		return
	}
	resp := make([]stockHistoryTickItem, 0, len(points))
	for _, p := range points {
		resp = append(resp, stockHistoryTickItem{
			Timestamp: p.at.Format("2006-01-02T15:04:05+08:00"),
			Price:     p.price,
			Avg:       p.avg,
			Volume:    p.volume,
			Amount:    p.amount,
		})
	}
	c.JSON(200, newStockHistoryTickResponse(preClose, resp))
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
	loc := time.FixedZone("CST", 8*60*60)
	for _, b := range reply.List {
		dt := b.DateTime
		if dt.IsZero() {
			dt = time.Date(b.Year, time.Month(b.Month), b.Day, b.Hour, b.Minute, 0, 0, loc)
		}
		out = append(out, proto.SecurityBar{
			PreClose:  b.PreClose,
			LastClose: b.LastClose,
			Open:      b.Open,
			Close:     b.Close,
			High:      b.High,
			Low:       b.Low,
			Vol:       b.Vol,
			Amount:    b.Amount,
			RisePrice: b.RisePrice,
			RiseRate:  b.RiseRate,
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
		if base := securityBarPreClose(k); base != 0 {
			amp = math.Round((k.High-k.Low)/base*10000) / 100
		} else if k.Open != 0 {
			amp = math.Round((k.High-k.Low)/k.Open*10000) / 100
		}
		resp[i] = stockKLineByDateResponse{SecurityBar: k, Amplitude: amp}
	}
	c.JSON(200, resp)
}
