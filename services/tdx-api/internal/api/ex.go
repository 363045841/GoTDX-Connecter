package api

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

var errTimeShareDataUnavailable = errors.New("timeshare data unavailable")

type exTimeSharePreCloseSource struct {
	now       func() time.Time
	quote     func(category uint8, code string) (float64, error)
	dailyBars func(category uint8, code string, date time.Time) ([]proto.ExKLineItem, error)
}

func newDefaultExTimeSharePreCloseSource() exTimeSharePreCloseSource {
	return exTimeSharePreCloseSource{
		now: time.Now,
		quote: func(category uint8, code string) (float64, error) {
			q, err := exCall(func(c client.ExQuerier) (*proto.ExQuoteItem, error) {
				return c.ExQuote(category, code)
			})
			if err != nil {
				return 0, err
			}
			if q == nil {
				return 0, errors.New("empty ex quote response")
			}
			return q.PreClose, nil
		},
		dailyBars: func(category uint8, code string, date time.Time) ([]proto.ExKLineItem, error) {
			// period 4 = 日线（与前端 PERIOD_TO_CATEGORY.daily 对齐）
			return ExKLineRange(category, code, 4, 1, date, date)
		},
	}
}

// resolveExTimeSharePreClose 当日优先实时昨收；无效（港股 ExQuote 常为 0）则回退目标日线。
// 历史日读目标日线昨收（PreClose → LastClose → Open）。
func resolveExTimeSharePreClose(category uint8, code string, date uint32, source exTimeSharePreCloseSource) (float64, error) {
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
		preClose, err := source.quote(category, code)
		if err == nil && !math.IsNaN(preClose) && !math.IsInf(preClose, 0) && preClose > 0 {
			return preClose, nil
		}
		// quote 失败或 PreClose=0：回退日线（HK 扩展行情常见）
	}

	bars, err := source.dailyBars(category, code, target)
	if err != nil {
		return 0, err
	}
	if len(bars) == 0 {
		return 0, errors.New("target daily bar not found")
	}
	preClose := exKLinePreClose(bars[0])
	if math.IsNaN(preClose) || math.IsInf(preClose, 0) || preClose <= 0 {
		return 0, fmt.Errorf("invalid historical preClose: %v", preClose)
	}
	return preClose, nil
}

// exKLinePreClose 读取扩展日线昨收：PreClose → LastClose → Open。
func exKLinePreClose(bar proto.ExKLineItem) float64 {
	if bar.PreClose > 0 {
		return bar.PreClose
	}
	if bar.LastClose > 0 {
		return bar.LastClose
	}
	return bar.Open
}

func fetchExHistoryTick(date uint32, category uint8, code string) ([]proto.ExTickChartData, error) {
	return exCall(func(c client.ExQuerier) ([]proto.ExTickChartData, error) {
		return c.ExTickChart(category, code, date)
	})
}

func handleExHistoryTick(c *gin.Context) {
	handleExHistoryTickWithDeps(c, fetchExHistoryTick, newDefaultExTimeSharePreCloseSource())
}

func handleExHistoryTickWithDeps(
	c *gin.Context,
	fetchTick func(date uint32, category uint8, code string) ([]proto.ExTickChartData, error),
	preCloseSource exTimeSharePreCloseSource,
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

	tick, err := fetchTick(req.Date, req.Category, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	resp, err := buildExHistoryTickResponse(req, tick, preCloseSource)
	if err != nil {
		if errors.Is(err, errTimeShareDataUnavailable) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "该日期暂无历史分时数据"})
			return
		}
		if strings.HasPrefix(err.Error(), "resolve preClose:") {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, resp)
}

// buildExHistoryTickResponse 将 GOTDX 扩展市场原始分时数据转换为统一点列和昨收。
func buildExHistoryTickResponse(
	req exHistoryTickRequest,
	tick []proto.ExTickChartData,
	preCloseSource exTimeSharePreCloseSource,
) (stockHistoryTickResponse, error) {
	if len(tick) > 0 {
		allZero := true
		for _, item := range tick {
			if item.Price != 0 || item.Avg != 0 || item.Vol != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			return stockHistoryTickResponse{}, errTimeShareDataUnavailable
		}
	}

	year := int(req.Date / 10000)
	month := int((req.Date % 10000) / 100)
	day := int(req.Date % 100)
	loc := time.FixedZone("CST", 8*60*60)
	base := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)

	resp := make([]stockHistoryTickItem, 0, len(tick))
	for _, item := range tick {
		ts, err := parseExTickClock(base, item.Time)
		if err != nil {
			return stockHistoryTickResponse{}, fmt.Errorf("invalid tick time: %w", err)
		}
		resp = append(resp, stockHistoryTickItem{
			Timestamp: ts.Format("2006-01-02T15:04:05-07:00"),
			Price:     item.Price,
			Avg:       item.Avg,
		})
	}

	preClose, err := resolveExTimeSharePreClose(req.Category, req.Code, req.Date, preCloseSource)
	if err != nil {
		return stockHistoryTickResponse{}, fmt.Errorf("resolve preClose: %w", err)
	}
	return newStockHistoryTickResponse(preClose, resp), nil
}

// parseExTickClock 将 gotdx "HH:mm" 接到目标日 Asia/Shanghai 墙钟
func parseExTickClock(base time.Time, clock string) (time.Time, error) {
	parts := strings.Split(clock, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("expected HH:mm, got %q", clock)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("out of range HH:mm %q", clock)
	}
	return time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location()), nil
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

// filterExKLineByDate 按日期区间过滤扩展 K 线，去重并升序
func filterExKLineByDate(klines []proto.ExKLineItem, startDate, endDate time.Time) []proto.ExKLineItem {
	out := make([]proto.ExKLineItem, 0, len(klines))
	seen := make(map[int64]bool)
	end := endDate.Add(24 * time.Hour)

	for _, k := range klines {
		if k.DateTime.IsZero() {
			continue
		}
		key := k.DateTime.UnixNano()
		if seen[key] {
			continue
		}
		seen[key] = true
		if (k.DateTime.Equal(startDate) || k.DateTime.After(startDate)) && k.DateTime.Before(end) {
			out = append(out, k)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].DateTime.Before(out[j].DateTime)
	})
	return out
}

// fetchExKLinePage 可测注入点；生产默认走 safeExKLine
var fetchExKLinePage = safeExKLine

func exKLineOldest(k proto.ExKLineItem) (time.Time, bool) {
	return k.DateTime, !k.DateTime.IsZero()
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
