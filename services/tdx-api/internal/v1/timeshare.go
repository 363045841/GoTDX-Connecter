// V1 协议分时接口：委托 domain 的点构建与昨收解析，映射为 V1 序列。
package v1

import (
	"errors"
	"net/http"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/domain"
	"github.com/gin-gonic/gin"
)

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

	var points []domain.TimeSharePoint
	var preClose float64
	switch v1ProviderRefKind(ref) {
	case domain.KindEx:
		cat, ok := v1ProviderRefNumber(ref, "category")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.category is required for ex")
			return
		}
		points, err = domain.BuildExTimeSharePoints(
			domain.ExTimeShareRequest{Date: date, Category: uint8(cat), Code: req.Instrument.Symbol},
			domain.FetchExHistoryTick,
		)
		if err != nil {
			if errors.Is(err, domain.ErrNoTimeShareData) {
				writeV1Error(c, http.StatusNotFound, v1CodeInstrumentNotFound, err.Error())
			} else {
				writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			}
			return
		}
		preClose, err = domain.ResolveExPreClose(uint8(cat), req.Instrument.Symbol, date, domain.NewDefaultExPreCloseSource())
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
		stockReq := domain.StockTimeShareRequest{
			Date: date, Market: uint8(market), Code: req.Instrument.Symbol, Kind: kind,
		}
		points, err = domain.BuildStockTimeSharePoints(stockReq, domain.FetchStockHistoryTick, time.Now)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
		isIndex := domain.IsIndexKind(kind, uint8(market), req.Instrument.Symbol)
		preClose, err = domain.ResolveStockPreClose(
			uint8(market), req.Instrument.Symbol, date,
			domain.NormalizeStockPreCloseSource(domain.NewDefaultStockPreCloseSource(), stockReq, isIndex),
		)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
	}

	items := make([]v1TimeShareItem, 0, len(points))
	for _, p := range points {
		items = append(items, v1TimeShareItem{
			Timestamp: p.At.UnixMilli(),
			Price:     p.Price,
			Average:   p.Avg,
			Volume:    p.Volume,
			Amount:    p.Amount,
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
