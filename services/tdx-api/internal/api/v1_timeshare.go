// V1 协议分时接口，以及 A 股/指数/扩展行情分时点的公共构建函数。
// 旧 /history-tick 接口与 V1 timeshare 共用点构建逻辑，避免行为分叉。
package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

// errV1NoTimeShareData 指定日期无历史分时数据（扩展行情全零点）。
var errV1NoTimeShareData = errors.New("该日期暂无历史分时数据")

// V1 分时可测注入点：复用主站/扩展行情历史分时拉取，测试可替换。
var (
	v1FetchStockHistoryTick = fetchStockHistoryTick
	v1FetchExHistoryTick    = fetchExHistoryTick
)

// timeSharePoint 分时点，时间保留为 time.Time 供不同序列化端（ISO 串 / unix 毫秒）使用。
type timeSharePoint struct {
	at     time.Time
	price  float64
	avg    float64
	volume *int
	amount *int64
}

// buildStockTimeSharePoints 构建 A 股/指数历史分时点，含开盘首笔补入与指数成交额换算。
func buildStockTimeSharePoints(
	req stockHistoryTickRequest,
	fetchTick func(date uint32, market uint8, code string) ([]proto.HistoryMinuteTimeData, error),
	now func() time.Time,
) ([]timeSharePoint, error) {
	tick, err := fetchTick(req.Date, req.Market, req.Code)
	if err != nil {
		return nil, err
	}
	year := int(req.Date / 10000)
	month := int((req.Date % 10000) / 100)
	day := int(req.Date % 100)
	loc := time.FixedZone("CST", 8*60*60)
	base := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
	isIndex := isIndexKLineRequest(req.Kind, req.Market, req.Code)

	var first *timeSharePoint
	if len(tick) > 0 {
		if trade, ok := fetchOpeningTrade(req.Date, req.Market, req.Code, now()); ok {
			p := &timeSharePoint{
				at:    base.Add(9*time.Hour + 30*time.Minute),
				price: trade.Price,
				avg:   trade.Price,
			}
			if isIndex {
				// 指数逐笔 Vol 为百元，统一转换为元再向前端传递。
				amount := int64(trade.Vol) * 100
				p.amount = &amount
			} else {
				volume := trade.Vol
				p.volume = &volume
			}
			first = p
		}
	}

	points := make([]timeSharePoint, 0, len(tick)+1)
	if first != nil {
		points = append(points, *first)
	}
	for i, item := range tick {
		var t time.Time
		if i < 120 {
			t = base.Add(9*time.Hour + 31*time.Minute + time.Duration(i)*time.Minute)
		} else {
			t = base.Add(13*time.Hour + 1*time.Minute + time.Duration(i-120)*time.Minute)
		}
		p := timeSharePoint{at: t, price: item.Price, avg: item.Avg}
		if isIndex {
			// 指数分钟 Vol 为万元，统一转换为元；首分钟包含集合竞价。
			amount := int64(item.Vol) * 10_000
			if i == 0 && first != nil && first.amount != nil {
				amount = max(0, amount-*first.amount)
			}
			p.amount = &amount
		} else {
			volume := item.Vol
			if i == 0 && first != nil && first.volume != nil {
				// gotdx 首分钟包含集合竞价，补出 09:30 后需从 09:31 扣除以避免重复。
				volume = max(0, volume-*first.volume)
			}
			p.volume = &volume
		}
		points = append(points, p)
	}
	return points, nil
}

// buildExTimeSharePoints 构建扩展行情分时点，全零点视为无历史数据。
func buildExTimeSharePoints(
	req exHistoryTickRequest,
	fetchTick func(date uint32, category uint8, code string) ([]proto.ExTickChartData, error),
) ([]timeSharePoint, error) {
	tick, err := fetchTick(req.Date, req.Category, req.Code)
	if err != nil {
		return nil, err
	}
	if len(tick) > 0 {
		allZero := true
		for _, item := range tick {
			if item.Price != 0 || item.Avg != 0 || item.Vol != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			return nil, errV1NoTimeShareData
		}
	}

	year := int(req.Date / 10000)
	month := int((req.Date % 10000) / 100)
	day := int(req.Date % 100)
	loc := time.FixedZone("CST", 8*60*60)
	base := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)

	points := make([]timeSharePoint, 0, len(tick))
	for _, item := range tick {
		ts, err := parseExTickClock(base, item.Time)
		if err != nil {
			return nil, err
		}
		points = append(points, timeSharePoint{at: ts, price: item.Price, avg: item.Avg})
	}
	return points, nil
}

