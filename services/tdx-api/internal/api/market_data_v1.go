// 本文件实现前端主导的统一行情 REST V1 协议。
package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

const v1SourceID = "gotdx"

var v1Periods = map[string]uint16{
	"1min": 8, "5min": 0, "15min": 1, "30min": 2, "60min": 3,
	"daily": 4, "weekly": 5, "monthly": 6, "quarterly": 10, "yearly": 11,
}

var v1PeriodOrder = []string{
	"1min", "5min", "15min", "30min", "60min",
	"daily", "weekly", "monthly", "quarterly", "yearly",
}

type v1ProviderRef struct {
	Market   any    `json:"market"`
	Category any    `json:"category"`
	Kind     string `json:"kind"`
}

type v1InstrumentRef struct {
	ID          string        `json:"id"`
	Symbol      string        `json:"symbol"`
	Exchange    string        `json:"exchange"`
	ProviderRef v1ProviderRef `json:"providerRef"`
}

type v1BarRequest struct {
	SourceID   string          `json:"sourceId"`
	Instrument v1InstrumentRef `json:"instrument"`
	Period     string          `json:"period"`
	Adjustment string          `json:"adjustment"`
	From       int64           `json:"from"`
	To         int64           `json:"to"`
}

type v1TimeShareRequest struct {
	SourceID    string          `json:"sourceId"`
	Instrument  v1InstrumentRef `json:"instrument"`
	TradingDate string          `json:"tradingDate"`
}

type v1SearchRequest struct {
	SourceID     string   `json:"sourceId"`
	Keyword      string   `json:"keyword"`
	Limit        int      `json:"limit"`
	AssetClasses []string `json:"assetClasses"`
}

type v1Envelope struct {
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}

type v1ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type v1ErrorEnvelope struct {
	Error     v1ErrorBody `json:"error"`
	RequestID string      `json:"requestId"`
}

type v1SearchResponse struct {
	Items []v1Instrument `json:"items"`
}

type v1Instrument struct {
	ID           string         `json:"id"`
	SourceID     string         `json:"sourceId"`
	Symbol       string         `json:"symbol"`
	Name         string         `json:"name"`
	AssetClass   string         `json:"assetClass"`
	Exchange     string         `json:"exchange"`
	SessionID    string         `json:"sessionId,omitempty"`
	Currency     string         `json:"currency,omitempty"`
	ProviderRef  v1ProviderRef  `json:"providerRef,omitempty"`
	Capabilities v1Capabilities `json:"capabilities"`
}

type v1Capabilities struct {
	Bars      *v1BarCapability `json:"bars,omitempty"`
	TimeShare bool             `json:"timeShare,omitempty"`
}

type v1BarCapability struct {
	Periods     []string `json:"periods"`
	Adjustments []string `json:"adjustments"`
}

