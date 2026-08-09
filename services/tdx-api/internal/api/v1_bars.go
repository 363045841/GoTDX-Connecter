// V1 协议 K 线接口：按 providerRef 的 kind 路由到股票/指数/扩展行情，并映射为 V1 序列。
package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// v1BarRequest V1 K 线请求；from/to 为 UTC Unix 毫秒时间戳，含边界。
type v1BarRequest struct {
	SourceID   string                `json:"sourceId"`
	Instrument v1InstrumentReference `json:"instrument"`
	Period     string                `json:"period"`
	Adjustment string                `json:"adjustment"`
	From       int64                 `json:"from"`
	To         int64                 `json:"to"`
}

// v1BarSeries V1 K 线序列。
type v1BarSeries struct {
	InstrumentID string        `json:"instrumentId"`
	Period       string        `json:"period"`
	Adjustment   string        `json:"adjustment"`
	Timezone     string        `json:"timezone"`
	VolumeUnit   string        `json:"volumeUnit,omitempty"`
	Items        []v1KLineItem `json:"items"`
}

// v1RangeToDates 将 UTC 毫秒区间转换为品种时区的日期边界（含整天）。
func v1RangeToDates(from, to int64, timezone string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	start := time.UnixMilli(from).In(loc)
	end := time.UnixMilli(to).In(loc)
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("to must be after from")
	}
	startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
	return startDate, endDate, nil
}

// handleV1Bars 拉取指定品种、周期和 UTC 区间的 K 线。
func handleV1Bars(c *gin.Context) {
	var req v1BarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Instrument.Symbol == "" {
		writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "instrument.symbol is required")
		return
	}
	timezone := v1ExchangeToTimezone(req.Instrument.Exchange)
	startDate, endDate, err := v1RangeToDates(req.From, req.To, timezone)
	if err != nil {
		writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, err.Error())
		return
	}
	ref := req.Instrument.ProviderRef
	kind := v1ProviderRefKind(ref)
	category, ok := v1PeriodToCategory[req.Period]
	if !ok {
		writeV1Error(c, http.StatusBadRequest, v1CodeUnsupportedCapability,
			fmt.Sprintf("period %q is not supported for kind %q", req.Period, kind))
		return
	}
	if !v1AdjustmentSupported(kind, req.Adjustment) {
		writeV1Error(c, http.StatusBadRequest, v1CodeUnsupportedCapability,
			fmt.Sprintf("adjustment %q is not supported for kind %q", req.Adjustment, kind))
		return
	}
	adjust := v1AdjustToUint[req.Adjustment]

	items := make([]v1KLineItem, 0)
	switch kind {
	case symbolKindIndex:
		market, ok := v1ProviderRefNumber(ref, "market")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.market is required for index")
			return
		}
		bars, err := IndexKLineRange(category, uint8(market), req.Instrument.Symbol, startDate, endDate)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
		for _, bar := range bars {
			items = append(items, securityBarToV1KLine(bar))
		}
	case symbolKindEx:
		cat, ok := v1ProviderRefNumber(ref, "category")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.category is required for ex")
			return
		}
		bars, err := ExKLineRange(uint8(cat), req.Instrument.Symbol, category, 1, startDate, endDate)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
		for _, bar := range bars {
			items = append(items, exKLineToV1KLine(bar))
		}
	default:
		market, ok := v1ProviderRefNumber(ref, "market")
		if !ok {
			writeV1Error(c, http.StatusBadRequest, v1CodeInvalidRequest, "providerRef.market is required")
			return
		}
		bars, err := StockKLineRange(category, uint8(market), req.Instrument.Symbol, 1, adjust, startDate, endDate)
		if err != nil {
			writeV1Error(c, http.StatusBadGateway, v1CodeUpstreamUnavailable, err.Error())
			return
		}
		for _, bar := range bars {
			items = append(items, securityBarToV1KLine(bar))
		}
	}

	writeV1Data(c, http.StatusOK, v1BarSeries{
		InstrumentID: req.Instrument.ID,
		Period:       req.Period,
		Adjustment:   req.Adjustment,
		Timezone:     timezone,
		Items:        items,
	})
}