// normalizeStockPreCloseSource 填充分时昨收源的默认依赖（now/quote/dailyBars）。
func normalizeStockPreCloseSource(
	src timeSharePreCloseSource,
	req stockHistoryTickRequest,
	isIndex bool,
) timeSharePreCloseSource {
	if src.now == nil {
		src.now = time.Now
	}
	if src.quote == nil {
		src.quote = newDefaultTimeSharePreCloseSource().quote
	}
	if src.dailyBars == nil {
		if isIndex {
			indexMarket, indexCode := req.Market, req.Code
			src.dailyBars = func(_ uint8, _ string, date time.Time) ([]proto.SecurityBar, error) {
				return IndexKLineRange(4, indexMarket, indexCode, date, date)
			}
		} else {
			src.dailyBars = func(market uint8, code string, date time.Time) ([]proto.SecurityBar, error) {
				return StockKLineRange(4, market, code, 1, 0, date, date)
			}
		}
	}
	return src
}

// v1TimeShareRequest V1 分时请求；tradingDate 为品种时区内的 YYYY-MM-DD 交易日。
type v1TimeShareRequest struct {
	SourceID    string                `json:"sourceId"`
	Instrument  v1InstrumentReference `json:"instrument"`
	TradingDate string                `json:"tradingDate"`
}

// v1TimeShareItem V1 分时条目。
type v1TimeShareItem struct {
	Timestamp int64   `json:"timestamp"`
	Price     float64 `json:"price"`
	Average   float64 `json:"average"`
	Volume    *int    `json:"volume,omitempty"`
	Amount    *int64  `json:"amount,omitempty"`
}

// v1TimeShareSeries V1 分时序列。
type v1TimeShareSeries struct {
	InstrumentID string            `json:"instrumentId"`
	TradingDate  string            `json:"tradingDate"`
	Timezone     string            `json:"timezone"`
	PreClose     float64           `json:"preClose"`
	VolumeUnit   string            `json:"volumeUnit,omitempty"`
	Items        []v1TimeShareItem `json:"items"`
}

// v1ParseTradingDate 解析 YYYY-MM-DD 交易日为 YYYYMMDD 整数。
func v1ParseTradingDate(value string) (uint32, error) {
	t, err := time.ParseInLocation("2006-01-02", value, time.FixedZone("CST", 8*60*60))
	if err != nil {
		return 0, err
	}
	return uint32(t.Year()*10000 + int(t.Month())*100 + t.Day()), nil
}

// handleV1Timeshare 拉取指定品种在单个交易日内的分时序列。
func handleV1Timeshare(c *gin.Context) {
	var req v1TimeShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "invalid JSON: "+err.Error())
		return
	}
	date, err := v1ParseTradingDate(req.TradingDate)
	if err != nil {
		writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "invalid tradingDate: "+err.Error())
		return
	}
	timezone := v1ExchangeToTimezone(req.Instrument.Exchange)
	ref := req.Instrument.ProviderRef

	var points []timeSharePoint
	var preClose float64
	switch v1ProviderRefKind(ref) {
	case symbolKindEx:
		cat, ok := v1ProviderRefNumber(ref, "category")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.category is required for ex")
			return
		}
		points, err = buildExTimeSharePoints(
			exHistoryTickRequest{Date: date, Category: uint8(cat), Code: req.Instrument.Symbol},
			v1FetchExHistoryTick,
		)
		if err != nil {
			if errors.Is(err, errV1NoTimeShareData) {
				writeV1Error(c, http.StatusNotFound, v1CodeInstrumentNotFound, err.Error())
			} else {
				writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			}
			return
		}
		preClose, err = resolveExTimeSharePreClose(uint8(cat), req.Instrument.Symbol, date, newDefaultExTimeSharePreCloseSource())
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
	default:
		market, ok := v1ProviderRefNumber(ref, "market")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.market is required")
			return
		}
		kind := v1ProviderRefKind(ref)
		stockReq := stockHistoryTickRequest{
			Date: date, Market: uint8(market), Code: req.Instrument.Symbol, Kind: kind,
		}
		points, err = buildStockTimeSharePoints(stockReq, v1FetchStockHistoryTick, time.Now)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
		isIndex := isIndexKLineRequest(kind, uint8(market), req.Instrument.Symbol)
		preClose, err = resolveTimeSharePreClose(
			uint8(market), req.Instrument.Symbol, date,
			normalizeStockPreCloseSource(newDefaultTimeSharePreCloseSource(), stockReq, isIndex),
		)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
	}

	items := make([]v1TimeShareItem, 0, len(points))
	for _, p := range points {
		items = append(items, v1TimeShareItem{
			Timestamp: p.at.UnixMilli(),
			Price:     p.price,
			Average:   p.avg,
			Volume:    p.volume,
			Amount:    p.amount,
		})
	}
	writeV1Data(c, http.StatusOK, v1TimeShareSeries{
		InstrumentID: req.Instrument.ID,
		TradingDate:  req.TradingDate,
		Timezone:     timezone,
		PreClose:     preClose,
		Items:        items,
	})
}
