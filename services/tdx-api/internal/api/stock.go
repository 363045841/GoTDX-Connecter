// 主站 A 股/指数旧 HTTP 接口：请求解析、领域委托与响应序列化。
// K 线分页、分时构建与昨收解析等领域逻辑统一在 domain 包。
package api

import (
	"log"
	"math"
	"net/http"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"KlineChartQuantGo/services/tdx-api/internal/domain"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

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

func handleStockHistoryTick(c *gin.Context) {
	handleStockHistoryTickWithDeps(c, domain.FetchStockHistoryTick, domain.NewDefaultStockPreCloseSource())
}

func handleStockHistoryTickWithDeps(
	c *gin.Context,
	fetchTick domain.StockHistoryTickFetcher,
	preCloseSource domain.StockPreCloseSource,
) {
	var req stockHistoryTickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	isIndex := domain.IsIndexKind(req.Kind, req.Market, req.Code)
	nowFn := preCloseSource.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	domainReq := domain.StockTimeShareRequest{Date: req.Date, Market: req.Market, Code: req.Code, Kind: req.Kind}
	points, err := domain.BuildStockTimeSharePoints(domainReq, fetchTick, nowFn)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	preClose, err := domain.ResolveStockPreClose(
		req.Market, req.Code, req.Date,
		domain.NormalizeStockPreCloseSource(preCloseSource, domainReq, isIndex),
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "resolve preClose: " + err.Error()})
		return
	}
	resp := make([]stockHistoryTickItem, 0, len(points))
	for _, p := range points {
		resp = append(resp, stockHistoryTickItem{
			Timestamp: p.At.Format("2006-01-02T15:04:05+08:00"),
			Price:     p.Price,
			Avg:       p.Avg,
			Volume:    p.Volume,
			Amount:    p.Amount,
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
	if domain.IsIndexKind(req.Kind, req.Market, req.Code) {
		klines, err = domain.SafeIndexBars(req.Category, req.Market, req.Code, 0, req.Count)
	} else {
		klines, err = domain.SafeStockKLine(req.Category, req.Market, req.Code, 0, req.Count, req.Times, req.Adjust)
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
	asIndex := domain.IsIndexKind(req.Kind, req.Market, req.Code)
	if asIndex {
		klines, err2 = domain.IndexKLineRange(req.Category, req.Market, req.Code, start, end)
	} else {
		klines, err2 = domain.StockKLineRange(req.Category, req.Market, req.Code, req.Times, req.Adjust, start, end)
	}
	if err2 != nil {
		c.JSON(500, gin.H{"error": err2.Error()})
		return
	}
	log.Printf("[gotdx] stock/kline-by-date %s kind=%s cat=%d count=%d range=[%s,%s]",
		req.Code, map[bool]string{true: domain.KindIndex, false: domain.KindStock}[asIndex], req.Category, len(klines), req.StartDate, req.EndDate)

	resp := make([]stockKLineByDateResponse, len(klines))
	for i, k := range klines {
		amp := 0.0
		if base := domain.SecurityBarPreClose(k); base != 0 {
			amp = math.Round((k.High-k.Low)/base*10000) / 100
		} else if k.Open != 0 {
			amp = math.Round((k.High-k.Low)/k.Open*10000) / 100
		}
		resp[i] = stockKLineByDateResponse{SecurityBar: k, Amplitude: amp}
	}
	c.JSON(200, resp)
}
