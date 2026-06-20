package api

import (
	"KlineChartQuantGo/internal/client"
	"github.com/bensema/gotdx"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
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

func handleMACBoardList(c *gin.Context) {
	var req macBoardListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACBoardList(req.BoardType, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACBoardMembers(c *gin.Context) {
	var req macBoardMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACBoardMembers(req.BoardSymbol, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACBoardMembersQuotes(c *gin.Context) {
	var req macBoardMembersQuotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACBoardMembersQuotes(req.BoardSymbol, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACBoardMembersQuotesDynamic(c *gin.Context) {
	var req macBoardMembersQuotesDynamicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACBoardMembersQuotesDynamic(req.BoardSymbol, req.Count, req.SortType, req.SortOrder, gotdx.DefaultMACBoardMembersQuotesFieldBitmap())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACSymbolQuotes(c *gin.Context) {
	var req macSymbolQuotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(req.Codes) == 0 {
		c.JSON(400, gin.H{"error": "codes are required"})
		return
	}
	data, err := client.Get().MACSymbolQuotes(req.Markets, req.Codes, gotdx.DefaultMACSymbolQuotesFieldBitmap())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACQuotes(c *gin.Context) {
	var req macQuotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACQuotes(req.Market, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACTransactions(c *gin.Context) {
	var req macTransactionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACTransactions(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACAuction(c *gin.Context) {
	var req macAuctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACAuction(req.Market, req.Code, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACTickCharts(c *gin.Context) {
	var req macTickChartsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACTickCharts(req.Market, req.Code, req.QueryDate, req.Days)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACSymbolInfo(c *gin.Context) {
	var req macSymbolInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACSymbolInfo(req.Market, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACCapitalFlow(c *gin.Context) {
	var req macCapitalFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACCapitalFlow(req.Market, req.Code)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

func handleMACMarketMonitor(c *gin.Context) {
	var req macMarketMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	data, err := client.Get().MACMarketMonitor(req.Market, req.Start, req.Count)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, data)
}

type macServerInfoResp struct {
	Market         uint8  `json:"market"`
	Today          string `json:"today"`
	LastTradingDay string `json:"last_trading_day"`
	IsTradingDay   bool   `json:"is_trading_day"`
	Flag           uint8  `json:"flag"`
}

func handleMACServerInfo(c *gin.Context) {
	info, err := retryWithReprobe(func() (*proto.MACServerInfoReply, error) {
		return client.Get().MACServerInfo()
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	resp := macServerInfoResp{
		Today:          info.Today,
		LastTradingDay: info.LastTradingDay,
		IsTradingDay:   len(info.Sessions1) > 0,
		Flag:           info.Flag,
	}
	c.JSON(200, resp)
}
