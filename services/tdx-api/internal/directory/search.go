// 证券目录缓存与搜索：从 gotdx 主站/扩展行情加载目录，缓存并持久化，按关键字搜索。
package directory

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
)

const (
	defaultTTL            = 24 * time.Hour
	exDirectoryPageSize   = 1000
	defaultSearchLimit    = 20
	maxSearchLimit        = 100
	directoryRetryDelay   = time.Minute
)

// GotdxLoader 通过 gotdx 客户端加载证券目录。
type GotdxLoader struct{}

func (GotdxLoader) StockAll(market uint8) ([]proto.Security, error) {
	return mainCall(func(c client.MainQuerier) ([]proto.Security, error) { return c.StockAll(market) })
}

func (GotdxLoader) ExCount() (uint32, error) {
	return exCall(func(c client.ExQuerier) (uint32, error) { return c.ExCount() })
}

func (GotdxLoader) ExList(start uint32, count uint16) ([]proto.ExListItem, error) {
	return exCall(func(c client.ExQuerier) ([]proto.ExListItem, error) { return c.ExList(start, count) })
}

// Cache 证券目录缓存：TTL 内复用内存目录，过期刷新，持久化失败不影响可用性。
type Cache struct {
	loader    Loader
	store     Store
	ttl       time.Duration
	now       func() time.Time
	mu        sync.Mutex
	entries   []Item
	loadedAt  time.Time
	retryAt   time.Time
	loaded    bool
	storeRead bool
}

// NewCache 创建证券目录缓存。
func NewCache(loader Loader, store Store, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Cache{loader: loader, store: store, ttl: ttl, now: time.Now}
}

// WarmUp 启动时预热目录缓存。
func (c *Cache) WarmUp() error {
	log.Printf("symbol directory: warming up")
	_, err := c.directory()
	return err
}

// Search 按关键字搜索证券目录，返回按匹配度排序的条目。
func (c *Cache) Search(query string, limit int) ([]Item, error) {
	entries, err := c.directory()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	matches := make([]Item, 0)
	for _, entry := range entries {
		if matchRank(entry, needle) < 6 {
			matches = append(matches, entry)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matchRank(matches[i], needle) < matchRank(matches[j], needle)
	})
	if limit < len(matches) {
		matches = matches[:limit]
	}
	return matches, nil
}

func (c *Cache) directory() ([]Item, error) {
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
			c.retryAt = now.Add(directoryRetryDelay)
			log.Printf("symbol directory: gotdx fetch failed, using stale cache entries=%d age=%s err=%v",
				len(c.entries), now.Sub(c.loadedAt).Round(time.Second), err)
			return c.entries, nil
		}
		log.Printf("symbol directory: gotdx fetch failed with no cache: %v", err)
		return nil, err
	}
	elapsed := time.Since(started).Round(time.Millisecond)
	if c.store != nil {
		if err := c.store.Replace(Snapshot{Entries: entries, LoadedAt: now}); err != nil {
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

func (c *Cache) loadPersistedDirectory() {
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

func (c *Cache) loadDirectory() ([]Item, error) {
	entries := make([]Item, 0)
	seen := make(map[string]struct{})
	for _, market := range []uint8{0, 1, 2} {
		stocks, err := c.loader.StockAll(market)
		if err != nil {
			return nil, err
		}
		for _, stock := range stocks {
			exchange := mainExchange(market)
			item := Item{
				Symbol: stock.Code, Description: stock.Name, Exchange: exchange, Source: "gotdx",
				Params: map[string]any{
					"market": market,
					"kind":   mainMarketSymbolKind(stock.Code, exchange),
				},
			}
			appendUnique(&entries, seen, "market", market, item)
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
			item := Item{
				Symbol: ex.Code, Description: ex.Name, Exchange: extendedExchange(ex.Market), Source: "gotdx",
				Params: map[string]any{
					"category": ex.Category,
					"kind":     KindEx,
				},
			}
			appendUnique(&entries, seen, "category", ex.Category, item)
		}
	}
	return entries, nil
}

func appendUnique(entries *[]Item, seen map[string]struct{}, kind string, market uint8, item Item) {
	key := fmt.Sprintf("%s:%s:%d:%s", item.Source, kind, market, item.Symbol)
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*entries = append(*entries, item)
}

func matchRank(item Item, query string) int {
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
		return KindIndex
	}
	return KindStock
}

func extendedExchange(market uint8) string {
	labels := map[uint8]string{1: "CN", 2: "HK", 3: "FUTURES", 4: "FX", 5: "INDEX", 6: "VALUATION", 7: "MONEY", 8: "FUND", 9: "MONEY_FUND", 10: "INDICATOR", 11: "MIRROR", 12: "OPTION", 13: "US", 14: "DE", 15: "SG"}
	if label, ok := labels[market]; ok {
		return label
	}
	return fmt.Sprintf("EX-%d", market)
}

func mainCall[T any](operation func(client.MainQuerier) (T, error)) (T, error) {
	return client.QueryMain(operation)
}

func exCall[T any](operation func(client.ExQuerier) (T, error)) (T, error) {
	return client.QueryEx(operation)
}
