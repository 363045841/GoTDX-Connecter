package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
	"github.com/gin-gonic/gin"
)

type fakeSymbolDirectoryLoader struct {
	stocks     map[uint8][]proto.Security
	ex         []proto.ExListItem
	err        error
	stockCalls int
	exCalls    int
}

type fakeSymbolDirectoryStore struct {
	snapshot     symbolDirectorySnapshot
	found        bool
	loadErr      error
	replaceErr   error
	loadCalls    int
	replaceCalls int
}

func (s *fakeSymbolDirectoryStore) Load() (symbolDirectorySnapshot, bool, error) {
	s.loadCalls++
	return s.snapshot, s.found, s.loadErr
}

func (s *fakeSymbolDirectoryStore) Replace(snapshot symbolDirectorySnapshot) error {
	s.replaceCalls++
	if s.replaceErr != nil {
		return s.replaceErr
	}
	s.snapshot = snapshot
	s.found = true
	return nil
}

func (l *fakeSymbolDirectoryLoader) StockAll(market uint8) ([]proto.Security, error) {
	l.stockCalls++
	if l.err != nil {
		return nil, l.err
	}
	return l.stocks[market], nil
}

func (l *fakeSymbolDirectoryLoader) ExCount() (uint32, error) {
	if l.err != nil {
		return 0, l.err
	}
	return uint32(len(l.ex)), nil
}

func (l *fakeSymbolDirectoryLoader) ExList(start uint32, count uint16) ([]proto.ExListItem, error) {
	l.exCalls++
	if l.err != nil {
		return nil, l.err
	}
	end := int(start) + int(count)
	if end > len(l.ex) {
		end = len(l.ex)
	}
	if int(start) >= len(l.ex) {
		return nil, nil
	}
	return l.ex[start:end], nil
}

func newSearchTestCache(loader *fakeSymbolDirectoryLoader, now *time.Time) *symbolDirectoryCache {
	cache := newSymbolDirectoryCache(loader, nil, 30*time.Minute)
	cache.now = func() time.Time { return *now }
	return cache
}

func TestSymbolDirectoryCacheUsesFreshPersistedSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	loader := &fakeSymbolDirectoryLoader{err: errors.New("loader must not be called")}
	store := &fakeSymbolDirectoryStore{snapshot: symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: now.Add(-time.Hour),
	}, found: true}
	cache := newSymbolDirectoryCache(loader, store, 24*time.Hour)
	cache.now = func() time.Time { return now }

	items, err := cache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, err:%v", items, err)
	}
	if loader.stockCalls != 0 || store.loadCalls != 1 || store.replaceCalls != 0 {
		t.Fatalf("calls = loader:%d load:%d replace:%d", loader.stockCalls, store.loadCalls, store.replaceCalls)
	}
}

func TestSymbolDirectoryCacheUsesStalePersistedSnapshotWhenRefreshFails(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	loader := &fakeSymbolDirectoryLoader{err: errors.New("TDX unavailable")}
	store := &fakeSymbolDirectoryStore{snapshot: symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: now.Add(-25 * time.Hour),
	}, found: true}
	cache := newSymbolDirectoryCache(loader, store, 24*time.Hour)
	cache.now = func() time.Time { return now }

	items, err := cache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, err:%v", items, err)
	}
	if loader.stockCalls != 1 || store.loadCalls != 1 || store.replaceCalls != 0 {
		t.Fatalf("calls = loader:%d load:%d replace:%d", loader.stockCalls, store.loadCalls, store.replaceCalls)
	}
}

func TestSymbolDirectoryCachePersistsSuccessfulRefresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{
		0: {{Code: "000001", Name: "Ping An"}},
	}}
	store := &fakeSymbolDirectoryStore{}
	cache := newSymbolDirectoryCache(loader, store, 24*time.Hour)
	cache.now = func() time.Time { return now }

	items, err := cache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, err:%v", items, err)
	}
	if store.loadCalls != 1 || store.replaceCalls != 1 {
		t.Fatalf("store calls = load:%d replace:%d", store.loadCalls, store.replaceCalls)
	}
	if !store.found || !store.snapshot.LoadedAt.Equal(now) || !reflect.DeepEqual(store.snapshot.Entries, cache.entries) {
		t.Fatalf("persisted snapshot = %#v", store.snapshot)
	}
}

func TestSymbolDirectoryCacheIgnoresStoreReadAndWriteFailures(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{
		0: {{Code: "000001", Name: "Ping An"}},
	}}
	store := &fakeSymbolDirectoryStore{
		loadErr:    errors.New("read failed"),
		replaceErr: errors.New("write failed"),
	}
	cache := newSymbolDirectoryCache(loader, store, 24*time.Hour)
	cache.now = func() time.Time { return now }

	items, err := cache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, err:%v", items, err)
	}
	if loader.stockCalls != 3 || store.loadCalls != 1 || store.replaceCalls != 1 {
		t.Fatalf("calls = loader:%d load:%d replace:%d", loader.stockCalls, store.loadCalls, store.replaceCalls)
	}
}

