package api

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

const (
	symbolDirectoryTTL        = 30 * time.Minute
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
	return client.Get().StockAll(market)
}

func (gotdxSymbolDirectoryLoader) ExCount() (uint32, error) {
	return client.Get().ExCount()
}

func (gotdxSymbolDirectoryLoader) ExList(start uint32, count uint16) ([]proto.ExListItem, error) {
	return client.Get().ExList(start, count)
}

type symbolSearchItem struct {
	Symbol      string           `json:"symbol"`
	Description string           `json:"description"`
	Exchange    string           `json:"exchange"`
	Source      string           `json:"source"`
	Params      map[string]uint8 `json:"params"`
}

type symbolDirectoryCache struct {
	loader   symbolDirectoryLoader
	ttl      time.Duration
	now      func() time.Time
	mu       sync.Mutex
	entries  []symbolSearchItem
	loadedAt time.Time
	retryAt  time.Time
	loaded   bool
}

func newSymbolDirectoryCache(loader symbolDirectoryLoader, ttl time.Duration) *symbolDirectoryCache {
	return &symbolDirectoryCache{loader: loader, ttl: ttl, now: time.Now}
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
	now := c.now()
	if c.loaded && (now.Sub(c.loadedAt) < c.ttl || now.Before(c.retryAt)) {
		return c.entries, nil
	}
	entries, err := c.loadDirectory()
	if err != nil {
		if c.loaded {
			c.retryAt = now.Add(symbolDirectoryRetryDelay)
			return c.entries, nil
		}
		return nil, err
	}
	c.entries = entries
	c.loadedAt = now
	c.retryAt = time.Time{}
	c.loaded = true
	return c.entries, nil
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
			item := symbolSearchItem{
				Symbol: stock.Code, Description: stock.Name, Exchange: mainExchange(market), Source: "gotdx",
				Params: map[string]uint8{"market": market},
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
				Params: map[string]uint8{"category": ex.Category},
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

var defaultSymbolDirectoryCache = newSymbolDirectoryCache(gotdxSymbolDirectoryLoader{}, symbolDirectoryTTL)

func handleSymbolSearch(c *gin.Context) {
	newSymbolSearchHandler(defaultSymbolDirectoryCache)(c)
}
