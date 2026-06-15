package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx/proto"
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

func handleStockQuotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Markets) == 0 || len(req.Codes) == 0 {
		writeError(w, http.StatusBadRequest, "markets and codes are required")
		return
	}
	stocks, err := client.Get().StockQuotesDetail(req.Markets, req.Codes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stocks)
}

func handleStockKLine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockKLineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	klines, err := client.Get().StockKLine(req.Category, req.Market, req.Code, req.Start, req.Count, req.Times, req.Adjust)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, klines)
}

func handleStockTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockTickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	tick, err := client.Get().StockTickChart(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tick)
}

func handleStockList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	stocks, err := client.Get().StockList(req.Market, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stocks)
}

func handleStockCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockCountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	count, err := client.Get().StockCount(req.Market)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]uint16{"count": count})
}

func handleStockIndexInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockIndexInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	info, err := client.Get().StockIndexInfo(req.Market, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func handleStockTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().StockTransaction(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleStockHistoryTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockHistoryTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().StockHistoryTransaction(req.Date, req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
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
			key := fmt.Sprintf("%d-%02d-%02d", k.Year, k.Month, k.Day)
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

func handleStockKLineCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockKLineCountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"count": 0,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": len(klines),
	})
}

func handleStockKLineByDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req stockKLineByDateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	start, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_date: "+err.Error())
		return
	}
	end, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_date: "+err.Error())
		return
	}
	if req.Times == 0 {
		req.Times = 1
	}

	klines, err := StockKLineRange(req.Category, req.Market, req.Code, req.Times, req.Adjust, start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[gotdx] stock/kline-by-date %s cat=%d count=%d range=[%s,%s]",
		req.Code, req.Category, len(klines), req.StartDate, req.EndDate)
	writeJSON(w, http.StatusOK, klines)
}
