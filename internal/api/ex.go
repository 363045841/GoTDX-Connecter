package api

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sort"
	"time"

	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx/proto"
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

func handleExCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	count, err := client.Get().ExCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]uint32{"count": count})
}

func handleExList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().ExList(req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleExQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exQuoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().ExQuote(req.Category, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleExQuotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Categories) == 0 || len(req.Codes) == 0 {
		writeError(w, http.StatusBadRequest, "categories and codes are required")
		return
	}
	data, err := client.Get().ExQuotes(req.Categories, req.Codes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleExKLine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exKLineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	klines, err := client.Get().ExKLine(req.Category, req.Code, req.Period, req.Start, req.Count, req.Times)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, klines)
}

func handleExTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exTickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	tick, err := client.Get().ExTickChart(req.Category, req.Code, req.Date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tick)
}

func handleExHistoryTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exHistoryTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().ExHistoryTransaction(req.Date, req.Category, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleExTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	data, err := client.Get().ExTable()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"table": data})
}

type exKLineByDateRequest struct {
	Category  uint8  `json:"category"`
	Code      string `json:"code"`
	Period    uint16 `json:"period"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Times     uint16 `json:"times"`
}

func ExKLineRange(category uint8, code string, period uint16, times uint16, startDate, endDate time.Time) ([]proto.ExKLineItem, error) {
	out := []proto.ExKLineItem{}
	seen := make(map[string]bool)
	end := endDate.Add(24 * time.Hour)

	for start := uint32(0); ; start += uint32(klinePageSize) {
		klines, err := safeExKLine(category, code, period, uint32(start), klinePageSize, times)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			break
		}

		for _, k := range klines {
			if seen[k.DateTime] {
				continue
			}
			seen[k.DateTime] = true
			t, err := time.ParseInLocation("2006-01-02", k.DateTime, time.Local)
			if err != nil {
				continue
			}
			if (t.Equal(startDate) || t.After(startDate)) && t.Before(end) {
				out = append(out, k)
			}
		}

		if len(klines) < int(klinePageSize) {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool {
		ti, _ := time.ParseInLocation("2006-01-02", out[i].DateTime, time.Local)
		tj, _ := time.ParseInLocation("2006-01-02", out[j].DateTime, time.Local)
		return ti.Before(tj)
	})
	return out, nil
}

type exKLineByDateResponse struct {
	proto.ExKLineItem
	Amplitude float64 `json:"Amplitude"`
}

func handleExKLineByDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req exKLineByDateRequest
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

	klines, err := ExKLineRange(req.Category, req.Code, req.Period, req.Times, start, end)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	writeJSON(w, http.StatusOK, resp)
}
