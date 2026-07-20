package binance

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Level 单档价格/数量（float64，前端直接消费）
type Level [2]float64

// depthEvent Binance Diff Depth Stream (@depth) JSON
type depthEvent struct {
	EventType string      `json:"e"`
	EventTime int64       `json:"E"`
	Symbol    string      `json:"s"`
	FirstID   int64       `json:"U"`
	FinalID   int64       `json:"u"`
	Bids      [][2]string `json:"b"`
	Asks      [][2]string `json:"a"`
}

// DeltaEntry SSE 增量推送单元
type DeltaEntry struct {
	Side      string  `json:"side"`
	Price     float64 `json:"price"`
	Size      float64 `json:"size"`
	Timestamp int64   `json:"timestamp"`
}

// restDepth REST /api/v3/depth 响应
type restDepth struct {
	LastUpdateID int64        `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"`
	Asks         [][2]string `json:"asks"`
}

// DepthHub 管理所有交易对的深度订阅
type DepthHub struct {
	mu    sync.Mutex
	books map[string]*depthBook
}

func NewDepthHub() *DepthHub {
	return &DepthHub{
		books: make(map[string]*depthBook),
	}
}

// Subscribe 订阅 symbol 的深度事件流，返回只读 channel
func (dh *DepthHub) Subscribe(symbol string) (chan []byte, error) {
	s := strings.ToLower(symbol)

	dh.mu.Lock()
	db, ok := dh.books[s]
	if !ok {
		db = newDepthBook(s)
		dh.books[s] = db
		go db.run()
	}
	dh.mu.Unlock()

	return db.subscribe()
}

func (dh *DepthHub) Unsubscribe(symbol string, ch chan []byte) {
	s := strings.ToLower(symbol)
	dh.mu.Lock()
	db, ok := dh.books[s]
	dh.mu.Unlock()
	if ok {
		db.unsubscribe(ch)
	}
}

// depthBook 单个交易对的深度管理
type depthBook struct {
	symbol string

	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	bids   map[string]float64
	asks   map[string]float64
	lastID int64
	ready  bool

	stopCh chan struct{}
}

func newDepthBook(symbol string) *depthBook {
	return &depthBook{
		symbol: symbol,
		subs:   make(map[chan []byte]struct{}),
		bids:   make(map[string]float64),
		asks:   make(map[string]float64),
		stopCh: make(chan struct{}),
	}
}

func (db *depthBook) subscribe() (chan []byte, error) {
	ch := make(chan []byte, 256)

	db.mu.Lock()
	db.subs[ch] = struct{}{}
	if db.ready {
		data := db.buildSnapshotJSON()
		select {
		case ch <- data:
		default:
			log.Printf("[%s] snapshot channel full, dropping", db.symbol)
		}
	}
	db.mu.Unlock()

	return ch, nil
}

func (db *depthBook) unsubscribe(ch chan []byte) {
	db.mu.Lock()
	delete(db.subs, ch)
	db.mu.Unlock()
}

func (db *depthBook) run() {
	for {
		db.connect()
		select {
		case <-db.stopCh:
			return
		default:
			time.Sleep(3 * time.Second)
		}
	}
}

