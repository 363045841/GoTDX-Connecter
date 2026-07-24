package api

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/bensema/gotdx/proto"
	"github.com/bensema/gotdx/types"
	"github.com/gin-gonic/gin"
)

// 搜索结果 params.kind：与 gotdx types 品种分类对齐，供 K 线路由使用
const (
	symbolKindStock = "stock"
	symbolKindIndex = "index"
	symbolKindEx    = "ex"
)

const (
	symbolDirectoryTTL        = 24 * time.Hour
	exDirectoryPageSize       = 1000
	defaultSymbolSearchLimit  = 20
	maxSymbolSearchLimit      = 100
	symbolDirectoryRetryDelay = time.Minute
)

type symbolDirectoryLoader interface {
	StockAll(market uint8) ([]proto.Security, error)
	ExCount() (uint32, error)
	ExList(start uint32, count uint16) ([]proto.ExListItem, error)
}

type gotdxSymbolDirectoryLoader struct{}

func (gotdxSymbolDirectoryLoader) StockAll(market uint8) ([]proto.Security, error) {
	return mainCall(func(c client.MainQuerier) ([]proto.Security, error) { return c.StockAll(market) })
}

func (gotdxSymbolDirectoryLoader) ExCount() (uint32, error) {
	return exCall(func(c client.ExQuerier) (uint32, error) { return c.ExCount() })
}

func (gotdxSymbolDirectoryLoader) ExList(start uint32, count uint16) ([]proto.ExListItem, error) {
	return exCall(func(c client.ExQuerier) ([]proto.ExListItem, error) { return c.ExList(start, count) })
}

type symbolSearchItem struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
	Exchange    string `json:"exchange"`
	Source      string `json:"source"`
	// Params 含 market|category 与 kind（stock|index|ex），前端原样带回拉 K 线
	Params map[string]any `json:"params"`
}

type symbolDirectoryCache struct {
	loader    symbolDirectoryLoader
	store     symbolDirectoryStore
	ttl       time.Duration
	now       func() time.Time
	mu        sync.Mutex
	entries   []symbolSearchItem
	loadedAt  time.Time
	retryAt   time.Time
	loaded    bool
	storeRead bool
}

func newSymbolDirectoryCache(loader symbolDirectoryLoader, store symbolDirectoryStore, ttl time.Duration) *symbolDirectoryCache {
	return &symbolDirectoryCache{loader: loader, store: store, ttl: ttl, now: time.Now}
}

func (c *symbolDirectoryCache) warmUp() error {
	log.Printf("symbol directory: warming up")
	_, err := c.directory()
	return err
}

func (c *symbolDirectoryCache) search(query string, limit int) ([]symbolSearchItem, error) {
	entries, err := c.directory()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	matches := make([]symbolSearchItem, 0)
	for _, entry := range entries {
		if symbolMatchRank(entry, needle) < 6 {
			matches = append(matches, entry)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return symbolMatchRank(matches[i], needle) < symbolMatchRank(matches[j], needle)
	})
	if limit < len(matches) {
		matches = matches[:limit]
	}
	return matches, nil
}

func (c *symbolDirectoryCache) directory() ([]symbolSearchItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadPersistedDirectory()
	now := c.now()
	if c.loaded && (now.Sub(c.loadedAt) < c.ttl || now.Before(c.retryAt)) {
		return c.entries, nil
	}
	log.Printf("symbol directory: fetching from gotdx (previous entries=%d)", len(c.entries))
	started := time.Now()
	entries, err := c.loadDirectory()
	if err != nil {
		if c.loaded {
			c.retryAt = now.Add(symbolDirectoryRetryDelay)
			log.Printf("symbol directory: gotdx fetch failed, using stale cache entries=%d age=%s err=%v",
				len(c.entries), now.Sub(c.loadedAt).Round(time.Second), err)
			return c.entries, nil
		}
		log.Printf("symbol directory: gotdx fetch failed with no cache: %v", err)
		return nil, err
	}
	elapsed := time.Since(started).Round(time.Millisecond)
	if c.store != nil {
		if err := c.store.Replace(symbolDirectorySnapshot{Entries: entries, LoadedAt: now}); err != nil {
			log.Printf("symbol directory database write failed: %v", err)
		} else {
			log.Printf("symbol directory: persisted %d entries to sqlite in %s", len(entries), elapsed)
		}
	}
	log.Printf("symbol directory: loaded %d entries from gotdx in %s", len(entries), elapsed)
	c.entries = entries
	c.loadedAt = now
	c.retryAt = time.Time{}
	c.loaded = true
	return c.entries, nil
}

