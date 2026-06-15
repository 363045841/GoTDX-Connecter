package api

import (
	"encoding/json"
	"net/http"

	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx"
)

type macBoardListRequest struct {
	BoardType uint16 `json:"board_type"`
	Count     uint32 `json:"count"`
}

type macBoardMembersRequest struct {
	BoardSymbol string `json:"board_symbol"`
	Count       uint32 `json:"count"`
}

type macBoardMembersQuotesRequest struct {
	BoardSymbol string `json:"board_symbol"`
	Count       uint32 `json:"count"`
}

type macBoardMembersQuotesDynamicRequest struct {
	BoardSymbol string `json:"board_symbol"`
	Count       uint32 `json:"count"`
	SortType    uint16 `json:"sort_type"`
	SortOrder   uint8  `json:"sort_order"`
}

type macSymbolQuotesRequest struct {
	Markets []uint8  `json:"markets"`
	Codes   []string `json:"codes"`
}

type macQuotesRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
}

type macTransactionsRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
	Start  uint32 `json:"start"`
	Count  uint32 `json:"count"`
}

type macAuctionRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
	Start  uint32 `json:"start"`
	Count  uint32 `json:"count"`
}

type macTickChartsRequest struct {
	Market    uint8  `json:"market"`
	Code      string `json:"code"`
	QueryDate uint32 `json:"query_date"`
	Days      uint16 `json:"days"`
}

type macSymbolInfoRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
}

type macCapitalFlowRequest struct {
	Market uint8  `json:"market"`
	Code   string `json:"code"`
}

type macMarketMonitorRequest struct {
	Market uint8  `json:"market"`
	Start  uint32 `json:"start"`
	Count  uint32 `json:"count"`
}

func handleMACBoardList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macBoardListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACBoardList(req.BoardType, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACBoardMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macBoardMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACBoardMembers(req.BoardSymbol, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACBoardMembersQuotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macBoardMembersQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACBoardMembersQuotes(req.BoardSymbol, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACBoardMembersQuotesDynamic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macBoardMembersQuotesDynamicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACBoardMembersQuotesDynamic(req.BoardSymbol, req.Count, req.SortType, req.SortOrder, gotdx.DefaultMACBoardMembersQuotesFieldBitmap())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACSymbolQuotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macSymbolQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Codes) == 0 {
		writeError(w, http.StatusBadRequest, "codes are required")
		return
	}
	data, err := client.Get().MACSymbolQuotes(req.Markets, req.Codes, gotdx.DefaultMACSymbolQuotesFieldBitmap())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACQuotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACQuotes(req.Market, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macTransactionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACTransactions(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACAuction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macAuctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACAuction(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACTickCharts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macTickChartsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACTickCharts(req.Market, req.Code, req.QueryDate, req.Days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACSymbolInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macSymbolInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACSymbolInfo(req.Market, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACCapitalFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macCapitalFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACCapitalFlow(req.Market, req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func handleMACMarketMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req macMarketMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	data, err := client.Get().MACMarketMonitor(req.Market, req.Start, req.Count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}