type v1BarItem struct {
	Timestamp     int64   `json:"timestamp"`
	Date          string  `json:"date,omitempty"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	Volume        float64 `json:"volume,omitempty"`
	Turnover      float64 `json:"turnover,omitempty"`
	Amplitude     float64 `json:"amplitude,omitempty"`
	ChangePercent float64 `json:"changePercent,omitempty"`
	ChangeAmount  float64 `json:"changeAmount,omitempty"`
	TurnoverRate  float64 `json:"turnoverRate,omitempty"`
}

type v1BarSeries struct {
	InstrumentID string      `json:"instrumentId"`
	Period       string      `json:"period"`
	Adjustment   string      `json:"adjustment"`
	Timezone     string      `json:"timezone"`
	VolumeUnit   string      `json:"volumeUnit,omitempty"`
	Items        []v1BarItem `json:"items"`
}

type v1TimeShareItem struct {
	Timestamp int64   `json:"timestamp"`
	Price     float64 `json:"price"`
	Average   float64 `json:"average"`
	Volume    *int    `json:"volume,omitempty"`
	Amount    *int64  `json:"amount,omitempty"`
}

type v1TimeShareSeries struct {
	InstrumentID string            `json:"instrumentId"`
	TradingDate  string            `json:"tradingDate"`
	Timezone     string            `json:"timezone"`
	PreClose     float64           `json:"preClose"`
	Items        []v1TimeShareItem `json:"items"`
}

// v1RequestID 返回当前请求的可追踪标识。
func v1RequestID() string { return strconv.FormatInt(time.Now().UnixNano(), 10) }

// writeV1Error 输出统一错误 envelope。
func writeV1Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, v1ErrorEnvelope{Error: v1ErrorBody{Code: code, Message: message}, RequestID: v1RequestID()})
}

// writeV1Data 输出统一成功 envelope。
func writeV1Data(c *gin.Context, data any) {
	c.JSON(http.StatusOK, v1Envelope{Data: data, RequestID: v1RequestID()})
}

// v1Number 从 JSON providerRef 读取数字字段。
func v1Number(value any) (uint8, bool) {
	switch n := value.(type) {
	case float64:
		if n >= 0 && n <= 255 && n == float64(uint8(n)) {
			return uint8(n), true
		}
	case int:
		if n >= 0 && n <= 255 {
			return uint8(n), true
		}
	case uint8:
		return n, true
	case uint16:
		if n <= 255 {
			return uint8(n), true
		}
	case uint32:
		if n <= 255 {
			return uint8(n), true
		}
	}
	return 0, false
}

// v1DateText 将 UTC 毫秒转换为 Asia/Shanghai 的自然日。
func v1DateText(timestamp int64) string {
	return time.UnixMilli(timestamp).In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02")
}

// v1DateNumber 校验并转换 YYYY-MM-DD 交易日。
func v1DateNumber(value string) (uint32, error) {
	date, err := time.ParseInLocation("2006-01-02", value, time.FixedZone("CST", 8*60*60))
	if err != nil {
		return 0, err
	}
	return uint32(date.Year()*10000 + int(date.Month())*100 + date.Day()), nil
}

// v1InstrumentID 为旧搜索缓存品种生成稳定 ID。
func v1InstrumentID(item symbolSearchItem) string {
	if category, ok := v1Number(item.Params["category"]); ok {
		return fmt.Sprintf("gotdx:ex:%d:%s", category, item.Symbol)
	}
	if market, ok := v1Number(item.Params["market"]); ok {
		kind := item.Params["kind"]
		if kind == nil {
			kind = symbolKindStock
		}
		return fmt.Sprintf("gotdx:%v:%d:%s", kind, market, item.Symbol)
	}
	return fmt.Sprintf("gotdx:unknown:%s:%s", item.Exchange, item.Symbol)
}

// v1AssetClass 将 GOTDX 搜索语义转换为前端资产类别。
func v1AssetClass(item symbolSearchItem) string {
	if item.Params["kind"] == symbolKindIndex {
		return symbolKindIndex
	}
	switch item.Exchange {
	case "FUTURES":
		return "future"
	case "FX":
		return "forex"
	case "OPTION":
		return "option"
	case "FUND", "MONEY", "MONEY_FUND":
		return "fund"
	case "INDEX":
		return symbolKindIndex
	default:
		if item.Params["kind"] == symbolKindStock || item.Params["market"] != nil {
			return symbolKindStock
		}
		return "unknown"
	}
}

// v1SessionID 将 GOTDX 搜索语义归一化为前端已注册的交易会话。
func v1SessionID(item symbolSearchItem) string {
	if market, ok := v1Number(item.Params["market"]); ok && market <= 2 {
		return "CN"
	}
	if _, ok := v1Number(item.Params["category"]); !ok || item.Params["kind"] != symbolKindEx {
		return ""
	}
	switch item.Exchange {
	case "CN", "FUND", "MONEY", "MONEY_FUND":
		return "CN"
	case "HK":
		return "HK"
	case "US":
		return "US"
	default:
		return ""
	}
}

// toV1Instrument 将缓存目录条目转换为统一品种描述。
func toV1Instrument(item symbolSearchItem) v1Instrument {
	market := v1SessionID(item)
	capability := v1Capabilities{}
	if market != "" {
		periods := append([]string(nil), v1PeriodOrder...)
		adjustments := []string{"none"}
		if v1AssetClass(item) == symbolKindStock && item.Params["kind"] != symbolKindEx {
			adjustments = []string{"qfq", "hfq", "none"}
		}
		capability.Bars = &v1BarCapability{Periods: periods, Adjustments: adjustments}
		capability.TimeShare = true
	}
	return v1Instrument{
		ID: v1InstrumentID(item), SourceID: v1SourceID, Symbol: item.Symbol,
		Name: item.Description, AssetClass: v1AssetClass(item), Exchange: item.Exchange,
		SessionID: market, ProviderRef: v1ProviderRefFromMap(item.Params), Capabilities: capability,
	}
}

// v1ProviderRefFromMap 将缓存中的通用参数投影为协议字段。
func v1ProviderRefFromMap(params map[string]any) v1ProviderRef {
	return v1ProviderRef{Market: params["market"], Category: params["category"], Kind: stringValue(params["kind"])}
}

// stringValue 返回通用参数中的字符串值。
func stringValue(value any) string {
	if result, ok := value.(string); ok {
		return result
	}
	return ""
}

// handleV1Probe 返回 GOTDX 数据源状态。
func handleV1Probe(status func() client.Status) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Param("sourceId") != v1SourceID {
			writeV1Error(c, http.StatusNotFound, "INSTRUMENT_NOT_FOUND", "unknown source")
			return
		}
		current := status()
		state := "offline"
		if current.Ready {
			state = "online"
		}
		writeV1Data(c, gin.H{"status": state, "checkedAt": time.Now().UnixMilli()})
	}
}

// handleV1Search 搜索并返回统一品种目录。
func handleV1Search(cache *symbolDirectoryCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v1SearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		if req.SourceID != v1SourceID {
			writeV1Error(c, http.StatusNotFound, "INSTRUMENT_NOT_FOUND", "unknown source")
			return
		}
		items, err := searchSymbolDirectory(cache, req.Keyword, &req.Limit)
		if err != nil {
			status := http.StatusBadRequest
			if err.Error() != "query is required" {
				status = http.StatusBadGateway
			}
			writeV1Error(c, status, "INVALID_REQUEST", err.Error())
			return
		}
		result := make([]v1Instrument, 0, len(items))
		for _, item := range items {
			if len(req.AssetClasses) > 0 && !containsString(req.AssetClasses, v1AssetClass(item)) {
				continue
			}
			result = append(result, toV1Instrument(item))
		}
		writeV1Data(c, v1SearchResponse{Items: result})
	}
}

// containsString 判断过滤列表是否包含目标资产类别。
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// handleV1Bars 拉取并映射统一 K 线序列。
func handleV1Bars(c *gin.Context) {
	var req v1BarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.SourceID != v1SourceID || req.Instrument.Symbol == "" {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "sourceId and instrument are required")
		return
	}
	category, ok := v1Periods[req.Period]
	if !ok {
		writeV1Error(c, http.StatusUnprocessableEntity, "UNSUPPORTED_CAPABILITY", "unsupported period")
		return
	}
	if req.From > req.To {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "from must not be after to")
		return
	}
	start, end := v1DateText(req.From), v1DateText(req.To)
	ref := req.Instrument.ProviderRef
	var result []v1BarItem
	var err error
	if exCategory, hasCategory := v1Number(ref.Category); hasCategory {
		items, fetchErr := ExKLineRange(exCategory, req.Instrument.Symbol, category, 1, parseV1Date(start), parseV1Date(end))
		err = fetchErr
		result = make([]v1BarItem, 0, len(items))
		for _, item := range items {
			result = append(result, v1ExBar(item))
		}
	} else if market, hasMarket := v1Number(ref.Market); hasMarket {
		items, fetchErr := fetchV1StockBars(category, market, req.Instrument, start, end, req.Adjustment)
		err = fetchErr
		result = make([]v1BarItem, 0, len(items))
		for _, item := range items {
			result = append(result, v1SecurityBar(item))
		}
	} else {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "providerRef.market or category is required")
		return
	}
	if err != nil {
		writeV1Error(c, http.StatusBadGateway, "UPSTREAM_UNAVAILABLE", err.Error())
		return
	}
	writeV1Data(c, v1BarSeries{InstrumentID: req.Instrument.ID, Period: req.Period, Adjustment: req.Adjustment, Timezone: "Asia/Shanghai", VolumeUnit: "lot", Items: result})
}

// parseV1Date 将协议日期解析到 GOTDX 使用的固定时区。
func parseV1Date(value string) time.Time {
	date, _ := time.ParseInLocation("2006-01-02", value, time.FixedZone("CST", 8*60*60))
	return date
}

// fetchV1StockBars 复用股票和指数已有分页领域函数。
func fetchV1StockBars(category uint16, market uint8, instrument v1InstrumentRef, start, end, adjustment string) ([]proto.SecurityBar, error) {
	startDate, endDate := parseV1Date(start), parseV1Date(end)
	if isIndexKLineRequest(instrument.ProviderRef.Kind, market, instrument.Symbol) {
		return IndexKLineRange(category, market, instrument.Symbol, startDate, endDate)
	}
	adjust := uint16(0)
	if adjustment == "qfq" {
		adjust = 1
	} else if adjustment == "hfq" {
		adjust = 2
	}
	return StockKLineRange(category, market, instrument.Symbol, 1, adjust, startDate, endDate)
}

// v1SecurityBar 将 GOTDX 股票/指数 K 线映射为统一字段。
func v1SecurityBar(item proto.SecurityBar) v1BarItem {
	amplitude := 0.0
	if base := securityBarPreClose(item); base != 0 {
		amplitude = (item.High - item.Low) / base * 100
	}
	return v1BarItem{Timestamp: item.DateTime.UnixMilli(), Date: item.DateTime.Format("2006-01-02"), Open: item.Open, High: item.High, Low: item.Low, Close: item.Close, Volume: float64(item.Vol), Turnover: item.Amount, ChangePercent: item.RiseRate, ChangeAmount: item.RisePrice, TurnoverRate: item.Turnover, Amplitude: amplitude}
}

// v1ExBar 将 GOTDX 扩展市场 K 线映射为统一字段。
func v1ExBar(item proto.ExKLineItem) v1BarItem {
	amplitude := 0.0
	if item.Open != 0 {
		amplitude = (item.High - item.Low) / item.Open * 100
	}
	return v1BarItem{Timestamp: item.DateTime.UnixMilli(), Date: item.DateTime.Format("2006-01-02"), Open: item.Open, High: item.High, Low: item.Low, Close: item.Close, Volume: float64(item.Vol), Turnover: item.Amount, Amplitude: amplitude}
}

// handleV1TimeShare 拉取并映射统一分时序列。
func handleV1TimeShare(c *gin.Context) {
	var req v1TimeShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.SourceID != v1SourceID || req.Instrument.Symbol == "" {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "sourceId and instrument are required")
		return
	}
	date, err := v1DateNumber(req.TradingDate)
	if err != nil {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid tradingDate")
		return
	}
	ref := req.Instrument.ProviderRef
	var response stockHistoryTickResponse
	if category, hasCategory := v1Number(ref.Category); hasCategory {
		tick, fetchErr := fetchExHistoryTick(date, category, req.Instrument.Symbol)
		if fetchErr != nil {
			err = fetchErr
		} else {
			response, err = buildExHistoryTickResponse(
				exHistoryTickRequest{Date: date, Category: category, Code: req.Instrument.Symbol},
				tick,
				newDefaultExTimeSharePreCloseSource(),
			)
		}
	} else if market, hasMarket := v1Number(ref.Market); hasMarket {
		tick, fetchErr := fetchStockHistoryTick(date, market, req.Instrument.Symbol)
		if fetchErr != nil {
			err = fetchErr
		} else {
			response, err = buildStockHistoryTickResponse(
				stockHistoryTickRequest{Date: date, Market: market, Code: req.Instrument.Symbol, Kind: ref.Kind},
				tick,
				newDefaultTimeSharePreCloseSource(),
			)
		}
	} else {
		writeV1Error(c, http.StatusBadRequest, "INVALID_REQUEST", "providerRef.market or category is required")
		return
	}
	if err != nil {
		status, code := http.StatusBadGateway, "UPSTREAM_UNAVAILABLE"
		if errors.Is(err, errTimeShareDataUnavailable) {
			status, code = http.StatusUnprocessableEntity, "UNSUPPORTED_CAPABILITY"
		}
		writeV1Error(c, status, code, err.Error())
		return
	}
	items := make([]v1TimeShareItem, 0, len(response.Data))
	for _, item := range response.Data {
		timestamp, parseErr := time.Parse(time.RFC3339, item.Timestamp)
		if parseErr != nil {
			writeV1Error(c, http.StatusBadGateway, "INVALID_RESPONSE", parseErr.Error())
			return
		}
		items = append(items, v1TimeShareItem{Timestamp: timestamp.UnixMilli(), Price: item.Price, Average: item.Avg, Volume: item.Volume, Amount: item.Amount})
	}
	writeV1Data(c, v1TimeShareSeries{InstrumentID: req.Instrument.ID, TradingDate: req.TradingDate, Timezone: "Asia/Shanghai", PreClose: response.PreClose, Items: items})
}