func TestSymbolDirectoryCacheReusesSQLiteSnapshotAcrossInstances(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "symbols.db")
	firstStore, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	firstLoader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{
		0: {{Code: "000001", Name: "Ping An"}},
	}}
	firstCache := newSymbolDirectoryCache(firstLoader, firstStore, 24*time.Hour)
	firstCache.now = func() time.Time { return now }
	if _, err := firstCache.search("000001", 20); err != nil {
		t.Fatalf("populate first cache: %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })
	secondLoader := &fakeSymbolDirectoryLoader{err: errors.New("loader must not be called")}
	secondCache := newSymbolDirectoryCache(secondLoader, secondStore, 24*time.Hour)
	secondCache.now = func() time.Time { return now }

	items, err := secondCache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search second cache = %#v, err:%v", items, err)
	}
	if secondLoader.stockCalls != 0 {
		t.Fatalf("second loader stock calls = %d, want 0", secondLoader.stockCalls)
	}
}

func TestSymbolSearchRanksCodeBeforeNameAndAppliesLimit(t *testing.T) {
	now := time.Unix(0, 0)
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{
		0: {
			{Code: "ABC", Name: "other"},
			{Code: "ABCD", Name: "other"},
			{Code: "XABC", Name: "other"},
			{Code: "001", Name: "ABC"},
			{Code: "002", Name: "ABCD name"},
			{Code: "003", Name: "name ABC tail"},
		},
	}}
	cache := newSearchTestCache(loader, &now)

	items, err := cache.search("aBc", 4)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	got := make([]string, len(items))
	for i, item := range items {
		got[i] = item.Symbol
	}
	want := []string{"ABC", "ABCD", "XABC", "001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("symbols = %v, want %v", got, want)
	}
}

func TestSymbolSearchReturnsGotdxMetadataAndDeduplicates(t *testing.T) {
	now := time.Unix(0, 0)
	loader := &fakeSymbolDirectoryLoader{
		stocks: map[uint8][]proto.Security{0: {{Code: "000001", Name: "Ping An Bank"}}},
		ex: []proto.ExListItem{
			{Market: 2, Category: 7, Code: "HK0001", Name: "Tencent"},
			{Market: 2, Category: 7, Code: "HK0001", Name: "Tencent duplicate"},
		},
	}
	cache := newSearchTestCache(loader, &now)

	mainItems, err := cache.search("000001", 20)
	if err != nil {
		t.Fatalf("main search: %v", err)
	}
	if len(mainItems) != 1 {
		t.Fatalf("main items = %d, want 1", len(mainItems))
	}
	if got, want := mainItems[0], (symbolSearchItem{
		Symbol:      "000001",
		Description: "Ping An Bank",
		Exchange:    "SZ",
		Source:      "gotdx",
		Params:      map[string]uint8{"market": 0},
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("main item = %#v, want %#v", got, want)
	}

	exItems, err := cache.search("hk0001", 20)
	if err != nil {
		t.Fatalf("extended search: %v", err)
	}
	if len(exItems) != 1 {
		t.Fatalf("extended items = %d, want 1", len(exItems))
	}
	if got, want := exItems[0], (symbolSearchItem{
		Symbol:      "HK0001",
		Description: "Tencent",
		Exchange:    "HK",
		Source:      "gotdx",
		Params:      map[string]uint8{"category": 7},
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("extended item = %#v, want %#v", got, want)
	}
}

func TestSymbolDirectoryCacheReusesFreshDirectory(t *testing.T) {
	now := time.Unix(0, 0)
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{0: {{Code: "000001", Name: "Ping An"}}}}
	cache := newSearchTestCache(loader, &now)

	for range 2 {
		if _, err := cache.search("000001", 20); err != nil {
			t.Fatalf("search: %v", err)
		}
	}
	if loader.stockCalls != 3 || loader.exCalls != 0 {
		t.Fatalf("loader calls = stock:%d ex:%d, want stock:3 ex:0", loader.stockCalls, loader.exCalls)
	}
}

func TestSymbolDirectoryCacheFallsBackToOldDirectoryAfterRefreshFailure(t *testing.T) {
	now := time.Unix(0, 0)
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{0: {{Code: "000001", Name: "Ping An"}}}}
	cache := newSearchTestCache(loader, &now)

	if _, err := cache.search("000001", 20); err != nil {
		t.Fatalf("initial search: %v", err)
	}
	loader.err = errors.New("TDX unavailable")
	now = now.Add(31 * time.Minute)
	items, err := cache.search("000001", 20)
	if err != nil {
		t.Fatalf("fallback search: %v", err)
	}
	if len(items) != 1 || items[0].Symbol != "000001" {
		t.Fatalf("fallback items = %#v", items)
	}
	failedRefreshCalls := loader.stockCalls
	if _, err := cache.search("000001", 20); err != nil {
		t.Fatalf("second fallback search: %v", err)
	}
	if loader.stockCalls != failedRefreshCalls {
		t.Fatalf("loader retried before backoff elapsed: calls = %d, want %d", loader.stockCalls, failedRefreshCalls)
	}
}

func TestSymbolSearchHandlerValidatesRequestAndIsRegistered(t *testing.T) {
	now := time.Unix(0, 0)
	loader := &fakeSymbolDirectoryLoader{stocks: map[uint8][]proto.Security{0: {{Code: "000001", Name: "Ping An"}}}}
	router := newRouter(newSearchTestCache(loader, &now))
	for _, body := range []string{"{", `{"query":"   "}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/symbol/search", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("POST /api/symbol/search %s = %d, want 400", body, resp.Code)
		}
	}

	handlerRouter := gin.New()
	handlerRouter.POST("/api/symbol/search", newSymbolSearchHandler(newSearchTestCache(loader, &now)))
	req := httptest.NewRequest(http.MethodPost, "/api/symbol/search", bytes.NewBufferString(`{"query":"000001","limit":101}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handlerRouter.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var items []symbolSearchItem
	if err := json.Unmarshal(resp.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}
