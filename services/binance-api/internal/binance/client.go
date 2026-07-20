package binance

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// [price, quantity] 订单簿条目
type OrderBookEntry [2]string

// 订单簿快照（完整前 20 档）
type OrderBook struct {
	Symbol    string           `json:"symbol"`
	Bids      []OrderBookEntry `json:"bids"`
	Asks      []OrderBookEntry `json:"asks"`
	Timestamp int64            `json:"timestamp"`
}

// Binance Partial Book Depth Stream JSON 结构
type depthData struct {
	LastUpdateId int64            `json:"lastUpdateId"`
	Bids         []OrderBookEntry `json:"bids"`
	Asks         []OrderBookEntry `json:"asks"`
}

// binance WebSocket 客户端，管理多交易对订单簿缓存
type Client struct {
	mu      sync.RWMutex
	books   map[string]*OrderBook
	symbols []string
}

// 新建客户端，symbols 格式: "btcusdt,ethusdt"
func NewClient(symbols string) *Client {
	list := strings.Split(symbols, ",")
	for i, s := range list {
		list[i] = strings.TrimSpace(strings.ToLower(s))
	}
	return &Client{
		books:   make(map[string]*OrderBook),
		symbols: list,
	}
}

// 为每个交易对启动独立 WebSocket 协程
func (c *Client) Start() {
	for _, s := range c.symbols {
		go func(symbol string) {
			for {
				c.connectOne(symbol)
				log.Printf("binance %s ws disconnected, reconnecting in 3s...", symbol)
				time.Sleep(3 * time.Second)
			}
		}(s)
	}
}

// 连接单个交易对的 depth20@100ms 全量快照流
func (c *Client) connectOne(symbol string) {
	url := "wss://stream.binance.com:9443/ws/" + symbol + "@depth20@100ms"

	// 自动读取 HTTP_PROXY 环境变量走代理
	d := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}

	conn, _, err := d.Dial(url, nil)
	if err != nil {
		log.Printf("binance %s ws dial error: %v", symbol, err)
		return
	}
	defer conn.Close()

	log.Printf("connected to binance: %s", url)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("binance %s ws read error: %v", symbol, err)
			return
		}

		var d depthData
		if err := json.Unmarshal(msg, &d); err != nil {
			log.Printf("binance %s ws unmarshal error: %v", symbol, err)
			continue
		}

		// 收到完整快照直接覆盖缓存
		book := &OrderBook{
			Symbol:    strings.ToUpper(symbol),
			Bids:      d.Bids,
			Asks:      d.Asks,
			Timestamp: time.Now().UnixMilli(),
		}

		c.mu.Lock()
		c.books[strings.ToUpper(symbol)] = book
		c.mu.Unlock()
	}
}

// 线程安全读取订单簿快照
func (c *Client) GetOrderBook(symbol string) *OrderBook {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.books[strings.ToUpper(symbol)]
}