func (c *symbolDirectoryCache) loadPersistedDirectory() {
	if c.storeRead || c.store == nil {
		return
	}
	c.storeRead = true
	snapshot, found, err := c.store.Load()
	if err != nil {
		log.Printf("symbol directory database read failed: %v", err)
		return
	}
	if found {
		c.entries = snapshot.Entries
		c.loadedAt = snapshot.LoadedAt
		c.loaded = true
		age := c.now().Sub(snapshot.LoadedAt).Round(time.Second)
		log.Printf("symbol directory: loaded %d entries from sqlite (age=%s)", len(snapshot.Entries), age)
	} else {
		log.Printf("symbol directory: no sqlite snapshot found")
	}
}

func (c *symbolDirectoryCache) loadDirectory() ([]symbolSearchItem, error) {
	entries := make([]symbolSearchItem, 0)
	seen := make(map[string]struct{})
	for _, market := range []uint8{0, 1, 2} {
		stocks, err := c.loader.StockAll(market)
		if err != nil {
			return nil, err
		}
		for _, stock := range stocks {
			exchange := mainExchange(market)
			item := symbolSearchItem{
				Symbol: stock.Code, Description: stock.Name, Exchange: exchange, Source: "gotdx",
				Params: map[string]any{
					"market": market,
					"kind":   mainMarketSymbolKind(stock.Code, exchange),
				},
			}
			appendUniqueSymbol(&entries, seen, "market", market, item)
		}
	}

	count, err := c.loader.ExCount()
	if err != nil {
		return nil, err
	}
	for start := uint32(0); start < count; start += exDirectoryPageSize {
		pageSize := uint32(exDirectoryPageSize)
		if remaining := count - start; remaining < pageSize {
			pageSize = remaining
		}
		items, err := c.loader.ExList(start, uint16(pageSize))
		if err != nil {
			return nil, err
		}
		for _, ex := range items {
			item := symbolSearchItem{
				Symbol: ex.Code, Description: ex.Name, Exchange: extendedExchange(ex.Market), Source: "gotdx",
				Params: map[string]any{
					"category": ex.Category,
					"kind":     symbolKindEx,
				},
			}
			appendUniqueSymbol(&entries, seen, "category", ex.Category, item)
		}
	}
	return entries, nil
}

func appendUniqueSymbol(entries *[]symbolSearchItem, seen map[string]struct{}, kind string, market uint8, item symbolSearchItem) {
	key := fmt.Sprintf("%s:%s:%d:%s", item.Source, kind, market, item.Symbol)
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*entries = append(*entries, item)
}

func symbolMatchRank(item symbolSearchItem, query string) int {
	code := strings.ToLower(item.Symbol)
	name := strings.ToLower(item.Description)
	switch {
	case code == query:
		return 0
	case strings.HasPrefix(code, query):
		return 1
	case strings.Contains(code, query):
		return 2
	case name == query:
		return 3
	case strings.HasPrefix(name, query):
		return 4
	case strings.Contains(name, query):
		return 5
	default:
		return 6
	}
}

func mainExchange(market uint8) string {
	switch market {
	case 0:
		return "SZ"
	case 1:
		return "SH"
	case 2:
		return "BJ"
	default:
		return fmt.Sprintf("CN-%d", market)
	}
}

// mainMarketSymbolKind 用 gotdx types.IsIndex(code.EXCHANGE) 判定主市场品种
func mainMarketSymbolKind(code, exchange string) string {
	// IsIndex 要求 9 位形如 000001.SH
	if types.IsIndex(fmt.Sprintf("%s.%s", code, strings.ToUpper(exchange))) {
		return symbolKindIndex
	}
	return symbolKindStock
}

func extendedExchange(market uint8) string {
	labels := map[uint8]string{1: "CN", 2: "HK", 3: "FUTURES", 4: "FX", 5: "INDEX", 6: "VALUATION", 7: "MONEY", 8: "FUND", 9: "MONEY_FUND", 10: "INDICATOR", 11: "MIRROR", 12: "OPTION", 13: "US", 14: "DE", 15: "SG"}
	if label, ok := labels[market]; ok {
		return label
	}
	return fmt.Sprintf("EX-%d", market)
}

type symbolSearchRequest struct {
	Query string `json:"query"`
	Limit *int   `json:"limit"`
}

func newSymbolSearchHandler(cache *symbolDirectoryCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req symbolSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
			return
		}
		query := strings.TrimSpace(req.Query)
		if query == "" {
			c.JSON(400, gin.H{"error": "query is required"})
			return
		}
		limit := defaultSymbolSearchLimit
		if req.Limit != nil && *req.Limit > 0 {
			limit = *req.Limit
		}
		if limit > maxSymbolSearchLimit {
			limit = maxSymbolSearchLimit
		}
		items, err := cache.search(query, limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, items)
	}
}
