package api

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/stock/quotes", handleStockQuotes)
	mux.HandleFunc("/api/stock/kline", handleStockKLine)
	mux.HandleFunc("/api/stock/tick", handleStockTick)
	mux.HandleFunc("/api/stock/list", handleStockList)
	mux.HandleFunc("/api/stock/count", handleStockCount)
	mux.HandleFunc("/api/stock/index-info", handleStockIndexInfo)
	mux.HandleFunc("/api/stock/transaction", handleStockTransaction)
	mux.HandleFunc("/api/stock/history-transaction", handleStockHistoryTransaction)

	mux.HandleFunc("/api/ex/count", handleExCount)
	mux.HandleFunc("/api/ex/list", handleExList)
	mux.HandleFunc("/api/ex/quote", handleExQuote)
	mux.HandleFunc("/api/ex/quotes", handleExQuotes)
	mux.HandleFunc("/api/ex/kline", handleExKLine)
	mux.HandleFunc("/api/ex/tick", handleExTick)
	mux.HandleFunc("/api/ex/history-transaction", handleExHistoryTransaction)
	mux.HandleFunc("/api/ex/table", handleExTable)

	mux.HandleFunc("/api/mac/board-list", handleMACBoardList)
	mux.HandleFunc("/api/mac/board-members", handleMACBoardMembers)
	mux.HandleFunc("/api/mac/board-members-quotes", handleMACBoardMembersQuotes)
	mux.HandleFunc("/api/mac/board-members-quotes-dynamic", handleMACBoardMembersQuotesDynamic)
	mux.HandleFunc("/api/mac/symbol-quotes", handleMACSymbolQuotes)
	mux.HandleFunc("/api/mac/quotes", handleMACQuotes)
	mux.HandleFunc("/api/mac/transactions", handleMACTransactions)
	mux.HandleFunc("/api/mac/auction", handleMACAuction)
	mux.HandleFunc("/api/mac/tick-charts", handleMACTickCharts)
	mux.HandleFunc("/api/mac/symbol-info", handleMACSymbolInfo)
	mux.HandleFunc("/api/mac/capital-flow", handleMACCapitalFlow)
	mux.HandleFunc("/api/mac/market-monitor", handleMACMarketMonitor)

	mux.HandleFunc("/api/hosts/probe", handleHostProbe)
	mux.HandleFunc("/api/hosts/list", handleHostList)

	return recoveryMiddleware(corsMiddleware(mux))
}