func (db *depthBook) connect() {
	url := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth", db.symbol)
	d := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, _, err := d.Dial(url, nil)
	if err != nil {
		log.Printf("[%s] ws dial error: %v", db.symbol, err)
		return
	}
	defer conn.Close()
	log.Printf("[%s] depth ws connected", db.symbol)

	evtCh := make(chan depthEvent, 512)
	errCh := make(chan error, 1)

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			var evt depthEvent
			if err := json.Unmarshal(msg, &evt); err != nil {
				continue
			}
			evtCh <- evt
		}
	}()

	snap, err := db.fetchSnapshot()
	if err != nil {
		log.Printf("[%s] rest snapshot error: %v", db.symbol, err)
		return
	}

	db.mu.Lock()
	for k := range db.bids {
		delete(db.bids, k)
	}
	for k := range db.asks {
		delete(db.asks, k)
	}
	for _, b := range snap.Bids {
		size, _ := strconv.ParseFloat(b[1], 64)
		if size > 0 {
			db.bids[b[0]] = size
		}
	}
	for _, a := range snap.Asks {
		size, _ := strconv.ParseFloat(a[1], 64)
		if size > 0 {
			db.asks[a[0]] = size
		}
	}
	lastID := snap.LastUpdateID
	db.ready = false
	db.mu.Unlock()

	var buf []depthEvent
	synced := false
	syncTimer := time.After(5 * time.Second)

	for !synced {
		select {
		case evt := <-evtCh:
			if evt.FinalID <= lastID {
				continue
			}
			buf = append(buf, evt)
			if evt.FirstID <= lastID+1 && evt.FinalID >= lastID+1 {
				synced = true
			}
		case err := <-errCh:
			log.Printf("[%s] ws disconnected during sync: %v", db.symbol, err)
			return
		case <-syncTimer:
			log.Printf("[%s] sync timeout, forcing ready", db.symbol)
			synced = true
		}
	}

	db.mu.Lock()
	for _, be := range buf {
		if be.FinalID <= lastID {
			continue
		}
		db.applyDelta(&be)
		lastID = be.FinalID
	}
	db.lastID = lastID
	db.ready = true
	snapJSON := db.buildSnapshotJSON()
	db.mu.Unlock()

	for ch := range db.subs {
		select {
		case ch <- snapJSON:
		default:
		}
	}

	for {
		select {
		case evt := <-evtCh:
			if evt.FinalID <= db.lastID {
				continue
			}
			db.mu.Lock()
			db.applyDelta(&evt)
			db.lastID = evt.FinalID
			entries := makeDeltaEntries(&evt)
			if len(entries) > 0 {
				data, _ := json.Marshal(map[string]any{
					"type":    "delta",
					"entries": entries,
				})
				for ch := range db.subs {
					select {
					case ch <- data:
					default:
					}
				}
			}
			db.mu.Unlock()
		case err := <-errCh:
			log.Printf("[%s] depth ws disconnected: %v", db.symbol, err)
			return
		}
	}
}

func (db *depthBook) applyDelta(evt *depthEvent) {
	for _, b := range evt.Bids {
		size, _ := strconv.ParseFloat(b[1], 64)
		if size == 0 {
			delete(db.bids, b[0])
		} else {
			db.bids[b[0]] = size
		}
	}
	for _, a := range evt.Asks {
		size, _ := strconv.ParseFloat(a[1], 64)
		if size == 0 {
			delete(db.asks, a[0])
		} else {
			db.asks[a[0]] = size
		}
	}
}

func (db *depthBook) fetchSnapshot() (*restDepth, error) {
	url := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=100",
		strings.ToUpper(db.symbol))

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var snap restDepth
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (db *depthBook) buildSnapshotJSON() []byte {
	bids := db.topN(db.bids, 100, false)
	asks := db.topN(db.asks, 100, true)

	data, _ := json.Marshal(map[string]any{
		"type":      "snapshot",
		"bids":      bids,
		"asks":      asks,
		"timestamp": time.Now().UnixMilli(),
	})
	return data
}

func (db *depthBook) topN(source map[string]float64, n int, asc bool) []Level {
	type item struct {
		price float64
		size  float64
	}
	items := make([]item, 0, len(source))
	for ps, s := range source {
		p, _ := strconv.ParseFloat(ps, 64)
		items = append(items, item{p, s})
	}
	if asc {
		sort.Slice(items, func(i, j int) bool { return items[i].price < items[j].price })
	} else {
		sort.Slice(items, func(i, j int) bool { return items[i].price > items[j].price })
	}
	if len(items) > n {
		items = items[:n]
	}
	out := make([]Level, len(items))
	for i, it := range items {
		out[i] = Level{it.price, it.size}
	}
	return out
}

func makeDeltaEntries(evt *depthEvent) []DeltaEntry {
	entries := make([]DeltaEntry, 0, len(evt.Bids)+len(evt.Asks))
	for _, b := range evt.Bids {
		price, _ := strconv.ParseFloat(b[0], 64)
		size, _ := strconv.ParseFloat(b[1], 64)
		entries = append(entries, DeltaEntry{
			Side: "bid", Price: price, Size: size, Timestamp: evt.EventTime,
		})
	}
	for _, a := range evt.Asks {
		price, _ := strconv.ParseFloat(a[0], 64)
		size, _ := strconv.ParseFloat(a[1], 64)
		entries = append(entries, DeltaEntry{
			Side: "ask", Price: price, Size: size, Timestamp: evt.EventTime,
		})
	}
	return entries
}
