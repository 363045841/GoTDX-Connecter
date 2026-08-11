// A 股/指数/扩展行情 K 线拉取与按日期区间查询；fetch 变量为可测注入点。
package domain

import (
	"errors"
	"fmt"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/bensema/gotdx/proto"
)

// ErrNoKLineData 数据源对该品种完全无 K 线数据（如已到期期货），与"指定区间内无数据"区分。
// 上层据此返回可流转的确定性错误，触发前端请求流转。
var ErrNoKLineData = errors.New("no kline data for instrument")

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

// SafeStockKLine 拉取 A 股 K 线（带 panic 兜底），供 api 层的按数查询使用。
func SafeStockKLine(category uint16, market uint8, code string, start uint16, count uint16, times uint16, adjust uint16) ([]proto.SecurityBar, error) {
	return safeStockKLine(category, market, code, start, count, times, adjust)
}

// SafeIndexBars 拉取指数 K 线（带 panic 兜底），供 api 层的按数查询使用。
func SafeIndexBars(category uint16, market uint8, code string, start uint16, count uint16) ([]proto.SecurityBar, error) {
	return safeIndexBars(category, market, code, start, count)
}

// 各 K 线源单页拉取的可测注入点；生产默认走 safe*。
var (
	FetchStockKLinePage = safeStockKLine
	FetchIndexBarsPage  = safeIndexBars
	FetchExKLinePage    = safeExKLine
)

// StockKLineRange 按 StockKLine 分页拉取 A 股 K 线并按日期过滤
func StockKLineRange(category uint16, market uint8, code string, times uint16, adjust uint16, startDate, endDate time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateFromRecent(klinePageSize, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		return FetchStockKLinePage(category, market, code, s, count, times, adjust)
	}, securityBarOldest, startDate, false)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return filterKLineByDate(raw, startDate, endDate, true), nil
}

// StockKLineBefore 按 cursor 拉取股票 K 线，返回严格早于 before 的最新 limit 条升序数据。
func StockKLineBefore(category uint16, market uint8, code string, times uint16, adjust uint16, limit int, before *time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateBefore(klinePageSize, limit, before, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		return FetchStockKLinePage(category, market, code, s, count, times, adjust)
	}, securityBarOldest, false)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return limitSecurityBarsBefore(raw, before, limit), nil
}

// IndexKLineRange 按 GetIndexBars 分页拉取指数 K 线并按日期过滤
// 深页偶发 gotdx invalid kline datetime：先缩 count 再试，仍失败且已有数据则截断
func IndexKLineRange(category uint16, market uint8, code string, startDate, endDate time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateFromRecent(indexPageSize, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		klines, err := FetchIndexBarsPage(category, market, code, s, count)
		if err != nil && count > indexFallbackPageSize {
			return FetchIndexBarsPage(category, market, code, s, indexFallbackPageSize)
		}
		return klines, err
	}, securityBarOldest, startDate, true)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return filterKLineByDate(raw, startDate, endDate, true), nil
}

// IndexKLineBefore 按 cursor 拉取指数 K 线，返回严格早于 before 的最新 limit 条升序数据。
func IndexKLineBefore(category uint16, market uint8, code string, limit int, before *time.Time) ([]proto.SecurityBar, error) {
	raw, err := paginateBefore(indexPageSize, limit, before, func(start uint32, count uint16) ([]proto.SecurityBar, error) {
		s, ok := clampUint16Start(start)
		if !ok {
			return nil, nil
		}
		klines, err := FetchIndexBarsPage(category, market, code, s, count)
		if err != nil && count > indexFallbackPageSize {
			return FetchIndexBarsPage(category, market, code, s, indexFallbackPageSize)
		}
		return klines, err
	}, securityBarOldest, true)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return limitSecurityBarsBefore(raw, before, limit), nil
}

// ExKLineRange 按 ExKLine 分页拉取扩展行情并按日期过滤
func ExKLineRange(category uint8, code string, period uint16, times uint16, startDate, endDate time.Time) ([]proto.ExKLineItem, error) {
	raw, err := paginateFromRecent(klinePageSize, func(start uint32, count uint16) ([]proto.ExKLineItem, error) {
		return FetchExKLinePage(category, code, period, start, count, times)
	}, exKLineOldest, startDate, false)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return filterExKLineByDate(raw, startDate, endDate), nil
}

// ExKLineBefore 按 cursor 拉取扩展行情 K 线，返回严格早于 before 的最新 limit 条升序数据。
func ExKLineBefore(category uint8, code string, period uint16, times uint16, limit int, before *time.Time) ([]proto.ExKLineItem, error) {
	raw, err := paginateBefore(klinePageSize, limit, before, func(start uint32, count uint16) ([]proto.ExKLineItem, error) {
		return FetchExKLinePage(category, code, period, start, count, times)
	}, exKLineOldest, false)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, ErrNoKLineData
	}
	return limitExKLinesBefore(raw, before, limit), nil
}
